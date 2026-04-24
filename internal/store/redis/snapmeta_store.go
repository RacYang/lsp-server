package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const defaultSnapMetaTTL = 24 * time.Hour

// PutRoomSnapMeta 写入房间快照元数据摘要。
func (c *Client) PutRoomSnapMeta(ctx context.Context, roomID string, meta RoomSnapMeta, ttl time.Duration) error {
	if c == nil || c.kv == nil {
		return fmt.Errorf("nil redis client")
	}
	if roomID == "" {
		return fmt.Errorf("empty room_id")
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal snapmeta: %w", err)
	}
	return c.kv.Set(ctx, RoomSnapshotMetaKey(roomID), b, ttlOrDefault(ttl, defaultSnapMetaTTL)).Err()
}

// GetRoomSnapMeta 读取房间快照元数据；不存在时 ok=false。
func (c *Client) GetRoomSnapMeta(ctx context.Context, roomID string) (RoomSnapMeta, bool, error) {
	if c == nil || c.kv == nil {
		return RoomSnapMeta{}, false, fmt.Errorf("nil redis client")
	}
	raw, err := c.kv.Get(ctx, RoomSnapshotMetaKey(roomID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return RoomSnapMeta{}, false, nil
	}
	if err != nil {
		return RoomSnapMeta{}, false, err
	}
	var m RoomSnapMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return RoomSnapMeta{}, false, fmt.Errorf("unmarshal snapmeta: %w", err)
	}
	return m, true, nil
}
