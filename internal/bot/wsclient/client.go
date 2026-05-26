//nolint:staticcheck // ADR-0025 既有客户端路径使用 nhooyr.io/websocket；机器人客户端保持同栈。
package wsclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"nhooyr.io/websocket"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/protocol"
)

// Client 是机器人使用的最小 WebSocket 协议客户端。
type Client struct {
	wsURL              string
	origin             string
	insecureSkipVerify bool
	name               string
	tokenFile          string

	mu     sync.Mutex
	conn   *websocket.Conn
	events chan *clientv1.Envelope
}

// New 创建机器人 WebSocket 客户端。
func New(wsURL, name, tokenFile, origin string, insecureSkipVerify bool) *Client {
	return &Client{
		wsURL:              wsURL,
		name:               name,
		tokenFile:          tokenFile,
		origin:             origin,
		insecureSkipVerify: insecureSkipVerify,
		events:             make(chan *clientv1.Envelope, 256),
	}
}

// Events 返回下行事件通道。
func (c *Client) Events() <-chan *clientv1.Envelope {
	return c.events
}

// ClearToken 清除本地会话令牌，下一次重连会按新玩家登录。
func (c *Client) ClearToken() {
	if c.tokenFile != "" {
		_ = os.Remove(c.tokenFile)
	}
}

// Connect 建立连接并发送登录请求。
func (c *Client) Connect(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	if err := c.login(ctx); err != nil {
		return err
	}
	return nil
}

// RunLoops 启动读循环与心跳循环，任一返回错误即结束。
func (c *Client) RunLoops(ctx context.Context) error {
	errCh := make(chan error, 2)
	go func() { errCh <- c.readLoop(ctx) }()
	go func() { errCh <- c.heartbeatLoop(ctx) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// Close 关闭底层连接。
func (c *Client) Close() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "bot shutdown")
	}
}

// Send 发送一条 Protobuf Envelope 帧。
func (c *Client) Send(ctx context.Context, id uint16, env *clientv1.Envelope) error {
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
	enc, err := protocol.Encode(id, payload)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageBinary, enc)
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	opts, err := c.dialOptions()
	if err != nil {
		return nil, err
	}
	conn, resp, err := websocket.Dial(dialCtx, c.wsURL, opts)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return conn, err
}

func (c *Client) dialOptions() (*websocket.DialOptions, error) {
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

func (c *Client) login(ctx context.Context) error {
	return c.Send(ctx, protocol.LoginReq, &clientv1.Envelope{
		ReqId: newReqID("login"),
		Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{
			Nickname:     c.name,
			SessionToken: readToken(c.tokenFile),
		}},
	})
}

func (c *Client) readLoop(ctx context.Context) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("尚未连接")
	}
	for ctx.Err() == nil {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if typ != websocket.MessageBinary {
			continue
		}
		h, err := protocol.ReadFrame(bytes.NewReader(data))
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

func (c *Client) heartbeatLoop(ctx context.Context) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.Send(ctx, protocol.HeartbeatReq, &clientv1.Envelope{
				ReqId: newReqID("heartbeat"),
				Body:  &clientv1.Envelope_HeartbeatReq{HeartbeatReq: &clientv1.HeartbeatRequest{ClientTsMs: time.Now().UnixMilli()}},
			}); err != nil {
				return err
			}
		}
	}
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
	return os.WriteFile(path, []byte(strings.TrimSpace(token)), 0o600)
}

func newReqID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
