package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"racoo.cn/lsp/internal/metrics"
	storex "racoo.cn/lsp/internal/store"
)

const defaultIdempotencyTTL = 10 * time.Minute

// PutIdempotencyAbsent 仅在键不存在时写入幂等记录；created=false 表示此前已有结果。
func (c *Client) PutIdempotencyAbsent(ctx context.Context, scope, key string, record IdempotencyRecord, ttl time.Duration) (bool, error) {
	started := time.Now()
	var opErr error
	defer func() { metrics.ObserveStorage("redis", "put_idempotency_absent", started, opErr) }()
	if c == nil || c.kv == nil {
		opErr = fmt.Errorf("nil redis client")
		return false, opErr
	}
	payload, err := json.Marshal(record)
	if err != nil {
		opErr = fmt.Errorf("marshal idempotency: %w", err)
		return false, opErr
	}
	var created bool
	err = storex.Retry(ctx, "redis", "put_idempotency_absent", 2, func(opCtx context.Context) error {
		var err error
		created, err = c.kv.SetNX(opCtx, IdempotencyKey(scope, key), payload, ttlOrDefault(ttl, defaultIdempotencyTTL)).Result()
		return err
	})
	if err != nil {
		opErr = err
		return false, err
	}
	return created, nil
}

// GetIdempotency 读取幂等记录；键不存在时返回 ok=false。
func (c *Client) GetIdempotency(ctx context.Context, scope, key string) (IdempotencyRecord, bool, error) {
	started := time.Now()
	var opErr error
	defer func() { metrics.ObserveStorage("redis", "get_idempotency", started, opErr) }()
	if c == nil || c.kv == nil {
		opErr = fmt.Errorf("nil redis client")
		return IdempotencyRecord{}, false, opErr
	}
	var raw []byte
	err := storex.Retry(ctx, "redis", "get_idempotency", 2, func(opCtx context.Context) error {
		var err error
		raw, err = c.kv.Get(opCtx, IdempotencyKey(scope, key)).Bytes()
		return err
	})
	if errors.Is(err, goredis.Nil) {
		return IdempotencyRecord{}, false, nil
	}
	if err != nil {
		opErr = err
		return IdempotencyRecord{}, false, err
	}
	var rec IdempotencyRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		opErr = fmt.Errorf("unmarshal idempotency: %w", err)
		return IdempotencyRecord{}, false, opErr
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
