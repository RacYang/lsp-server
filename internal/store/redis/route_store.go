package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const defaultRouteTTL = 45 * time.Second

// PutRoomRouteCache 写入房间路由缓存；它不是权威真相源，只用于减少 etcd 读放大。
func (c *Client) PutRoomRouteCache(ctx context.Context, roomID string, record RouteRecord, ttl time.Duration) error {
	if c == nil || c.kv == nil {
		return fmt.Errorf("nil redis client")
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal route cache: %w", err)
	}
	return c.kv.Set(ctx, RoomRouteCacheKey(roomID), payload, ttlOrDefault(ttl, defaultRouteTTL)).Err()
}

// GetRoomRouteCache 读取房间路由缓存；miss 时返回 ok=false。
func (c *Client) GetRoomRouteCache(ctx context.Context, roomID string) (RouteRecord, bool, error) {
	if c == nil || c.kv == nil {
		return RouteRecord{}, false, fmt.Errorf("nil redis client")
	}
	raw, err := c.kv.Get(ctx, RoomRouteCacheKey(roomID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return RouteRecord{}, false, nil
	}
	if err != nil {
		return RouteRecord{}, false, err
	}
	var rec RouteRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return RouteRecord{}, false, fmt.Errorf("unmarshal route cache: %w", err)
	}
	return rec, true, nil
}

// DeleteRoomRouteCache 删除路由缓存；房间迁移或关闭时调用。
func (c *Client) DeleteRoomRouteCache(ctx context.Context, roomID string) error {
	if c == nil || c.kv == nil {
		return fmt.Errorf("nil redis client")
	}
	return c.kv.Del(ctx, RoomRouteCacheKey(roomID)).Err()
}
