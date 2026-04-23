package room

import (
	"fmt"
	"sync"

	domainroom "racoo.cn/lsp/internal/domain/room"
)

// Lobby 为内存大厅索引：创建、查询房间（Phase 1 不做持久化）。
type Lobby struct {
	mu    sync.RWMutex
	rooms map[string]*domainroom.Room
}

// NewLobby 创建空大厅。
func NewLobby() *Lobby {
	return &Lobby{rooms: make(map[string]*domainroom.Room)}
}

// CreateRoom 创建房间；roomID 由调用方生成。
func (l *Lobby) CreateRoom(roomID string, rv *domainroom.Room) error {
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
func (l *Lobby) GetRoom(roomID string) (*domainroom.Room, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	rv, ok := l.rooms[roomID]
	return rv, ok
}
