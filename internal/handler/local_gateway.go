package handler

import (
	"context"
	"fmt"

	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
)

// LocalRoomGateway 适配进程内房间服务，供 `cmd/all` 与本地 gate 冒烟复用。
type LocalRoomGateway struct {
	rooms *roomsvc.Service
	hub   *session.Hub
}

// NewLocalRoomGateway 创建进程内房间网关。
func NewLocalRoomGateway(rooms *roomsvc.Service, hub *session.Hub) *LocalRoomGateway {
	return &LocalRoomGateway{rooms: rooms, hub: hub}
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
