// 本文件提供 WebSocket 出站写的进程级串行化，配合 Hub 广播与 handler 单连接写。
package session

import (
	"sync"

	"github.com/gorilla/websocket"
)

// 全局写互斥：gorilla/websocket 要求同一连接上不得并发 WriteMessage，
// 而广播与各自 handler 可能同时命中同一连接，故统一串行化出站写。
var wsWriteMu sync.Mutex

// WriteBinary 以线程安全方式发送二进制 WebSocket 帧。
func WriteBinary(c *websocket.Conn, data []byte) error {
	if c == nil {
		return nil
	}
	wsWriteMu.Lock()
	defer wsWriteMu.Unlock()
	return c.WriteMessage(websocket.BinaryMessage, data)
}
