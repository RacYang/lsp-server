package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const defaultIdempotencyTTL = 10 * time.Minute

// PutIdempotencyAbsent 仅在键不存在时写入幂等记录；created=false 表示此前已有结果。
func (c *Client) PutIdempotencyAbsent(ctx context.Context, scope, key string, record IdempotencyRecord, ttl time.Duration) (bool, error) {
	if c == nil || c.kv == nil {
		return false, fmt.Errorf("nil redis client")
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return false, fmt.Errorf("marshal idempotency: %w", err)
	}
	created, err := c.kv.SetNX(ctx, IdempotencyKey(scope, key), payload, ttlOrDefault(ttl, defaultIdempotencyTTL)).Result()
	if err != nil {
		return false, err
	}
	return created, nil
}

// GetIdempotency 读取幂等记录；键不存在时返回 ok=false。
func (c *Client) GetIdempotency(ctx context.Context, scope, key string) (IdempotencyRecord, bool, error) {
	if c == nil || c.kv == nil {
		return IdempotencyRecord{}, false, fmt.Errorf("nil redis client")
	}
	raw, err := c.kv.Get(ctx, IdempotencyKey(scope, key)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return IdempotencyRecord{}, false, err
	}
	var rec IdempotencyRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("unmarshal idempotency: %w", err)
	}
	return rec, true, nil
}

// DeleteIdempotency 删除幂等键；便于测试或补偿回滚。
func (c *Client) DeleteIdempotency(ctx context.Context, scope, key string) error {
	if c == nil || c.kv == nil {
		return fmt.Errorf("nil redis client")
	}
	return c.kv.Del(ctx, IdempotencyKey(scope, key)).Err()
}
