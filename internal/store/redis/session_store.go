package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const defaultSessionTTL = 2 * time.Minute

// PutSession 保存在线会话；TTL 取较短默认值，避免脏会话长期残留。
func (c *Client) PutSession(ctx context.Context, userID string, record SessionRecord, ttl time.Duration) error {
	if c == nil || c.kv == nil {
		return fmt.Errorf("nil redis client")
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	ttlVal := ttlOrDefault(ttl, defaultSessionTTL)
	if err := c.kv.Set(ctx, SessionKey(userID), payload, ttlVal).Err(); err != nil {
		return err
	}
	if record.TokenHash != "" {
		if err := c.kv.Set(ctx, SessionLookupKey(record.TokenHash), userID, ttlVal).Err(); err != nil {
			return fmt.Errorf("set session lookup: %w", err)
		}
	}
	return nil
}

// SaveSessionWithPlainToken 写入会话主记录并建立令牌反查键；plainToken 不落 Redis。
func (c *Client) SaveSessionWithPlainToken(ctx context.Context, userID, plainToken string, record SessionRecord, ttl time.Duration) error {
	if plainToken == "" {
		return fmt.Errorf("empty session token")
	}
	rec := record
	rec.TokenHash = HashSessionToken(plainToken)
	if rec.SessionVer == 0 {
		rec.SessionVer = 1
	}
	return c.PutSession(ctx, userID, rec, ttl)
}

// ResolveUserIDByPlainToken 通过明文令牌反查 user_id。
func (c *Client) ResolveUserIDByPlainToken(ctx context.Context, plainToken string) (string, bool, error) {
	if c == nil || c.kv == nil {
		return "", false, fmt.Errorf("nil redis client")
	}
	h := HashSessionToken(plainToken)
	if h == "" {
		return "", false, nil
	}
	raw, err := c.kv.Get(ctx, SessionLookupKey(h)).Result()
	if errors.Is(err, goredis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if raw == "" {
		return "", false, nil
	}
	return raw, true, nil
}

// GetSession 读取在线会话；键不存在时 ok=false 且 err=nil。
func (c *Client) GetSession(ctx context.Context, userID string) (SessionRecord, bool, error) {
	if c == nil || c.kv == nil {
		return SessionRecord{}, false, fmt.Errorf("nil redis client")
	}
	raw, err := c.kv.Get(ctx, SessionKey(userID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return SessionRecord{}, false, nil
	}
	if err != nil {
		return SessionRecord{}, false, err
	}
	var rec SessionRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return SessionRecord{}, false, fmt.Errorf("unmarshal session: %w", err)
	}
	return rec, true, nil
}

// DeleteSession 删除在线会话；键不存在时视为成功，便于幂等下线。
func (c *Client) DeleteSession(ctx context.Context, userID string) error {
	if c == nil || c.kv == nil {
		return fmt.Errorf("nil redis client")
	}
	return c.kv.Del(ctx, SessionKey(userID)).Err()
}
