package session

import (
	"context"
	"sync"
	"time"

	"racoo.cn/lsp/internal/store/redis"
)

const userProfileTTL = 7 * 24 * time.Hour

// UserProfile 是 handler 可见的玩家公开资料结构；持久化层负责转换到 Redis 表示。
type UserProfile = redis.UserProfile

// UserDirectory 管理玩家公开资料；Redis 不可用时退回进程内内存表。
type UserDirectory struct {
	mu    sync.RWMutex
	mem   map[string]redis.UserProfile
	store *redis.Client
}

func NewUserDirectory(store *redis.Client) *UserDirectory {
	return &UserDirectory{mem: make(map[string]redis.UserProfile), store: store}
}

func (d *UserDirectory) Set(ctx context.Context, userID string, profile redis.UserProfile) error {
	if d == nil || userID == "" {
		return nil
	}
	d.mu.Lock()
	d.mem[userID] = profile
	d.mu.Unlock()
	if d.store != nil {
		return d.store.PutUserProfile(ctx, userID, profile, userProfileTTL)
	}
	return nil
}

func (d *UserDirectory) Get(ctx context.Context, userID string) (redis.UserProfile, bool, error) {
	if d == nil || userID == "" {
		return redis.UserProfile{}, false, nil
	}
	if d.store != nil {
		if profile, ok, err := d.store.GetUserProfile(ctx, userID); err != nil || ok {
			return profile, ok, err
		}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	profile, ok := d.mem[userID]
	return profile, ok, nil
}
