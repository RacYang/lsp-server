//nolint:staticcheck // CLI 连接层当前与 loadgen 一致，使用 nhooyr.io/websocket。
package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"nhooyr.io/websocket"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/protocol"
)

func TestTokenFileRoundTrip(t *testing.T) {
	// 会话令牌需要能稳定落盘，便于断线后重新登录恢复。
	path := t.TempDir() + "/session.token"
	require.NoError(t, writeToken(path, "tok-1"))
	require.Equal(t, "tok-1", readToken(path))
}

func TestDialOptionsOriginAndWSS(t *testing.T) {
	// 云端反向代理常用 Origin 白名单和 wss，自签证书只允许显式调试开关跳过。
	c := NewWSClient("wss://example.test/ws", "n", "", "https://cli.example", true, NewAppState("n"))
	opts, err := c.dialOptions()
	require.NoError(t, err)
	require.Equal(t, "https://cli.example", opts.HTTPHeader.Get("Origin"))
	require.NotNil(t, opts.HTTPClient)
}

func TestConnectOnceSendsLoginAndReadsResponse(t *testing.T) {
	// 该用例只跑一次连接循环：客户端发登录帧，测试服务端回登录响应后关闭连接。
	tokenPath := t.TempDir() + "/session.token"
	gotOrigin := ""
	errCh := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrigin = r.Header.Get("Origin")
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()
		_, data, err := conn.Read(r.Context())
		if err != nil {
			errCh <- err
			return
		}
		h, err := protocol.ReadFrame(bytes.NewReader(data))
		if err != nil {
			errCh <- err
			return
		}
		if h.MsgID != protocol.LoginReq {
			errCh <- fmt.Errorf("unexpected msg_id: %d", h.MsgID)
			return
		}
		var req clientv1.Envelope
		if err := proto.Unmarshal(h.Payload, &req); err != nil {
			errCh <- err
			return
		}
		if req.GetLoginReq().GetNickname() != "测试玩家" {
			errCh <- fmt.Errorf("unexpected nickname: %s", req.GetLoginReq().GetNickname())
			return
		}
		resp, err := proto.Marshal(&clientv1.Envelope{
			ReqId: "login-resp",
			Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
				UserId:       "u1",
				SessionToken: "tok-new",
			}},
		})
		if err != nil {
			errCh <- err
			return
		}
		enc, _ := protocol.Encode(protocol.LoginResp, resp)
		if err := conn.Write(r.Context(), websocket.MessageBinary, enc); err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	state := NewAppState("测试玩家")
	client := NewWSClient(wsURL, "测试玩家", tokenPath, "https://origin.example", false, state)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := client.connectOnce(ctx)
	require.Error(t, err)
	require.NoError(t, <-errCh)
	require.Equal(t, "https://origin.example", gotOrigin)
	select {
	case env := <-client.Events():
		require.Equal(t, "u1", env.GetLoginResp().GetUserId())
	case <-time.After(time.Second):
		t.Fatal("未收到登录响应事件")
	}
	require.Equal(t, "tok-new", readToken(tokenPath))
}

func TestConnectOnceReturnsRouteRedirectError(t *testing.T) {
	errCh := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()
		if _, _, err := conn.Read(r.Context()); err != nil {
			errCh <- err
			return
		}
		redirect, err := proto.Marshal(&clientv1.Envelope{
			ReqId: "route",
			Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{
				WsUrl:  "wss://next.example/ws",
				Reason: "迁移入口",
			}},
		})
		if err != nil {
			errCh <- err
			return
		}
		enc, _ := protocol.Encode(protocol.RouteRedirectNotify, redirect)
		if err := conn.Write(r.Context(), websocket.MessageBinary, enc); err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := NewWSClient(wsURL, "测试玩家", "", "", false, NewAppState("测试玩家"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := client.connectOnce(ctx)
	var redirectErr *routeRedirectError
	require.ErrorAs(t, err, &redirectErr)
	require.Equal(t, "wss://next.example/ws", redirectErr.url)
	require.NoError(t, <-errCh)
	select {
	case env := <-client.Events():
		require.Equal(t, "wss://next.example/ws", env.GetRouteRedirect().GetWsUrl())
	case <-time.After(time.Second):
		t.Fatal("未收到路由重定向事件")
	}
}

func TestRunAppliesRouteRedirectWithoutBackoff(t *testing.T) {
	calls := make(chan string, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()
		calls <- r.URL.String()
		if _, _, err := conn.Read(r.Context()); err != nil {
			return
		}
		redirect, _ := proto.Marshal(&clientv1.Envelope{
			Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{
				WsUrl: "ws://" + r.Host + "/ws-next",
			}},
		})
		enc, _ := protocol.Encode(protocol.RouteRedirectNotify, redirect)
		_ = conn.Write(r.Context(), websocket.MessageBinary, enc)
	}))
	defer srv.Close()

	initialURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	client := NewWSClient(initialURL, "测试玩家", "", "", false, NewAppState("测试玩家"))
	redirected := make(chan string, 10)
	client.SetRedirectHandler(func(url string) error {
		redirected <- url
		client.SetConfig(url, "")
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)

	select {
	case url := <-redirected:
		require.Contains(t, url, "/ws-next")
	case <-time.After(2 * time.Second):
		t.Fatal("未触发重定向")
	}
}
