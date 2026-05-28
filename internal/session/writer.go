// 本文件提供每连接一个出站写协程与有界队列，避免全局互斥成为 gate 热点。
package session

import (
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

// Conn 是连接的最小抽象接口，与 *websocket.Conn 解耦；
// *websocket.Conn 开箱即满足本接口，测试可直接提供 fakeConn。
type Conn interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// 默认队列长度需覆盖一局自动回放的 burst 推送（换三张/定缺/若干摸打/结算），
// 同时保留一定余量给 ready/login 等应答帧。
const defaultWriteQueueSize = 128

type connWriter struct {
	conn Conn
	ch   chan []byte
	once sync.Once
}

var writerRegistry sync.Map

func newConnWriter(c Conn) *connWriter {
	return &connWriter{
		conn: c,
		ch:   make(chan []byte, defaultWriteQueueSize),
	}
}

func (cw *connWriter) run() {
	for data := range cw.ch {
		if err := cw.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			break
		}
	}
	cw.once.Do(func() {
		writerRegistry.Delete(cw.conn)
		_ = cw.conn.Close()
	})
}

func getConnWriter(c Conn) *connWriter {
	if c == nil {
		return nil
	}
	if v, ok := writerRegistry.Load(c); ok {
		return v.(*connWriter)
	}
	cw := newConnWriter(c)
	actual, loaded := writerRegistry.LoadOrStore(c, cw)
	if loaded {
		return actual.(*connWriter)
	}
	go cw.run()
	return cw
}

// WriteBinary 将完整帧放入连接专属写队列；队列满则返回错误给调用方决定是否丢弃。
func WriteBinary(c Conn, data []byte) error {
	if c == nil {
		return nil
	}
	cw := getConnWriter(c)
	if cw == nil {
		return nil
	}
	cp := append([]byte(nil), data...)
	select {
	case cw.ch <- cp:
		return nil
	default:
		return fmt.Errorf("session write queue full")
	}
}

// CloseConn 关闭连接并回收其写协程；重复调用安全。
func CloseConn(c Conn) error {
	if c == nil {
		return nil
	}
	if v, ok := writerRegistry.Load(c); ok {
		cw := v.(*connWriter)
		cw.once.Do(func() {
			writerRegistry.Delete(c)
			close(cw.ch)
		})
		return nil
	}
	return c.Close()
}
