package roomadapter

import (
	"context"
	"fmt"
	"strings"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/redis"
)

// ApplyEvent 通过 room.Service 驱动真实房间 worker，并把产出的通知桥接到订阅流。
func (s *GRPCServer) ApplyEvent(ctx context.Context, req *svcv1.ApplyEventRequest) (*svcv1.ApplyEventResponse, error) {
	if s == nil {
		return nil, fmt.Errorf("nil room grpc server")
	}
	if !s.ready.Load() {
		return &svcv1.ApplyEventResponse{Accepted: false, Error: "recovering"}, nil
	}
	if s.rooms == nil {
		return nil, fmt.Errorf("nil room service")
	}
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
	if _, err := s.rooms.Join(ctx, roomID, userID); err != nil && err.Error() != "room full" {
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
		notifications, err := s.rooms.Discard(ctx, roomID, userID, req.GetDiscard().GetTile(), clusterPhaseTokToRoom(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Pong:
		notifications, err := s.rooms.Pong(ctx, roomID, userID, clusterPhaseTokToRoom(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Chi:
		notifications, err := s.rooms.Chi(ctx, roomID, userID, req.GetChi().GetTiles(), clusterPhaseTokToRoom(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Gang:
		notifications, err := s.rooms.Gang(ctx, roomID, userID, req.GetGang().GetTile(), clusterPhaseTokToRoom(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Hu:
		notifications, err := s.rooms.Hu(ctx, roomID, userID, clusterPhaseTokToRoom(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_Pass:
		notifications, err := s.rooms.Pass(ctx, roomID, userID, clusterPhaseTokToRoom(req.GetPhaseToken()))
		return s.applyNotifications(ctx, roomID, idemKey, notifications, err)
	case *svcv1.ApplyEventRequest_OpeningAction:
		event := req.GetOpeningAction()
		notifications, err := s.rooms.OpeningAction(ctx, roomID, userID, event.GetAction(), event.GetTiles(), event.GetDirection(), event.GetSuit(), event.GetParams(), clusterPhaseTokToRoom(req.GetPhaseToken()))
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
	default:
		return &svcv1.ApplyEventResponse{Accepted: false, Error: "unsupported room event"}, nil
	}
}

// clusterPhaseTokToRoom 把 client.v1.PhaseToken 映射到 room.PhaseToken；nil 时透传 nil。
// proto 统一后 ApplyEventRequest.phase_token 直接使用 client.v1.PhaseToken，无须 cluster 层中转。
func clusterPhaseTokToRoom(tok *clientv1.PhaseToken) *roomsvc.PhaseToken {
	if tok == nil {
		return nil
	}
	var reason roomsvc.WaitingReason
	switch tok.GetReason() {
	case clientv1.WaitingReason_WAITING_REASON_OPENING:
		reason = roomsvc.ReasonOpening
	case clientv1.WaitingReason_WAITING_REASON_CLAIM_WINDOW:
		reason = roomsvc.ReasonClaimWindow
	case clientv1.WaitingReason_WAITING_REASON_TSUMO:
		reason = roomsvc.ReasonTsumo
	case clientv1.WaitingReason_WAITING_REASON_DISCARD:
		reason = roomsvc.ReasonDiscard
	case clientv1.WaitingReason_WAITING_REASON_SURRENDER:
		reason = roomsvc.ReasonSurrender
	default:
		reason = roomsvc.ReasonNone
	}
	return &roomsvc.PhaseToken{Step: tok.GetStep(), Reason: reason}
}

func (s *GRPCServer) applyNotifications(ctx context.Context, roomID, idemKey string, notifications []roomsvc.Notification, err error) (*svcv1.ApplyEventResponse, error) {
	if err != nil {
		return &svcv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
	}
	if err := s.PersistPublishAndFinalize(ctx, roomID, idemKey, notifications); err != nil {
		return &svcv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
	}
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
