package roomadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	act "racoo.cn/lsp/internal/service/room/actor"
	eng "racoo.cn/lsp/internal/service/room/engine"
	codec "racoo.cn/lsp/internal/service/room/engine/codec"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	"racoo.cn/lsp/internal/metrics"
	"racoo.cn/lsp/internal/store/redis"
)

// protoToEnginePhaseToken 将 proto PhaseToken 转换为 engine.PhaseToken；nil 输入返回 nil。
func protoToEnginePhaseToken(p *clientv1.PhaseToken) *eng.PhaseToken {
	if p == nil {
		return nil
	}
	return &eng.PhaseToken{
		Step:   p.GetStep(),
		Reason: eng.WaitingReason(codec.WaitingReasonFromProto(p.GetReason())),
	}
}

// ApplyEvent 通过 room.Service 驱动真实房间 worker，并把产出的通知桥接到订阅流。
func (s *GRPCServer) ApplyEvent(ctx context.Context, req *svcv1.ApplyEventRequest) (*svcv1.ApplyEventResponse, error) {
	started := time.Now()
	if s == nil {
		return nil, fmt.Errorf("nil room grpc server")
	}
	if !s.ready.Load() {
		return &svcv1.ApplyEventResponse{Accepted: false, Error: "recovering"}, nil
	}
	if s.rooms == nil {
		return nil, fmt.Errorf("nil room service")
	}
	defer func() {
		elapsed := time.Since(started).Seconds()
		metrics.GRPCApplyEventSeconds.Observe(elapsed)
	}()
	roomID := req.GetRoomId()
	if roomID == "" {
		return &svcv1.ApplyEventResponse{Accepted: false, Error: "empty room_id"}, nil
	}
	userID := req.GetUserId()
	if userID == "" {
		return &svcv1.ApplyEventResponse{Accepted: false, Error: "empty user_id"}, nil
	}
	idemKey := strings.TrimSpace(req.GetIdempotencyKey())
	if idemKey != "" && s.rdb != nil {
		scope := "room_apply_event"
		fullKey := roomID + ":" + idemKey
		rec, ok, err := s.rdb.GetIdempotency(ctx, scope, fullKey)
		if err != nil {
			return &svcv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
		}
		if ok && rec.Result == "ok" {
			return &svcv1.ApplyEventResponse{Accepted: true}, nil
		}
	}
	if _, err := s.rooms.Join(ctx, roomID, userID); err != nil && !errors.Is(err, act.ErrRoomFull) {
		return &svcv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
	}
	s.persistRoomMeta(ctx, roomID, 0, nil)
	switch req.GetBody().(type) {
	case *svcv1.ApplyEventRequest_Ready:
		notifications, err := s.rooms.Ready(ctx, roomID, userID)
		if err != nil {
			return &svcv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
		}
		if err := s.PersistPublishAndFinalize(ctx, roomID, idemKey, notifications); err != nil {
			return &svcv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
		}
		return &svcv1.ApplyEventResponse{Accepted: true}, nil
	case *svcv1.ApplyEventRequest_Discard:
		notifications, err := s.rooms.Discard(ctx, roomID, userID, req.GetDiscard().GetTile(), protoToEnginePhaseToken(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Pong:
		notifications, err := s.rooms.Pong(ctx, roomID, userID, protoToEnginePhaseToken(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Chi:
		notifications, err := s.rooms.Chi(ctx, roomID, userID, req.GetChi().GetTiles(), protoToEnginePhaseToken(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Gang:
		notifications, err := s.rooms.Gang(ctx, roomID, userID, req.GetGang().GetTile(), protoToEnginePhaseToken(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Hu:
		notifications, err := s.rooms.Hu(ctx, roomID, userID, protoToEnginePhaseToken(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Pass:
		notifications, err := s.rooms.Pass(ctx, roomID, userID, protoToEnginePhaseToken(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_OpeningAction:
		event := req.GetOpeningAction()
		notifications, err := s.rooms.OpeningAction(ctx, roomID, userID, event.GetAction(), event.GetTiles(), event.GetDirection(), event.GetSuit(), event.GetParams(), protoToEnginePhaseToken(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Leave:
		if err := s.rooms.Leave(ctx, roomID, userID); err != nil {
			return &svcv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
		}
		s.markIdempotency(ctx, roomID, idemKey)
		return &svcv1.ApplyEventResponse{Accepted: true}, nil
	case *svcv1.ApplyEventRequest_Join:
		// Join 仅占座：s.rooms.Join 已在 switch 之前（Join 调用）隐式执行。
		s.markIdempotency(ctx, roomID, idemKey)
		return &svcv1.ApplyEventResponse{Accepted: true}, nil
	case *svcv1.ApplyEventRequest_MarkOffline:
		// MarkOffline 为 fire-and-forget 指令；actor 内部启动投降倒计时，无通知产出。
		s.rooms.MarkSeatOffline(roomID, userID)
		return &svcv1.ApplyEventResponse{Accepted: true}, nil
	case *svcv1.ApplyEventRequest_CancelOffline:
		// CancelOffline 为 fire-and-forget 指令；actor 内部取消待触发的投降倒计时。
		s.rooms.CancelOfflineSurrender(roomID, userID)
		return &svcv1.ApplyEventResponse{Accepted: true}, nil
	default:
		return &svcv1.ApplyEventResponse{Accepted: false, Error: "unsupported room event"}, nil
	}
}

func (s *GRPCServer) applyNotifications(ctx context.Context, roomID, idemKey string, notifications []eng.Notification, err error) (*svcv1.ApplyEventResponse, error) {
	if err != nil {
		metrics.GRPCApplyEventTotal.WithLabelValues("error").Inc()
		return &svcv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
	}
	if err := s.PersistPublishAndFinalize(ctx, roomID, idemKey, notifications); err != nil {
		metrics.GRPCApplyEventTotal.WithLabelValues("persist_error").Inc()
		return &svcv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
	}
	metrics.GRPCApplyEventTotal.WithLabelValues("ok").Inc()
	return &svcv1.ApplyEventResponse{Accepted: true}, nil
}

func (s *GRPCServer) markIdempotency(ctx context.Context, roomID, idemKey string) {
	if idemKey == "" || s.rdb == nil {
		return
	}
	scope := "room_apply_event"
	fullKey := roomID + ":" + idemKey
	// 事件已成功持久化后，幂等键只做成功标记；写入失败不再回滚已落盘事件。
	_, _ = s.rdb.PutIdempotencyAbsent(ctx, scope, fullKey, redis.IdempotencyRecord{Result: "ok"}, s.idempotencyTTL)
}
