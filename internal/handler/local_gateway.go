package handler

import (
	"context"
	"fmt"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
)

// LocalRoomGateway 适配进程内房间服务，供 `cmd/all` 与本地 gate 冒烟复用。
type LocalRoomGateway struct {
	rooms *roomsvc.Service
	hub   *session.Hub
	sess  *session.Manager
}

// NewLocalRoomGateway 创建进程内房间网关；sess 可为 nil 表示不启用 Redis 会话。
func NewLocalRoomGateway(rooms *roomsvc.Service, hub *session.Hub, sess *session.Manager) *LocalRoomGateway {
	return &LocalRoomGateway{rooms: rooms, hub: hub, sess: sess}
}

// Join 直接走本地房间服务加入逻辑。
func (g *LocalRoomGateway) Join(ctx context.Context, roomID, userID string) (int, error) {
	if g == nil || g.rooms == nil {
		return -1, fmt.Errorf("nil local room gateway")
	}
	return g.rooms.Join(ctx, roomID, userID)
}

// Ready 触发本地 worker，并返回一个在 ReadyResp 之后执行的广播回调。
func (g *LocalRoomGateway) Ready(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Ready(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	return func() {
		for _, notification := range notifications {
			outMsgID, ok := outboundMsgID(notification.Kind)
			if !ok || g.hub == nil {
				continue
			}
			g.hub.Broadcast(roomID, outMsgID, notification.Payload)
		}
	}, nil
}

// EnsureRoomEventSubscription 本地进程内无 gRPC 事件流，由 Hub 广播承担。
func (g *LocalRoomGateway) EnsureRoomEventSubscription(_ context.Context, _, _ string) error {
	return nil
}

// Resume 基于 Redis 会话与内存房间视图构造快照；无持久化游标时以会话 LastCursor 为准。
func (g *LocalRoomGateway) Resume(ctx context.Context, sessionToken string) (*ResumeResult, error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	if g.sess == nil {
		return nil, fmt.Errorf("会话管理器未启用")
	}
	uid, srec, err := g.sess.Resume(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	if srec.RoomID == "" {
		return nil, fmt.Errorf("会话未绑定房间")
	}
	players, state, ok := g.rooms.RoomSnapshot(srec.RoomID)
	if !ok {
		return nil, fmt.Errorf("房间不存在或已回收")
	}
	snap := &clientv1.SnapshotNotify{
		RoomId:        srec.RoomID,
		PlayerIds:     append([]string(nil), players...),
		QueSuitBySeat: nil,
		Cursor:        srec.LastCursor,
		State:         state,
	}
	return &ResumeResult{
		UserID:              uid,
		RoomID:              srec.RoomID,
		Resumed:             true,
		Snapshot:            snap,
		SnapshotSinceCursor: srec.LastCursor,
	}, nil
}
