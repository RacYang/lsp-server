package session

import (
	"context"
	"sync"
	"time"

	"racoo.cn/lsp/internal/store/redis"
)

const userProfileTTL = 7 * 24 * time.Hour

// UserProfile 是 handler 可见的玩家公开资料结构，与存储层实现解耦。
type UserProfile struct {
	Nickname string
}

// UserDirectory 管理玩家公开资料；Redis 不可用时退回进程内内存表。
type UserDirectory struct {
	mu    sync.RWMutex
	mem   map[string]UserProfile
	store *redis.Client
}

func NewUserDirectory(store *redis.Client) *UserDirectory {
	return &UserDirectory{mem: make(map[string]UserProfile), store: store}
}

func (d *UserDirectory) Set(ctx context.Context, userID string, profile UserProfile) error {
	if d == nil || userID == "" {
		return nil
	}
	d.mu.Lock()
	d.mem[userID] = profile
	d.mu.Unlock()
	if d.store != nil {
		return d.store.PutUserProfile(ctx, userID, redis.UserProfile{Nickname: profile.Nickname}, userProfileTTL)
	}
	return nil
}

func (d *UserDirectory) Get(ctx context.Context, userID string) (UserProfile, bool, error) {
	if d == nil || userID == "" {
		return UserProfile{}, false, nil
	}
	if d.store != nil {
		if rp, ok, err := d.store.GetUserProfile(ctx, userID); err != nil || ok {
			return UserProfile{Nickname: rp.Nickname}, ok, err
		}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	profile, ok := d.mem[userID]
	return profile, ok, nil
}
