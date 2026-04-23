package lobby

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrRoomNotFound 表示房间尚未创建或已被移除。
	ErrRoomNotFound = errors.New("room not found")
	// ErrRoomFull 表示房间 4 个座位已占满。
	ErrRoomFull = errors.New("room full")
	// ErrInvalidArgument 表示调用参数缺失。
	ErrInvalidArgument = errors.New("invalid argument")
)

// Service 为大厅服务：维护房间到 room 节点映射与简单座位分配。
type Service struct {
	mu      sync.Mutex
	roomIDs map[string]string
	seats   map[string]map[string]int32
}

// New 创建大厅服务实例。
func New() *Service {
	return &Service{
		roomIDs: make(map[string]string),
		seats:   make(map[string]map[string]int32),
	}
}

// CreateRoom 创建房间并绑定到 room-local；后续会由调度器/etcd claim 替换。
func (s *Service) CreateRoom(_ context.Context, roomID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("nil lobby service")
	}
	if roomID == "" {
		return "", fmt.Errorf("%w: empty room_id", ErrInvalidArgument)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if nodeID, ok := s.roomIDs[roomID]; ok {
		return nodeID, nil
	}
	s.roomIDs[roomID] = "room-local"
	s.seats[roomID] = make(map[string]int32)
	return "room-local", nil
}

// JoinRoom 为测试与基线阶段分配座位；重复加入返回原座位。
func (s *Service) JoinRoom(_ context.Context, roomID, userID string) (int32, error) {
	if s == nil {
		return 0, fmt.Errorf("nil lobby service")
	}
	if roomID == "" || userID == "" {
		return 0, fmt.Errorf("%w: empty room_id or user_id", ErrInvalidArgument)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roomIDs[roomID]; !ok {
		s.roomIDs[roomID] = "room-local"
		s.seats[roomID] = make(map[string]int32)
	}
	if seat, ok := s.seats[roomID][userID]; ok {
		return seat, nil
	}
	seatCount := len(s.seats[roomID])
	if seatCount >= 4 {
		return 0, ErrRoomFull
	}
	seat := int32(seatCount) //nolint:gosec // 最大仅 0..3，已由上方边界限制
	s.seats[roomID][userID] = seat
	return seat, nil
}

// GetRoom 查询房间归属节点。
func (s *Service) GetRoom(_ context.Context, roomID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("nil lobby service")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	nodeID, ok := s.roomIDs[roomID]
	if !ok {
		return "", ErrRoomNotFound
	}
	return nodeID, nil
}
