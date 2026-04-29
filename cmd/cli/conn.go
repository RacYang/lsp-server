//nolint:staticcheck // ADR-0025 既有客户端路径使用 nhooyr.io/websocket；CLI 先保持同栈复用。
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"nhooyr.io/websocket"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
)

type WSClient struct {
	wsURL              string
	origin             string
	insecureSkipVerify bool
	tokenFile          string
	name               string

	mu     sync.Mutex
	conn   *websocket.Conn
	events chan *clientv1.Envelope
	state  *AppState
}

func NewWSClient(wsURL string, name string, tokenFile string, origin string, insecureSkipVerify bool, state *AppState) *WSClient {
	return &WSClient{
		wsURL:              wsURL,
		name:               name,
		tokenFile:          tokenFile,
		origin:             origin,
		insecureSkipVerify: insecureSkipVerify,
		events:             make(chan *clientv1.Envelope, 128),
		state:              state,
	}
}

func (c *WSClient) Events() <-chan *clientv1.Envelope {
	return c.events
}

func (c *WSClient) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		if err := c.connectOnce(ctx); err != nil {
			c.state.Mutate(func(v *RoomView) {
				v.Connected = false
				v.Reconnecting = true
				v.LastError = err.Error()
			})
			c.state.AddLog("连接失败: " + err.Error())
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *WSClient) connectOnce(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	opts, err := c.dialOptions()
	if err != nil {
		return err
	}
	conn, resp, err := websocket.Dial(dialCtx, c.wsURL, opts)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.mu.Unlock()
		_ = conn.Close(websocket.StatusNormalClosure, "cli reconnect")
	}()
	c.state.Mutate(func(v *RoomView) {
		v.Connected = true
		v.Reconnecting = false
	})
	c.state.AddLog("WebSocket 已连接")
	if err := c.login(ctx); err != nil {
		return err
	}
	errCh := make(chan error, 2)
	go func() { errCh <- c.readLoop(ctx, conn) }()
	go func() { errCh <- c.heartbeatLoop(ctx) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (c *WSClient) dialOptions() (*websocket.DialOptions, error) {
	opts := &websocket.DialOptions{}
	if c.origin != "" {
		opts.HTTPHeader = http.Header{"Origin": []string{c.origin}}
	}
	u, err := url.Parse(c.wsURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "wss" && c.insecureSkipVerify {
		opts.HTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} //nolint:gosec // 仅由显式调试参数启用。
	}
	return opts, nil
}

func (c *WSClient) login(ctx context.Context) error {
	token := readToken(c.tokenFile)
	return c.Send(ctx, msgid.LoginReq, &clientv1.Envelope{
		ReqId: newReqID("login"),
		Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{
			Nickname:     c.name,
			SessionToken: token,
		}},
	})
}

func (c *WSClient) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for ctx.Err() == nil {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if typ != websocket.MessageBinary {
			continue
		}
		h, err := frame.ReadFrame(bytes.NewReader(data))
		if err != nil {
			return err
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(h.Payload, &env); err != nil {
			return err
		}
		if login := env.GetLoginResp(); login != nil && login.GetSessionToken() != "" {
			_ = writeToken(c.tokenFile, login.GetSessionToken())
		}
		select {
		case c.events <- &env:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return ctx.Err()
}

func (c *WSClient) heartbeatLoop(ctx context.Context) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			start := time.Now()
			if err := c.Send(ctx, msgid.HeartbeatReq, &clientv1.Envelope{
				ReqId: newReqID("heartbeat"),
				Body:  &clientv1.Envelope_HeartbeatReq{HeartbeatReq: &clientv1.HeartbeatRequest{ClientTsMs: start.UnixMilli()}},
			}); err != nil {
				return err
			}
			c.state.Mutate(func(v *RoomView) { v.RTTms = time.Since(start).Milliseconds() })
		}
	}
}

func (c *WSClient) Send(ctx context.Context, id uint16, env *clientv1.Envelope) error {
	payload, err := proto.Marshal(env)
	if err != nil {
		return err
	}
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("尚未连接")
	}
	return conn.Write(ctx, websocket.MessageBinary, frame.Encode(id, payload))
}

func readToken(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path) // #nosec G304：token 路径由用户参数显式指定。
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(data))
}

func writeToken(path string, token string) error {
	if path == "" || token == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token), 0o600)
}

func newReqID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
