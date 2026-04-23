// Hub 基础测试：创建与空操作安全。
package session

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"racoo.cn/lsp/internal/net/frame"
)

func TestHubNilBroadcast(t *testing.T) {
	var h *Hub
	h.Broadcast("r1", 1, []byte{1})
}

func TestHubNilRegister(t *testing.T) {
	var h *Hub
	h.Register("u", "r", nil)
}

func TestNewHub(t *testing.T) {
	h := NewHub()
	if h == nil {
		t.Fatal("nil hub")
	}
}

func TestHubBroadcastTwoClients(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	var srvMu sync.Mutex
	var serverConns []*websocket.Conn

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		srvMu.Lock()
		serverConns = append(serverConns, c)
		srvMu.Unlock()
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	dial := func() *websocket.Conn {
		t.Helper()
		u := "ws" + strings.TrimPrefix(srv.URL, "http")
		c, resp, err := websocket.DefaultDialer.Dial(u, nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp != nil {
			if cerr := resp.Body.Close(); cerr != nil {
				t.Fatalf("关闭握手响应体失败: %v", cerr)
			}
		}
		return c
	}

	ca := dial()
	cb := dial()
	defer func() { _ = ca.Close() }()
	defer func() { _ = cb.Close() }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		srvMu.Lock()
		n := len(serverConns)
		srvMu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("超时未建立两条服务端连接")
		}
		time.Sleep(5 * time.Millisecond)
	}

	srvMu.Lock()
	sc0, sc1 := serverConns[0], serverConns[1]
	srvMu.Unlock()

	h := NewHub()
	h.Register("a", "room1", sc0)
	h.Register("b", "room1", sc1)

	want := frame.Encode(9, []byte{1, 2, 3})
	h.Broadcast("room1", 9, []byte{1, 2, 3})

	_ = ca.SetReadDeadline(time.Now().Add(2 * time.Second))
	_ = cb.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, gotA, err := ca.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	_, gotB, err := cb.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(gotA) != string(want) || string(gotB) != string(want) {
		t.Fatalf("帧不一致 lenA=%d lenB=%d", len(gotA), len(gotB))
	}
}
