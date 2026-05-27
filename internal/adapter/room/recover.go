package roomadapter

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"racoo.cn/lsp/internal/cluster"
	"racoo.cn/lsp/internal/metrics"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

// RecoverConcurrency 是冷启动恢复的最大并发房间数。
const RecoverConcurrency = 8

// RecoverTimeout 是整批房间恢复的超时上限；超时后已完成的房间继续运行，剩余跳过（降级上线）。
const RecoverTimeout = 120 * time.Second

// RecoverOwnedRooms 从 etcd/Redis/Postgres 并发恢复本节点拥有的全部房间。
// rt 或 rcli 为 nil 时静默返回；etcd 查询失败时向上传播。
func RecoverOwnedRooms(ctx context.Context, rt *cluster.EtcdRouter, rnodeID string, rcli *redis.Client, ev *postgres.RoomEventStore, gs *postgres.GameSummaryStore, svc *roomsvc.Service) error {
	if rt == nil || rcli == nil || svc == nil {
		return nil
	}
	roomIDs, err := rt.ListRoomsByOwner(ctx, rnodeID)
	if err != nil {
		return err
	}
	recoverCtx, cancel := context.WithTimeout(ctx, RecoverTimeout)
	defer cancel()

	sem := semaphore.NewWeighted(RecoverConcurrency)
	eg, egCtx := errgroup.WithContext(recoverCtx)
	for _, rid := range roomIDs {
		roomID := rid
		if err := sem.Acquire(egCtx, 1); err != nil {
			// 整体超时，降级上线：终止剩余恢复，已完成的房间继续运行。
			metrics.RoomRecoverSkipTotal.Add(float64(len(roomIDs)))
			logx.Warn(ctx, "房间冷启动恢复超时，降级上线", "remaining", len(roomIDs), "err", err.Error())
			break
		}
		eg.Go(func() error {
			defer sem.Release(1)
			return RecoverSingleRoom(egCtx, rcli, ev, gs, svc, roomID)
		})
	}
	return eg.Wait()
}

// RecoverSingleRoom 恢复单个房间的状态；失败时返回 error。
func RecoverSingleRoom(ctx context.Context, rcli *redis.Client, ev *postgres.RoomEventStore, gs *postgres.GameSummaryStore, svc *roomsvc.Service, roomID string) error {
	var (
		players   []string
		state     = "waiting"
		roundJSON []byte
	)
	if rcli == nil {
		return errors.New("nil redis client")
	}
	if meta, ok, err := rcli.GetRoomSnapMeta(ctx, roomID); err != nil {
		return err
	} else if ok {
		players = append(players, meta.PlayerIDs...)
		if strings.TrimSpace(meta.State) != "" {
			state = meta.State
		}
		if meta.RoundJSON != "" {
			roundJSON = []byte(meta.RoundJSON)
		}
	}
	if gs != nil {
		summary, err := gs.GetGameSummary(ctx, roomID)
		if err != nil && !errors.Is(err, postgres.ErrGameSummaryNotFound) {
			return err
		}
		if err == nil {
			if len(summary.PlayerIDs) > 0 {
				players = append([]string(nil), summary.PlayerIDs...)
			}
			if summary.EndedAt != nil {
				state = "closed"
			}
		}
	}
	if ev != nil {
		rows, err := ev.ListEventsAfter(ctx, roomID, 0)
		if err != nil {
			return err
		}
		derived, clearRound := DeriveRecoveredState(state, rows)
		if derived != "" {
			state = derived
		}
		// settlement 后又有开局事件，说明新一局已开始；旧局 roundJSON 快照不适用，丢弃。
		if clearRound {
			roundJSON = nil
		}
	}
	if state == "closed" || len(players) == 0 {
		return nil
	}
	if state == "playing" && len(roundJSON) == 0 {
		state = "ready"
	}
	if err := svc.RecoverRoom(roomID, players, state, roundJSON); err != nil {
		if errors.Is(err, roomsvc.ErrRoundPersistUnsupportedSchema) {
			return svc.RecoverRoom(roomID, players, "ready", nil)
		}
		return err
	}
	return nil
}

// DeriveRecoveredState 根据持久化事件行推导最终恢复状态。
// 若 settlement 之后出现开局类事件（如 start_game），表示新一局已开始；
// 此时同时返回 clearRound=true，提示调用方丢弃可能属于上一局的 roundJSON 快照。
func DeriveRecoveredState(current string, rows []postgres.RoomEventRow) (state string, clearRound bool) {
	state = current
	var afterSettlement bool
	for _, row := range rows {
		switch row.Kind {
		case string(roomsvc.KindSettlement):
			state = "closed"
			afterSettlement = true
		case string(roomsvc.KindOpeningDone), string(roomsvc.KindStartGame), string(roomsvc.KindDrawTile), string(roomsvc.KindAction):
			state = "playing"
			if afterSettlement {
				// settlement 后紧跟开局事件，说明新一局已开始，旧快照不适用。
				clearRound = true
			}
		}
	}
	return state, clearRound
}
