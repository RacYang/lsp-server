// Package session 管理连接注册与房间级广播（Phase 1 内存实现）。
package session

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"racoo.cn/lsp/internal/clock"
)

// Hub 保存 user_id 到连接的映射，并提供按房间广播。
type Hub struct {
	mu               sync.Mutex
	users            map[string]*websocket.Conn
	rooms            map[string]map[string]struct{}
	lastHeartbeat    map[string]time.Time
	clk              clock.Clock
	heartbeatTimeout time.Duration
}

// NewHub 创建 Hub。
func NewHub() *Hub {
	return &Hub{
		users:            make(map[string]*websocket.Conn),
		rooms:            make(map[string]map[string]struct{}),
		lastHeartbeat:    make(map[string]time.Time),
		clk:              clock.NewReal(),
		heartbeatTimeout: 60 * time.Second,
	}
}

// SetClock 注入时间源，主要供心跳测试使用。
func (h *Hub) SetClock(c clock.Clock) {
	if h == nil || c == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clk = c
}

// SetHeartbeatTimeout 设置心跳超时；非正值关闭超时淘汰。
func (h *Hub) SetHeartbeatTimeout(d time.Duration) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.heartbeatTimeout = d
}

// Register 注册连接并记录其所在房间。
func (h *Hub) Register(userID, roomID string, c *websocket.Conn) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.users[userID] = c
	h.lastHeartbeat[userID] = h.clk.Now()
	if h.rooms[roomID] == nil {
		h.rooms[roomID] = make(map[string]struct{})
	}
	h.rooms[roomID][userID] = struct{}{}
}

// Broadcast 将已编码好的完整帧广播给房间内所有用户。
func (h *Hub) Broadcast(roomID string, encoded []byte) {
	_ = h.BroadcastDeliveredUsers(roomID, encoded)
}

// BroadcastDeliveredUsers 广播并返回成功入队的用户 ID；失败连接会从房间注册表中移除。
func (h *Hub) BroadcastDeliveredUsers(roomID string, encoded []byte) []string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delivered := make([]string, 0, len(h.rooms[roomID]))
	for uid := range h.rooms[roomID] {
		if c := h.users[uid]; c != nil {
			if err := WriteBinary(c, encoded); err == nil {
				delivered = append(delivered, uid)
				continue
			}
		}
		delete(h.rooms[roomID], uid)
		delete(h.users, uid)
		delete(h.lastHeartbeat, uid)
	}
	if len(h.rooms[roomID]) == 0 {
		delete(h.rooms, roomID)
	}
	return delivered
}

// IterRoomUsers 遍历某房间内已注册的用户 ID；fn 在持锁期间调用，勿执行阻塞或再次获取 Hub 锁。
func (h *Hub) IterRoomUsers(roomID string, fn func(userID string)) {
	if h == nil || fn == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for uid := range h.rooms[roomID] {
		fn(uid)
	}
}

// Unregister 删除某用户与房间的连接映射；用于离房或连接关闭后的清理。
func (h *Hub) Unregister(userID, roomID string) {
	if h == nil || userID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.users, userID)
	delete(h.lastHeartbeat, userID)
	if roomID == "" {
		return
	}
	if h.rooms[roomID] != nil {
		delete(h.rooms[roomID], userID)
		if len(h.rooms[roomID]) == 0 {
			delete(h.rooms, roomID)
		}
	}
}

// TouchHeartbeat 记录用户最近一次心跳时间。
func (h *Hub) TouchHeartbeat(userID string) {
	if h == nil || userID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastHeartbeat[userID] = h.clk.Now()
}

// CloseExpiredHeartbeats 关闭超过心跳阈值的连接，但不改变房间业务状态。
func (h *Hub) CloseExpiredHeartbeats() []string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.heartbeatTimeout <= 0 {
		return nil
	}
	now := h.clk.Now()
	var closed []string
	for uid, last := range h.lastHeartbeat {
		if now.Sub(last) <= h.heartbeatTimeout {
			continue
		}
		if c := h.users[uid]; c != nil {
			_ = c.Close()
		}
		delete(h.users, uid)
		delete(h.lastHeartbeat, uid)
		for roomID, users := range h.rooms {
			delete(users, uid)
			if len(users) == 0 {
				delete(h.rooms, roomID)
			}
		}
		closed = append(closed, uid)
	}
	return closed
}
