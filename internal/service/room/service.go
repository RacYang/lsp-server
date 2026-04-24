// Package room 提供房间应用服务：加入、准备与开局编排。
package room

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	domainroom "racoo.cn/lsp/internal/domain/room"
)

// Service 编排房间命令；每房间在内部通过 roomActor 单协程串行化变更。
type Service struct {
	lobby  *Lobby
	mu     sync.Mutex
	actors map[string]*roomActor
	engine *Engine
}

// NewService 创建房间服务（广播由 handler 在写完应答帧后调用 Hub 完成）。
func NewService(l *Lobby) *Service {
	return NewServiceWithRule(l, "")
}

// NewServiceWithRule 使用指定规则装配房间服务；ruleID 为空时回退默认四川血战规则。
func NewServiceWithRule(l *Lobby, ruleID string) *Service {
	return &Service{
		lobby:  l,
		actors: make(map[string]*roomActor),
		engine: NewEngine(ruleID),
	}
}

// EnsureRoom 若不存在则创建房间并启动该房的 mailbox 协程。
func (s *Service) EnsureRoom(roomID string) error {
	if s == nil {
		return fmt.Errorf("nil service")
	}
	if _, ok := s.lobby.GetRoom(roomID); ok {
		s.ensureActorForExistingRoom(roomID)
		return nil
	}
	r := domainroom.NewRoom(roomID)
	if err := s.lobby.CreateRoom(roomID, r); err != nil {
		// 并发首进房时，另一协程可能已经抢先建好了房；此时回读并补 actor 即可。
		if _, ok := s.lobby.GetRoom(roomID); ok {
			s.ensureActorForExistingRoom(roomID)
			return nil
		}
		return err
	}
	s.startActorLocked(roomID, r)
	return nil
}

func (s *Service) startActorLocked(roomID string, r *domainroom.Room) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.actors[roomID]; ok {
		return
	}
	a := newRoomActor(r)
	a.engine = s.engine
	a.onExit = s.removeActor
	s.actors[roomID] = a
	go a.run()
}

func (s *Service) ensureActorForExistingRoom(roomID string) {
	s.mu.Lock()
	if _, ok := s.actors[roomID]; ok {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	r, ok := s.lobby.GetRoom(roomID)
	if !ok {
		return
	}
	s.mu.Lock()
	if _, ok := s.actors[roomID]; ok {
		s.mu.Unlock()
		return
	}
	a := newRoomActor(r)
	a.engine = s.engine
	a.onExit = s.removeActor
	s.actors[roomID] = a
	s.mu.Unlock()
	go a.run()
}

func (s *Service) removeActor(roomID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.actors, roomID)
}

func (s *Service) getActor(roomID string) *roomActor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.actors[roomID]
}

// Join 自动占座并返回座位号。
func (s *Service) Join(ctx context.Context, roomID, userID string) (int, error) {
	if err := s.EnsureRoom(roomID); err != nil {
		return -1, err
	}
	a := s.getActor(roomID)
	if a == nil {
		return -1, fmt.Errorf("room missing: %s", roomID)
	}
	return a.submitJoin(ctx, userID)
}

// Ready 标记准备并尝试开局。
// 返回值：非空载荷表示须在调用方写完准备应答帧之后再调用 Hub.Broadcast，避免与同一
// WebSocket 连接上的其它写操作并发（gorilla/websocket 要求单写者）。
func (s *Service) Ready(ctx context.Context, roomID, userID string) ([]Notification, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, fmt.Errorf("room not found")
	}
	return a.submitReady(ctx, userID)
}

// RecoverRoom 基于 Redis snapmeta 恢复房间基础内存态，并重新挂起 actor。
func (s *Service) RecoverRoom(roomID string, playerIDs []string, fsmState string) error {
	if s == nil || s.lobby == nil {
		return fmt.Errorf("nil service")
	}
	if roomID == "" {
		return fmt.Errorf("empty room_id")
	}
	if _, ok := s.lobby.GetRoom(roomID); ok {
		s.ensureActorForExistingRoom(roomID)
		return nil
	}
	r := domainroom.NewRoom(roomID)
	for _, userID := range playerIDs {
		if userID == "" {
			continue
		}
		if _, ok := r.JoinAutoSeat(userID); !ok {
			return fmt.Errorf("recover room %s: room full", roomID)
		}
	}
	switch domainroom.State(fsmState) {
	case "", domainroom.StateWaiting:
	case domainroom.StateReady:
		if err := r.FSM.Transition(domainroom.StateReady); err != nil {
			return err
		}
	case domainroom.StatePlaying:
		if err := r.FSM.Transition(domainroom.StateReady); err != nil {
			return err
		}
		if err := r.FSM.Transition(domainroom.StatePlaying); err != nil {
			return err
		}
	case domainroom.StateSettling:
		if err := r.FSM.Transition(domainroom.StateReady); err != nil {
			return err
		}
		if err := r.FSM.Transition(domainroom.StatePlaying); err != nil {
			return err
		}
		if err := r.FSM.Transition(domainroom.StateSettling); err != nil {
			return err
		}
	case domainroom.StateClosed:
		if err := r.FSM.Transition(domainroom.StateClosed); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown room state: %s", fsmState)
	}
	if err := s.lobby.CreateRoom(roomID, r); err != nil {
		return err
	}
	s.startActorLocked(roomID, r)
	return nil
}

// RoomSnapshot 返回当前内存房间的玩家列表与 FSM 状态字符串，供快照与 Redis 元数据写入。
func (s *Service) RoomSnapshot(roomID string) (playerIDs []string, fsmState string, ok bool) {
	if s == nil || s.lobby == nil {
		return nil, "", false
	}
	r, ok := s.lobby.GetRoom(roomID)
	if !ok || r == nil {
		return nil, "", false
	}
	out := make([]string, 0, 4)
	for _, id := range r.PlayerIDs {
		if id != "" {
			out = append(out, id)
		}
	}
	st := ""
	if r.FSM != nil {
		st = string(r.FSM.State())
	}
	return out, st, true
}

// RuleID 返回当前房间服务使用的规则 ID，供持久化摘要写入。
func (s *Service) RuleID() string {
	if s == nil || s.engine == nil {
		return ""
	}
	return s.engine.ruleID
}

// NewUserID 生成用户 ID（登录用）。
func NewUserID() string {
	return uuid.NewString()
}
