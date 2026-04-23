package router

import (
	"context"
	"strings"
)

// Resolver 根据房间 ID 解析应转发到的 room 节点 ID；未找到时 ok 为 false。
type Resolver interface {
	ResolveRoomOwner(ctx context.Context, roomID string) (nodeID string, ok bool, err error)
}

// Claimer 由 lobby 在创建或首次绑定时声明房间归属（与租约绑定）。
type Claimer interface {
	ClaimRoom(ctx context.Context, roomID, roomNodeID string) error
}

// SanitizeRoomID 去除 roomID 两端空白，避免 etcd 键误拼空格。
func SanitizeRoomID(roomID string) string {
	return strings.TrimSpace(roomID)
}
