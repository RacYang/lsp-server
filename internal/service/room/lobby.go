package room

import (
	"fmt"
	"sync"

	domainroom "racoo.cn/lsp/internal/domain/room"
)

// RoomRegistry 为本进程房间索引，避免与集群大厅服务混淆。
type RoomRegistry struct {
	mu    sync.RWMutex
	rooms map[string]*domainroom.Room
}

// Lobby 是 RoomRegistry 的兼容别名；新代码应优先使用 RoomRegistry。
type Lobby = RoomRegistry

// NewRoomRegistry 创建空房间索引。
func NewRoomRegistry() *RoomRegistry {
	return &RoomRegistry{rooms: make(map[string]*domainroom.Room)}
}

// NewLobby 创建空房间索引，保留旧名称以兼容现有调用点。
func NewLobby() *RoomRegistry {
	return NewRoomRegistry()
}

// CreateRoom 创建房间；roomID 由调用方生成。
func (l *RoomRegistry) CreateRoom(roomID string, rv *domainroom.Room) error {
	if l == nil {
		return fmt.Errorf("nil lobby")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.rooms[roomID]; ok {
		return fmt.Errorf("room exists: %s", roomID)
	}
	l.rooms[roomID] = rv
	return nil
}

// GetRoom 返回房间。
func (l *RoomRegistry) GetRoom(roomID string) (*domainroom.Room, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	rv, ok := l.rooms[roomID]
	return rv, ok
}
