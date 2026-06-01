package reconnect

import (
	"context"
	"fmt"

	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
)

// ReconnectResult 是断线重连服务层结果，不含 proto 类型。
// 调用方（adapter/local）负责将其投影为 contract.ResumeResult。
type ReconnectResult struct {
	UserID     string
	RoomID     string
	Resumed    bool
	Players    []string
	State      string
	LastCursor string
	View       roomsvc.RoundView
}

// Service 处理断线重连业务逻辑：会话校验与内存房间状态读取。
type Service struct {
	rooms roomsvc.RoomQueries
	sess  *session.Manager
}

// New 创建重连服务。rooms 为 nil 时 Resume 始终返回错误。
func New(rooms roomsvc.RoomQueries, sess *session.Manager) *Service {
	return &Service{rooms: rooms, sess: sess}
}

// Resume 校验会话并读取内存房间快照，返回服务层重连结果。
// 不含 proto 投影，proto 投影由 adapter/local 完成。
func (s *Service) Resume(ctx context.Context, sessionToken string) (*ReconnectResult, error) {
	if s == nil || s.rooms == nil {
		return nil, fmt.Errorf("nil 重连服务")
	}
	if s.sess == nil {
		return nil, fmt.Errorf("会话管理器未启用")
	}
	uid, srec, err := s.sess.Resume(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	if srec.RoomID == "" {
		return &ReconnectResult{UserID: uid, Resumed: false}, nil
	}
	players, state, _, ok := s.rooms.RoomSnapshot(srec.RoomID)
	if !ok {
		return nil, fmt.Errorf("房间不存在或已回收")
	}
	view, _, _ := s.rooms.RoundView(ctx, srec.RoomID)
	return &ReconnectResult{
		UserID:     uid,
		RoomID:     srec.RoomID,
		Resumed:    true,
		Players:    players,
		State:      state,
		LastCursor: srec.LastCursor,
		View:       view,
	}, nil
}
