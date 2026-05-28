// Package session 除 Hub 外，Phase 3 起提供可选的 Redis 会话管理器，供 gate 登录与重连使用。
package session

import (
	"context"
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/google/uuid"

	"racoo.cn/lsp/internal/store/redis"
)

const defaultSessionTTL = 30 * time.Minute

// sessionStore 是会话持久化的最小接口，仅使用原语类型，不直接暴露 redis 包类型。
type sessionStore interface {
	SaveSessionWithPlainToken(ctx context.Context, userID, plainToken string, sessionVer int64, ttl time.Duration) error
	ResolveUserIDByPlainToken(ctx context.Context, plainToken string) (userID string, found bool, err error)
	GetSession(ctx context.Context, userID string) (roomID, lastCursor, tokenHash string, sessionVer int64, found bool, err error)
	PutSession(ctx context.Context, userID, roomID, lastCursor, tokenHash string, sessionVer int64, ttl time.Duration) error
}

// Manager 封装会话令牌与持久化；store 为 nil 时所有方法为无操作成功路径。
type Manager struct {
	store sessionStore
}

// NewManager 创建会话管理器；c 可为 nil（表示禁用 Redis 会话）。
// 内部通过 redisSessionAdapter 将 *redis.Client 适配为 sessionStore 接口。
func NewManager(c *redis.Client) *Manager {
	if c == nil {
		return &Manager{}
	}
	return &Manager{store: &redisSessionAdapter{c: c}}
}

// Issue 为新用户签发不透明令牌并写入后端；会话只绑定用户，不绑定具体 gate 副本。
func (m *Manager) Issue(ctx context.Context, userID string) (plainToken string, err error) {
	if m == nil || m.store == nil {
		return "", nil
	}
	sessionVer := int64(1)
	plain := formatSessionToken(sessionVer, uuid.NewString()+"."+uuid.NewString())
	if err := m.store.SaveSessionWithPlainToken(ctx, userID, plain, sessionVer, defaultSessionTTL); err != nil {
		return "", err
	}
	return plain, nil
}

// Record 为重连解析后的会话视图（handler 层使用，避免直接依赖存储类型）。
type Record struct {
	RoomID     string
	LastCursor string
	TokenHash  string
	SessionVer int64
}

// Resume 校验明文令牌并返回 user_id 与会话字段。
func (m *Manager) Resume(ctx context.Context, plainToken string) (userID string, rec Record, err error) {
	if m == nil || m.store == nil {
		return "", Record{}, fmt.Errorf("会话恢复未启用")
	}
	uid, ok, err := m.store.ResolveUserIDByPlainToken(ctx, plainToken)
	if err != nil || !ok {
		return "", Record{}, fmt.Errorf("无效或过期的会话令牌")
	}
	roomID, lastCursor, tokenHash, sessionVer, ok, err := m.store.GetSession(ctx, uid)
	if err != nil || !ok {
		return "", Record{}, fmt.Errorf("会话记录不存在")
	}
	// 使用时序恒定比较，避免基于 hash 字符串前缀的 timing oracle（防御纵深）。
	if subtle.ConstantTimeCompare([]byte(tokenHash), []byte(hashSessionToken(plainToken))) != 1 {
		return "", Record{}, fmt.Errorf("会话令牌校验失败")
	}
	tokenVer, ok := parseSessionTokenVersion(plainToken)
	if !ok || tokenVer != sessionVer {
		return "", Record{}, fmt.Errorf("会话版本校验失败")
	}
	return uid, Record{
		RoomID:     roomID,
		LastCursor: lastCursor,
		TokenHash:  tokenHash,
		SessionVer: sessionVer,
	}, nil
}

// BindRoom 将会话绑定到房间号。
func (m *Manager) BindRoom(ctx context.Context, userID, roomID string) error {
	if m == nil || m.store == nil || roomID == "" || userID == "" {
		return nil
	}
	_, lastCursor, tokenHash, sessionVer, ok, err := m.store.GetSession(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("会话不存在无法绑定房间")
	}
	return m.store.PutSession(ctx, userID, roomID, lastCursor, tokenHash, sessionVer, defaultSessionTTL)
}

// UnbindRoom 清空会话绑定的房间号；离房成功后由 gate 调用。
func (m *Manager) UnbindRoom(ctx context.Context, userID string) error {
	if m == nil || m.store == nil || userID == "" {
		return nil
	}
	_, lastCursor, tokenHash, sessionVer, ok, err := m.store.GetSession(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return m.store.PutSession(ctx, userID, "", lastCursor, tokenHash, sessionVer, defaultSessionTTL)
}

// UpdateCursor 更新用户会话中最后收到的房间事件游标。
func (m *Manager) UpdateCursor(ctx context.Context, userID, cursor string) error {
	if m == nil || m.store == nil || userID == "" || cursor == "" {
		return nil
	}
	roomID, _, tokenHash, sessionVer, ok, err := m.store.GetSession(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return m.store.PutSession(ctx, userID, roomID, cursor, tokenHash, sessionVer, defaultSessionTTL)
}

// redisSessionAdapter 将 *redis.Client 适配为 sessionStore 接口。
type redisSessionAdapter struct {
	c *redis.Client
}

func (a *redisSessionAdapter) SaveSessionWithPlainToken(ctx context.Context, userID, plainToken string, sessionVer int64, ttl time.Duration) error {
	return a.c.SaveSessionWithPlainToken(ctx, userID, plainToken, redis.SessionRecord{SessionVer: sessionVer}, ttl)
}

func (a *redisSessionAdapter) ResolveUserIDByPlainToken(ctx context.Context, plainToken string) (string, bool, error) {
	return a.c.ResolveUserIDByPlainToken(ctx, plainToken)
}

func (a *redisSessionAdapter) GetSession(ctx context.Context, userID string) (string, string, string, int64, bool, error) {
	srec, ok, err := a.c.GetSession(ctx, userID)
	return srec.RoomID, srec.LastCursor, srec.TokenHash, srec.SessionVer, ok, err
}

func (a *redisSessionAdapter) PutSession(ctx context.Context, userID, roomID, lastCursor, tokenHash string, sessionVer int64, ttl time.Duration) error {
	return a.c.PutSession(ctx, userID, redis.SessionRecord{
		RoomID:     roomID,
		LastCursor: lastCursor,
		TokenHash:  tokenHash,
		SessionVer: sessionVer,
	}, ttl)
}
