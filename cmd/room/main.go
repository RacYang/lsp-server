package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"racoo.cn/lsp/internal/app"
	"racoo.cn/lsp/internal/cluster/discovery"
	"racoo.cn/lsp/internal/cluster/nodeid"
	"racoo.cn/lsp/internal/cluster/router"
	"racoo.cn/lsp/internal/config"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

const defaultRoomNodeID = "room-local"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := run(ctx, stop)
	os.Exit(code)
}

func run(ctx context.Context, stop context.CancelFunc) int {
	defer stop()
	cfg, err := config.Load(os.Getenv("LSP_CONFIG"))
	if err != nil {
		logx.Error(ctx, "房间服务配置加载失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return 1
	}
	var (
		ev   *postgres.RoomEventStore
		gs   *postgres.GameSummaryStore
		st   *postgres.SettlementStore
		rcli *redis.Client
	)
	if cfg.PostgresDSN != "" {
		pool, err := postgres.OpenPool(ctx, cfg.PostgresDSN)
		if err != nil {
			logx.Error(ctx, "房间事件持久化数据库连接失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
			return 1
		}
		defer pool.Close()
		ev = postgres.NewRoomEventStore(pool)
		gs = postgres.NewGameSummaryStore(pool)
		st = postgres.NewSettlementStore(pool)
	}
	if cfg.RedisAddr != "" {
		c, err := redis.NewClient(cfg.RedisAddr)
		if err != nil {
			logx.Error(ctx, "Redis 连接失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
			return 1
		}
		defer func() { _ = c.Close() }()
		rcli = c
	}
	if cfg.EtcdEndpoints != "" && rcli == nil {
		logx.Error(ctx, "启用 etcd 房间恢复时必须同时配置 Redis", "trace_id", "", "user_id", "", "room_id", "", "err", "missing redis.addr")
		return 1
	}
	svcCore := roomsvc.NewServiceWithRule(roomsvc.NewLobby(), cfg.RuleID)
	svcCore.SetMailboxCapacity(cfg.Runtime.RoomMailboxCapacity)
	svcCore.SetTimeoutConfig(roomsvc.TimeoutConfig{
		ExchangeThree: cfg.RoomTimeouts.ExchangeThree,
		QueMen:        cfg.RoomTimeouts.QueMen,
		ClaimWindow:   cfg.RoomTimeouts.ClaimWindow,
		TsumoWindow:   cfg.RoomTimeouts.TsumoWindow,
		Discard:       cfg.RoomTimeouts.Discard,
	})
	svc := newRoomGRPCServer(svcCore, ev, gs, st, rcli)
	svc.setIdempotencyTTL(cfg.Runtime.RedisIdempotencyTTL)
	if cfg.EtcdEndpoints != "" {
		svc.setReady(false)
		cli, err := clientv3.New(clientv3.Config{Endpoints: splitEndpoints(cfg.EtcdEndpoints), DialTimeout: 5 * time.Second})
		if err != nil {
			logx.Error(ctx, "房间服务 etcd 客户端初始化失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
			return 1
		}
		defer func() { _ = cli.Close() }()
		disco := discovery.NewEtcd(cli, "/lsp", 30)
		reg, err := disco.RegisterAndKeepAlive(ctx, nodeid.KindRoom, defaultRoomNodeID, discovery.NodeMeta{
			AdvertiseAddr: cfg.ServerAddr,
			Version:       "phase3",
		}, 10*time.Second)
		if err != nil {
			logx.Error(ctx, "房间节点注册到 etcd 失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
			return 1
		}
		defer func() { _ = reg.Stop(context.Background()) }()

		if rcli != nil {
			rt := router.NewEtcd(cli, "/lsp")
			if err := recoverOwnedRooms(ctx, rt, defaultRoomNodeID, rcli, ev, gs, svcCore); err != nil {
				logx.Error(ctx, "房间冷启动恢复失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
				return 1
			}
		}
		svc.setReady(true)
	}
	a, err := app.NewGRPC(cfg.ServerAddr, func(s *grpc.Server) {
		registerRoomService(s, svc)
	})
	if err != nil {
		logx.Error(ctx, "房间服务装配失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return 1
	}
	obsStop, err := app.StartObsHTTP(cfg.ObsAddr, rcli)
	if err != nil {
		logx.Error(ctx, "可观测性 HTTP 启动失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return 1
	}
	defer obsStop()
	logx.Info(ctx, "房间服务启动", "trace_id", "", "user_id", "", "room_id", "", "addr", cfg.ServerAddr)
	if err := a.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logx.Error(ctx, "房间服务退出异常", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return 1
	}
	return 0
}

func recoverOwnedRooms(ctx context.Context, rt *router.Etcd, rnodeID string, rcli *redis.Client, ev *postgres.RoomEventStore, gs *postgres.GameSummaryStore, svc *roomsvc.Service) error {
	if rt == nil || rcli == nil || svc == nil {
		return nil
	}
	roomIDs, err := rt.ListRoomsByOwner(ctx, rnodeID)
	if err != nil {
		return err
	}
	for _, roomID := range roomIDs {
		var (
			players   []string
			state     = "waiting"
			roundJSON []byte
		)
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
			if derived := deriveRecoveredState(state, rows); derived != "" {
				state = derived
			}
		}
		if state == "closed" || len(players) == 0 {
			continue
		}
		if state == "playing" && len(roundJSON) == 0 {
			state = "ready"
		}
		if err := svc.RecoverRoom(roomID, players, state, roundJSON); err != nil {
			if errors.Is(err, roomsvc.ErrRoundPersistUnsupportedSchema) {
				state = "ready"
				roundJSON = nil
				if errRetry := svc.RecoverRoom(roomID, players, state, roundJSON); errRetry != nil {
					return errRetry
				}
				continue
			}
			return err
		}
	}
	return nil
}

func splitEndpoints(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func deriveRecoveredState(current string, rows []postgres.RoomEventRow) string {
	state := current
	for _, row := range rows {
		switch row.Kind {
		case string(roomsvc.KindExchangeThreeDone), string(roomsvc.KindQueMenDone), string(roomsvc.KindStartGame), string(roomsvc.KindDrawTile), string(roomsvc.KindAction):
			state = "playing"
		case string(roomsvc.KindSettlement):
			state = "closed"
		}
	}
	return state
}
