package redis

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// UserProfile 是 Redis 中保存的玩家公开资料。
type UserProfile struct {
	Nickname string `json:"nickname"`
}

// PutUserProfile 写入玩家资料。
func (c *Client) PutUserProfile(ctx context.Context, userID string, profile UserProfile, ttl time.Duration) error {
	if c == nil || c.kv == nil || userID == "" {
		return nil
	}
	payload, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return c.kv.Set(ctx, UserProfileKey(userID), payload, ttl).Err()
}

// GetUserProfile 读取玩家资料。
func (c *Client) GetUserProfile(ctx context.Context, userID string) (UserProfile, bool, error) {
	if c == nil || c.kv == nil || userID == "" {
		return UserProfile{}, false, nil
	}
	raw, err := c.kv.Get(ctx, UserProfileKey(userID)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return UserProfile{}, false, nil
		}
		return UserProfile{}, false, err
	}
	var profile UserProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return UserProfile{}, false, err
	}
	return profile, true, nil
}
