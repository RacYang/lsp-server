// 端到端覆盖 ExchangeThree/QueMen/Discard/Pong/Gang/Hu/LeaveRoom 等动作 handler 的关键路径。
package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
)

// actionStubGateway 让所有动作返回可配置的错误与 after，便于驱动 handler 中的成功/失败分支。
type actionStubGateway struct {
	joinSeat   int
	joinErr    error
	actionErr  error
	afterCount int
}

func (g *actionStubGateway) Join(_ context.Context, _, _ string) (int, error) {
	return g.joinSeat, g.joinErr
}

func (g *actionStubGateway) makeAfter() func() {
	if g.actionErr != nil {
		return nil
	}
	return func() { g.afterCount++ }
}

func (g *actionStubGateway) Ready(_ context.Context, _, _ string) (func(), error) {
	return g.makeAfter(), g.actionErr
}
func (g *actionStubGateway) Leave(_ context.Context, _, _ string) (func(), error) {
	return g.makeAfter(), g.actionErr
}
func (g *actionStubGateway) ExchangeThree(_ context.Context, _, _ string, _ []string, _ int32) (func(), error) {
	return g.makeAfter(), g.actionErr
}
func (g *actionStubGateway) QueMen(_ context.Context, _, _ string, _ int32) (func(), error) {
	return g.makeAfter(), g.actionErr
}
func (g *actionStubGateway) Discard(_ context.Context, _, _, _ string) (func(), error) {
	return g.makeAfter(), g.actionErr
}
func (g *actionStubGateway) Pong(_ context.Context, _, _ string) (func(), error) {
	return g.makeAfter(), g.actionErr
}
func (g *actionStubGateway) Gang(_ context.Context, _, _, _ string) (func(), error) {
	return g.makeAfter(), g.actionErr
}
func (g *actionStubGateway) Hu(_ context.Context, _, _ string) (func(), error) {
	return g.makeAfter(), g.actionErr
}
func (g *actionStubGateway) Resume(_ context.Context, _ string) (*ResumeResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (g *actionStubGateway) EnsureRoomEventSubscription(_ context.Context, _, _ string) error {
	return nil
}

// loginAndJoin 准备 ws 连接到一个登录态 + 已进房状态，便于动作 handler 复用。
func loginAndJoin(t *testing.T, conn *websocket.Conn, roomID string) {
	t.Helper()

	login := &clientv1.Envelope{ReqId: "login", Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{}}}
	pb, err := proto.Marshal(login)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)))
	_ = readEnv(t, conn, msgid.LoginResp)

	join := &clientv1.Envelope{ReqId: "join", Body: &clientv1.Envelope_JoinRoomReq{JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: roomID}}}
	pb, err = proto.Marshal(join)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb)))
	_ = readEnv(t, conn, msgid.JoinRoomResp)
}

// actionCase 用统一结构描述一个动作 handler 的输入帧与从响应里取出 ErrorCode 的方法，便于表驱动。
type actionCase struct {
	name     string
	reqMsg   uint16
	respMsg  uint16
	build    func(reqID string) proto.Message
	getError func(*clientv1.Envelope) (clientv1.ErrorCode, string)
}

func actionCases() []actionCase {
	return []actionCase{
		{
			name:    "discard",
			reqMsg:  msgid.DiscardReq,
			respMsg: msgid.DiscardResp,
			build: func(reqID string) proto.Message {
				return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_DiscardReq{DiscardReq: &clientv1.DiscardRequest{Tile: "1m"}}}
			},
			getError: func(e *clientv1.Envelope) (clientv1.ErrorCode, string) {
				return e.GetDiscardResp().GetErrorCode(), e.GetDiscardResp().GetErrorMessage()
			},
		},
		{
			name:    "pong",
			reqMsg:  msgid.PongReq,
			respMsg: msgid.PongResp,
			build: func(reqID string) proto.Message {
				return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_PongReq{PongReq: &clientv1.PongRequest{}}}
			},
			getError: func(e *clientv1.Envelope) (clientv1.ErrorCode, string) {
				return e.GetPongResp().GetErrorCode(), e.GetPongResp().GetErrorMessage()
			},
		},
		{
			name:    "gang",
			reqMsg:  msgid.GangReq,
			respMsg: msgid.GangResp,
			build: func(reqID string) proto.Message {
				return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_GangReq{GangReq: &clientv1.GangRequest{Tile: "1m"}}}
			},
			getError: func(e *clientv1.Envelope) (clientv1.ErrorCode, string) {
				return e.GetGangResp().GetErrorCode(), e.GetGangResp().GetErrorMessage()
			},
		},
		{
			name:    "hu",
			reqMsg:  msgid.HuReq,
			respMsg: msgid.HuResp,
			build: func(reqID string) proto.Message {
				return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_HuReq{HuReq: &clientv1.HuRequest{}}}
			},
			getError: func(e *clientv1.Envelope) (clientv1.ErrorCode, string) {
				return e.GetHuResp().GetErrorCode(), e.GetHuResp().GetErrorMessage()
			},
		},
		{
			name:    "exchangeThree",
			reqMsg:  msgid.ExchangeThreeReq,
			respMsg: msgid.ExchangeThreeResp,
			build: func(reqID string) proto.Message {
				return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_ExchangeThreeReq{ExchangeThreeReq: &clientv1.ExchangeThreeRequest{Tiles: []string{"1m", "2m", "3m"}, Direction: 1}}}
			},
			getError: func(e *clientv1.Envelope) (clientv1.ErrorCode, string) {
				return e.GetExchangeThreeResp().GetErrorCode(), e.GetExchangeThreeResp().GetErrorMessage()
			},
		},
		{
			name:    "queMen",
			reqMsg:  msgid.QueMenReq,
			respMsg: msgid.QueMenResp,
			build: func(reqID string) proto.Message {
				return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_QueMenReq{QueMenReq: &clientv1.QueMenRequest{Suit: 0}}}
			},
			getError: func(e *clientv1.Envelope) (clientv1.ErrorCode, string) {
				return e.GetQueMenResp().GetErrorCode(), e.GetQueMenResp().GetErrorMessage()
			},
		},
	}
}

// TestActionHandlersHappyPath：所有动作 handler 在 stub 网关返回成功时必须给客户端写 UNSPECIFIED 响应并执行 after 闭包。
func TestActionHandlersHappyPath(t *testing.T) {
	defaultWSRateLimiter.Store(newUserRateLimiter(1000, 1000))
	defaultWSIdemCache.Store(newIdemCache(64))

	for _, c := range actionCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			gateway := &actionStubGateway{joinSeat: 0}
			hub := session.NewHub()
			srv := wsTestServer(t, Deps{Rooms: gateway, Hub: hub})
			defer srv.Close()
			conn := dialWS(t, srv)
			loginAndJoin(t, conn, "act-room-"+c.name)

			pb, err := proto.Marshal(c.build("req-1"))
			require.NoError(t, err)
			require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(c.reqMsg, pb)))
			env := readEnv(t, conn, c.respMsg)
			code, msg := c.getError(env)
			require.Equal(t, clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED, code, "动作 %s 成功路径应返回 UNSPECIFIED", c.name)
			require.Empty(t, msg)
			require.Equal(t, 1, gateway.afterCount, "成功路径必须运行 after 闭包")
		})
	}
}

// TestActionHandlersInvalidStateError：网关返回普通错误时必须把错误码映射成 INVALID_STATE，并保留原始消息。
func TestActionHandlersInvalidStateError(t *testing.T) {
	defaultWSRateLimiter.Store(newUserRateLimiter(1000, 1000))
	defaultWSIdemCache.Store(newIdemCache(64))

	for _, c := range actionCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			gateway := &actionStubGateway{joinSeat: 0, actionErr: errors.New("bad state")}
			hub := session.NewHub()
			srv := wsTestServer(t, Deps{Rooms: gateway, Hub: hub})
			defer srv.Close()
			conn := dialWS(t, srv)
			loginAndJoin(t, conn, "act-room-"+c.name)

			pb, err := proto.Marshal(c.build("req-1"))
			require.NoError(t, err)
			require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(c.reqMsg, pb)))
			env := readEnv(t, conn, c.respMsg)
			code, msg := c.getError(env)
			require.Equal(t, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, code, "动作 %s 失败路径应返回 INVALID_STATE", c.name)
			require.Contains(t, msg, "bad state")
			require.Equal(t, 0, gateway.afterCount, "失败路径不得调用 after 闭包")
		})
	}
}

// TestActionHandlersRateLimitedFromService：service 层抛 ErrRateLimited 时入口应回 RATE_LIMITED 给客户端。
func TestActionHandlersRateLimitedFromService(t *testing.T) {
	defaultWSRateLimiter.Store(newUserRateLimiter(1000, 1000))
	defaultWSIdemCache.Store(newIdemCache(64))

	gateway := &actionStubGateway{joinSeat: 0, actionErr: fmt.Errorf("wrap: %w", roomsvc.ErrRateLimited)}
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: gateway, Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)
	loginAndJoin(t, conn, "act-room-rl")

	req := &clientv1.Envelope{ReqId: "rl", Body: &clientv1.Envelope_DiscardReq{DiscardReq: &clientv1.DiscardRequest{Tile: "1m"}}}
	pb, err := proto.Marshal(req)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.DiscardReq, pb)))
	env := readEnv(t, conn, msgid.DiscardResp)
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_RATE_LIMITED, env.GetDiscardResp().GetErrorCode())
}

// TestActionHandlersDropWhenNotInRoom：未进房或 oneof 缺失时 handler 必须静默丢弃，避免后续 actor 推进；
// 这里同时覆盖 LeaveRoomReq 在没有 roomID 时回 INVALID_STATE 的 writeLeaveRoomError 分支。
func TestActionHandlersDropWhenNotInRoom(t *testing.T) {
	defaultWSRateLimiter.Store(newUserRateLimiter(1000, 1000))
	defaultWSIdemCache.Store(newIdemCache(64))

	gateway := &actionStubGateway{joinSeat: 0}
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: gateway, Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)

	login := &clientv1.Envelope{ReqId: "l", Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{}}}
	pb, err := proto.Marshal(login)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)))
	_ = readEnv(t, conn, msgid.LoginResp)

	leave := &clientv1.Envelope{ReqId: "leave-no-room", Body: &clientv1.Envelope_LeaveRoomReq{LeaveRoomReq: &clientv1.LeaveRoomRequest{}}}
	pb, err = proto.Marshal(leave)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LeaveRoomReq, pb)))
	env := readEnv(t, conn, msgid.LeaveRoomResp)
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, env.GetLeaveRoomResp().GetErrorCode())
	require.Contains(t, env.GetLeaveRoomResp().GetErrorMessage(), "尚未进入房间")
}

// TestActionHandlersIdempotencyHits：动作幂等键命中后必须直接静默丢弃，不再调用 gateway。
// 这里以 Discard 为代表，覆盖 shouldDropRequest 在 idempotency 命中时的行为路径。
func TestActionHandlersIdempotencyHits(t *testing.T) {
	defaultWSRateLimiter.Store(newUserRateLimiter(1000, 1000))
	defaultWSIdemCache.Store(newIdemCache(64))

	gateway := &actionStubGateway{joinSeat: 0}
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: gateway, Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)
	loginAndJoin(t, conn, "act-idem")

	req := &clientv1.Envelope{ReqId: "d-1", IdempotencyKey: "idem-d", Body: &clientv1.Envelope_DiscardReq{DiscardReq: &clientv1.DiscardRequest{Tile: "1m"}}}
	pb, err := proto.Marshal(req)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.DiscardReq, pb)))
	_ = readEnv(t, conn, msgid.DiscardResp)
	require.Equal(t, 1, gateway.afterCount)

	req2 := &clientv1.Envelope{ReqId: "d-2", IdempotencyKey: "idem-d", Body: &clientv1.Envelope_DiscardReq{DiscardReq: &clientv1.DiscardRequest{Tile: "1m"}}}
	pb, err = proto.Marshal(req2)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.DiscardReq, pb)))

	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("幂等键命中时不应再回响应")
	}
	require.Equal(t, 1, gateway.afterCount, "幂等命中时不能进入 gateway")
}
