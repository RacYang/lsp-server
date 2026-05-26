package roomadapter

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/pkg/logx"
)

// PersistPublishAndFinalize 持久化通知列表，打幂等戳，再通过 Redis RPUSH 发布事件。
func (s *GRPCServer) PersistPublishAndFinalize(ctx context.Context, roomID, idemKey string, notifications []roomsvc.Notification) error {
	notifications = expandPerSeatNotifications(notifications)
	events, err := s.persistNotifications(ctx, roomID, notifications)
	if err != nil {
		return err
	}
	s.markIdempotency(ctx, roomID, idemKey)
	for idx, event := range events {
		s.afterEventSideEffects(ctx, roomID, notifications[idx], event.evt, event.cursor)
		s.publishToRedis(ctx, roomID, event.evt)
	}
	return nil
}

// publishToRedis 将已生成的事件帧序列化后推送到 Redis List，供 gate 通过 BLPOP 消费。
// rdb 为 nil 时（单进程模式）静默跳过；推送失败仅警告，不阻断事件流。
func (s *GRPCServer) publishToRedis(ctx context.Context, roomID string, evt *svcv1.RoomServiceStreamEventsResponse) {
	if s.rdb == nil {
		return
	}
	data, err := proto.Marshal(evt)
	if err != nil {
		logx.Warn(logx.WithRoomID(ctx, roomID), "序列化事件失败，跳过 Redis 推送", "err", err.Error())
		return
	}
	if err := s.rdb.RoomEventQueuePush(ctx, roomID, data); err != nil {
		logx.Warn(logx.WithRoomID(ctx, roomID), "推送事件到 Redis 队列失败", "err", err.Error())
	}
}

type persistedEvent struct {
	cursor string
	evt    *svcv1.RoomServiceStreamEventsResponse
}

func (s *GRPCServer) persistNotifications(ctx context.Context, roomID string, notifications []roomsvc.Notification) ([]persistedEvent, error) {
	if len(notifications) == 0 {
		return nil, nil
	}
	cursors := make([]string, len(notifications))
	if s.ev == nil {
		for idx, notification := range notifications {
			cursors[idx] = fmt.Sprintf("%s-%d", notification.Kind, idx)
		}
	} else {
		rows := make([]postgres.RoomEventRow, 0, len(notifications))
		for _, notification := range notifications {
			rows = append(rows, postgres.RoomEventRow{
				Kind:       string(notification.Kind),
				Payload:    append([]byte(nil), notification.Payload...),
				TargetSeat: notification.TargetSeat.Proto(),
			})
		}
		persistedRows, err := s.ev.AppendEvents(ctx, roomID, rows)
		if err != nil {
			return nil, err
		}
		for idx, row := range persistedRows {
			cursors[idx] = fmt.Sprintf("%s:%d", roomID, row.Seq)
		}
	}

	out := make([]persistedEvent, 0, len(notifications))
	for idx, notification := range notifications {
		evt, err := mapNotificationToEvent(roomID, cursors[idx], notification)
		if err != nil {
			return nil, err
		}
		out = append(out, persistedEvent{cursor: cursors[idx], evt: evt})
	}
	return out, nil
}

func expandPerSeatNotifications(notifications []roomsvc.Notification) []roomsvc.Notification {
	out := make([]roomsvc.Notification, 0, len(notifications))
	for _, notification := range notifications {
		if notification.Privacy != roomsvc.PrivacyPerSeat || notification.Project == nil {
			out = append(out, notification)
			continue
		}
		for seat := 0; seat < 4; seat++ {
			projected := notification
			projected.TargetSeat = roomsvc.Seat(seat)
			projected.Payload = notification.Project(roomsvc.Seat(seat))
			projected.Privacy = roomsvc.PrivacyPublic
			projected.Project = nil
			out = append(out, projected)
		}
	}
	return out
}

func (s *GRPCServer) afterEventSideEffects(ctx context.Context, roomID string, notification roomsvc.Notification, evt *svcv1.RoomServiceStreamEventsResponse, cursor string) {
	s.persistRoomMeta(ctx, roomID, parseSinceSeq(roomID, cursor), &notification)
	if s.gs != nil {
		players, _, _, ok := s.rooms.RoomSnapshot(roomID)
		if ok && len(players) > 0 {
			if err := s.gs.CreateGameSummary(ctx, roomID, s.rooms.RuleID(), append([]string(nil), players...)); err != nil {
				logx.Error(logx.WithRoomID(ctx, roomID), "创建对局摘要失败", "err", err.Error())
			}
		}
	}
	if notification.Kind == roomsvc.KindSettlement && s.st != nil && evt.GetSettlement() != nil {
		// evt.GetSettlement() 已是 *clientv1.SettlementNotify，直接持久化。
		if err := s.st.AppendSettlement(ctx, evt.GetSettlement()); err != nil {
			logx.Error(logx.WithRoomID(ctx, roomID), "结算数据持久化失败", "err", err.Error())
		}
	}
	if notification.Kind == roomsvc.KindSettlement && s.gs != nil {
		if err := s.gs.EndGameSummary(ctx, roomID, time.Now().UTC()); err != nil {
			logx.Error(logx.WithRoomID(ctx, roomID), "结束对局摘要失败", "err", err.Error())
		}
	}
}

func mapPGRowToEvent(roomID string, row postgres.RoomEventRow) (*svcv1.RoomServiceStreamEventsResponse, error) {
	n := roomsvc.Notification{Kind: roomsvc.Kind(row.Kind), Payload: append([]byte(nil), row.Payload...), TargetSeat: roomsvc.Seat(row.TargetSeat)}
	cur := fmt.Sprintf("%s:%d", roomID, row.Seq)
	return mapNotificationToEvent(roomID, cur, n)
}
