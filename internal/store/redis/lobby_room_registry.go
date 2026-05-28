package redis

import (
	"context"
	"encoding/json"
	"fmt"

	domainlobby "racoo.cn/lsp/internal/domain/lobby"
)

// LobbyRoomRegistry 实现 domainlobby.RoomRegistry，将大厅房间列表持久化到 Redis Hash。
// 使用单一 Hash 键 lsp:lobby:rooms，field=roomID，value=JSON(domainlobby.RoomRecord)。
// 进程重启后调用 ListAll 从此处恢复 Service 内存状态。
type LobbyRoomRegistry struct {
	c *Client
}

// NewLobbyRoomRegistry 构造 Redis 持久化的大厅房间注册表。
func NewLobbyRoomRegistry(c *Client) *LobbyRoomRegistry {
	return &LobbyRoomRegistry{c: c}
}

// UpsertRoom 序列化并写入（或覆盖）指定房间记录。
func (r *LobbyRoomRegistry) UpsertRoom(ctx context.Context, rec domainlobby.RoomRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("序列化大厅房间记录: %w", err)
	}
	return r.c.kv.HSet(ctx, LobbyRoomsKey(), rec.RoomID, data).Err()
}

// DeleteRoom 从注册表中移除指定房间。
func (r *LobbyRoomRegistry) DeleteRoom(ctx context.Context, roomID string) error {
	return r.c.kv.HDel(ctx, LobbyRoomsKey(), roomID).Err()
}

// ListAll 返回注册表中全部房间记录；损坏的单条记录跳过，不影响整体恢复。
func (r *LobbyRoomRegistry) ListAll(ctx context.Context) ([]domainlobby.RoomRecord, error) {
	vals, err := r.c.kv.HGetAll(ctx, LobbyRoomsKey()).Result()
	if err != nil {
		return nil, fmt.Errorf("读取大厅房间注册表: %w", err)
	}
	records := make([]domainlobby.RoomRecord, 0, len(vals))
	for _, v := range vals {
		var rec domainlobby.RoomRecord
		if err := json.Unmarshal([]byte(v), &rec); err != nil {
			continue // 跳过损坏记录，不中断恢复
		}
		records = append(records, rec)
	}
	return records, nil
}
