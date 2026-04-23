// Package session 管理连接注册与房间级广播（Phase 1 内存实现）。
package session

import (
	"sync"

	"github.com/gorilla/websocket"

	"racoo.cn/lsp/internal/net/frame"
)

// Hub 保存 user_id 到连接的映射，并提供按房间广播。
type Hub struct {
	mu    sync.Mutex
	users map[string]*websocket.Conn
	rooms map[string]map[string]struct{}
}

// NewHub 创建 Hub。
func NewHub() *Hub {
	return &Hub{
		users: make(map[string]*websocket.Conn),
		rooms: make(map[string]map[string]struct{}),
	}
}

// Register 注册连接并记录其所在房间。
func (h *Hub) Register(userID, roomID string, c *websocket.Conn) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.users[userID] = c
	if h.rooms[roomID] == nil {
		h.rooms[roomID] = make(map[string]struct{})
	}
	h.rooms[roomID][userID] = struct{}{}
}

// Broadcast 将 msg_id 与载荷编码为完整帧后向房间内所有用户推送。
func (h *Hub) Broadcast(roomID string, msgID uint16, payload []byte) {
	if h == nil {
		return
	}
	fr := frame.Encode(msgID, payload)
	h.mu.Lock()
	defer h.mu.Unlock()
	for uid := range h.rooms[roomID] {
		if c := h.users[uid]; c != nil {
			_ = WriteBinary(c, fr)
		}
	}
}
