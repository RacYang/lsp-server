// Package room 提供房间应用服务：加入、准备与开局编排。
package room

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	domainroom "racoo.cn/lsp/internal/domain/room"
)

// Service 编排房间命令。
type Service struct {
	lobby *Lobby
	mu    sync.Mutex
}

// NewService 创建房间服务（广播由 handler 在写完应答帧后调用 Hub 完成）。
func NewService(l *Lobby) *Service {
	return &Service{lobby: l}
}

// EnsureRoom 若不存在则创建房间。
func (s *Service) EnsureRoom(roomID string) error {
	if s == nil {
		return fmt.Errorf("nil service")
	}
	if _, ok := s.lobby.GetRoom(roomID); ok {
		return nil
	}
	r := domainroom.NewRoom(roomID)
	return s.lobby.CreateRoom(roomID, r)
}

// Join 自动占座并返回座位号。
func (s *Service) Join(ctx context.Context, roomID, userID string) (int, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.EnsureRoom(roomID); err != nil {
		return -1, err
	}
	r, ok := s.lobby.GetRoom(roomID)
	if !ok {
		return -1, fmt.Errorf("room missing: %s", roomID)
	}
	seat, ok := r.JoinAutoSeat(userID)
	if !ok {
		return -1, fmt.Errorf("room full")
	}
	return seat, nil
}

// Ready 标记准备并尝试开局。
// 返回值：非空载荷表示须在调用方写完准备应答帧之后再调用 Hub.Broadcast，避免与同一
// WebSocket 连接上的其它写操作并发（gorilla/websocket 要求单写者）。
func (s *Service) Ready(ctx context.Context, roomID, userID string) ([]byte, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.lobby.GetRoom(roomID)
	if !ok {
		return nil, fmt.Errorf("room not found")
	}
	seat := -1
	for i := 0; i < 4; i++ {
		if r.PlayerIDs[i] == userID {
			seat = i
			break
		}
	}
	if seat < 0 {
		return nil, fmt.Errorf("not in room")
	}
	if err := r.SetReady(seat, true); err != nil {
		return nil, err
	}
	if r.FSM.State() == domainroom.StateReady {
		if err := r.StartPlaying(); err != nil {
			return nil, err
		}
		// Phase 1：开局后立即推送简化结算，用于打通端到端链路；完整血战流程后续迭代补齐。
		var settlement []byte
		env := &clientv1.Envelope{ReqId: "ready", Body: &clientv1.Envelope_Settlement{
			Settlement: &clientv1.SettlementNotify{RoomId: roomID, TotalFan: 0},
		}}
		if b, err := proto.Marshal(env); err == nil {
			settlement = b
		}
		_ = r.CloseToSettling()
		_ = r.CloseRoom()
		return settlement, nil
	}
	return nil, nil
}

// NewUserID 生成用户 ID（登录用）。
func NewUserID() string {
	return uuid.NewString()
}
