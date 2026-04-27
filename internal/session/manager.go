// Package session 除 Hub 外，Phase 3 起提供可选的 Redis 会话管理器，供 gate 登录与重连使用。
package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"racoo.cn/lsp/internal/store/redis"
)

const defaultSessionTTL = 30 * time.Minute

// Manager 封装会话令牌与 Redis 持久化；c 为空时所有方法为无操作成功路径。
type Manager struct {
	c *redis.Client
}

// NewManager 创建会话管理器；c 可为 nil（表示禁用 Redis 会话）。
func NewManager(c *redis.Client) *Manager {
	return &Manager{c: c}
}

// Issue 为新用户签发不透明令牌并写入 Redis。
func (m *Manager) Issue(ctx context.Context, userID, gateAdvertiseAddr string) (plainToken string, err error) {
	if m == nil || m.c == nil {
		return "", nil
	}
	sessionVer := int64(1)
	plain := redis.FormatSessionToken(sessionVer, uuid.NewString()+"."+uuid.NewString())
	rec := redis.SessionRecord{
		GateNodeID:    "gate",
		AdvertiseAddr: gateAdvertiseAddr,
		SessionVer:    sessionVer,
	}
	if err := m.c.SaveSessionWithPlainToken(ctx, userID, plain, rec, defaultSessionTTL); err != nil {
		return "", err
	}
	return plain, nil
}

// Record 为重连解析后的会话视图（handler 层使用，避免直接依赖 Redis 类型）。
type Record struct {
	RoomID        string
	LastCursor    string
	TokenHash     string
	SessionVer    int64
	AdvertiseAddr string
}

// Resume 校验明文令牌并返回 user_id 与会话字段。
func (m *Manager) Resume(ctx context.Context, plainToken string) (userID string, rec Record, err error) {
	if m == nil || m.c == nil {
		return "", Record{}, fmt.Errorf("会话恢复未启用")
	}
	uid, ok, err := m.c.ResolveUserIDByPlainToken(ctx, plainToken)
	if err != nil || !ok {
		return "", Record{}, fmt.Errorf("无效或过期的会话令牌")
	}
	srec, ok, err := m.c.GetSession(ctx, uid)
	if err != nil || !ok {
		return "", Record{}, fmt.Errorf("会话记录不存在")
	}
	if srec.TokenHash != redis.HashSessionToken(plainToken) {
		return "", Record{}, fmt.Errorf("会话令牌校验失败")
	}
	tokenVer, ok := redis.ParseSessionTokenVersion(plainToken)
	if !ok || tokenVer != srec.SessionVer {
		return "", Record{}, fmt.Errorf("会话版本校验失败")
	}
	return uid, Record{
		RoomID:        srec.RoomID,
		LastCursor:    srec.LastCursor,
		TokenHash:     srec.TokenHash,
		SessionVer:    srec.SessionVer,
		AdvertiseAddr: srec.AdvertiseAddr,
	}, nil
}

// BindRoom 将会话绑定到房间号。
func (m *Manager) BindRoom(ctx context.Context, userID, roomID string) error {
	if m == nil || m.c == nil || roomID == "" || userID == "" {
		return nil
	}
	srec, ok, err := m.c.GetSession(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("会话不存在无法绑定房间")
	}
	srec.RoomID = roomID
	return m.c.PutSession(ctx, userID, srec, defaultSessionTTL)
}

// UnbindRoom 清空会话绑定的房间号；离房成功后由 gate 调用。
func (m *Manager) UnbindRoom(ctx context.Context, userID string) error {
	if m == nil || m.c == nil || userID == "" {
		return nil
	}
	srec, ok, err := m.c.GetSession(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	srec.RoomID = ""
	return m.c.PutSession(ctx, userID, srec, defaultSessionTTL)
}

// UpdateCursor 更新用户会话中最后收到的房间事件游标。
func (m *Manager) UpdateCursor(ctx context.Context, userID, cursor string) error {
	if m == nil || m.c == nil || userID == "" || cursor == "" {
		return nil
	}
	srec, ok, err := m.c.GetSession(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	srec.LastCursor = cursor
	return m.c.PutSession(ctx, userID, srec, defaultSessionTTL)
}
