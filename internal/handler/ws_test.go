// WebSocket 处理器集成测试：帧编解码与登录/进房主路径。
package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
	redisstore "racoo.cn/lsp/internal/store/redis"
)

type fakeResumeGateway struct {
	resumeResult *ResumeResult
	resumeErr    error
}

func (f *fakeResumeGateway) Join(_ context.Context, _, _ string) (int, error) { return 0, nil }
func (f *fakeResumeGateway) Ready(_ context.Context, _, _ string) (func(), error) {
	return nil, nil
}
func (f *fakeResumeGateway) Leave(_ context.Context, _, _ string) (func(), error) {
	return nil, nil
}
func (f *fakeResumeGateway) ExchangeThree(_ context.Context, _, _ string, _ []string, _ int32) (func(), error) {
	return nil, nil
}
func (f *fakeResumeGateway) QueMen(_ context.Context, _, _ string, _ int32) (func(), error) {
	return nil, nil
}
func (f *fakeResumeGateway) Discard(_ context.Context, _, _, _ string) (func(), error) {
	return nil, nil
}
func (f *fakeResumeGateway) Pong(_ context.Context, _, _ string) (func(), error) {
	return nil, nil
}
func (f *fakeResumeGateway) Gang(_ context.Context, _, _, _ string) (func(), error) {
	return nil, nil
}
func (f *fakeResumeGateway) Hu(_ context.Context, _, _ string) (func(), error) {
	return nil, nil
}
func (f *fakeResumeGateway) Resume(_ context.Context, _ string) (*ResumeResult, error) {
	if f.resumeErr != nil {
		return nil, f.resumeErr
	}
	return f.resumeResult, nil
}
func (f *fakeResumeGateway) EnsureRoomEventSubscription(_ context.Context, _, _ string) error {
	return nil
}

type joinStubGateway struct {
	joinSeat int
	joinErr  error
	readyN   int
}

func (g *joinStubGateway) Join(_ context.Context, _, _ string) (int, error) {
	if g == nil {
		return 0, fmt.Errorf("nil gateway")
	}
	return g.joinSeat, g.joinErr
}

func (g *joinStubGateway) Ready(_ context.Context, _, _ string) (func(), error) {
	g.readyN++
	return nil, nil
}
func (g *joinStubGateway) Leave(_ context.Context, _, _ string) (func(), error) { return nil, nil }
func (g *joinStubGateway) ExchangeThree(_ context.Context, _, _ string, _ []string, _ int32) (func(), error) {
	return nil, nil
}
func (g *joinStubGateway) QueMen(_ context.Context, _, _ string, _ int32) (func(), error) {
	return nil, nil
}
func (g *joinStubGateway) Discard(_ context.Context, _, _, _ string) (func(), error) {
	return nil, nil
}
func (g *joinStubGateway) Pong(_ context.Context, _, _ string) (func(), error) {
	return nil, nil
}
func (g *joinStubGateway) Gang(_ context.Context, _, _, _ string) (func(), error) {
	return nil, nil
}
func (g *joinStubGateway) Hu(_ context.Context, _, _ string) (func(), error) {
	return nil, nil
}
func (g *joinStubGateway) Resume(_ context.Context, _ string) (*ResumeResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (g *joinStubGateway) EnsureRoomEventSubscription(_ context.Context, _, _ string) error {
	return nil
}

func newTestRedisClient(t *testing.T) (*redisstore.Client, *miniredis.Miniredis) {
	t.Helper()
	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)
	cli := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = cli.Close() })
	return redisstore.NewClientFromUniversal(cli), srv
}

func wsTestServer(t *testing.T, deps Deps) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(context.Background(), deps, w, r)
	}))
}

func dialWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
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
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func readEnv(t *testing.T, conn *websocket.Conn, wantMsg uint16) *clientv1.Envelope {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	h, err := frame.ReadFrame(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if h.MsgID != wantMsg {
		t.Fatalf("msg_id want %d got %d", wantMsg, h.MsgID)
	}
	var env clientv1.Envelope
	if err := proto.Unmarshal(h.Payload, &env); err != nil {
		t.Fatal(err)
	}
	return &env
}

func TestHandleWebSocketLoginJoinReady(t *testing.T) {
	lobby := roomsvc.NewLobby()
	hub := session.NewHub()
	svc := roomsvc.NewService(lobby)
	srv := wsTestServer(t, Deps{Rooms: NewLocalRoomGateway(svc, hub, nil), Hub: hub})
	defer srv.Close()

	conn := dialWS(t, srv)

	login := &clientv1.Envelope{ReqId: "a", Body: &clientv1.Envelope_LoginReq{
		LoginReq: &clientv1.LoginRequest{Nickname: "玩家"},
	}}
	pb, _ := proto.Marshal(login)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)); err != nil {
		t.Fatal(err)
	}
	env := readEnv(t, conn, msgid.LoginResp)
	uid := env.GetLoginResp().GetUserId()
	if uid == "" {
		t.Fatal("empty user id")
	}

	jr := &clientv1.Envelope{ReqId: "b", Body: &clientv1.Envelope_JoinRoomReq{
		JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: "room-1"},
	}}
	pb, _ = proto.Marshal(jr)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb)); err != nil {
		t.Fatal(err)
	}
	env = readEnv(t, conn, msgid.JoinRoomResp)
	if env.GetJoinRoomResp().GetSeatIndex() < 0 {
		t.Fatal("seat")
	}

	rd := &clientv1.Envelope{ReqId: "c", Body: &clientv1.Envelope_ReadyReq{ReadyReq: &clientv1.ReadyRequest{}}}
	pb, _ = proto.Marshal(rd)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.ReadyReq, pb)); err != nil {
		t.Fatal(err)
	}
	env = readEnv(t, conn, msgid.ReadyResp)
	if env.GetReadyResp().GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		t.Fatalf("ready err %v", env.GetReadyResp().GetErrorCode())
	}
}

func TestHandleWebSocketBadFrameIgnored(t *testing.T) {
	svc := roomsvc.NewService(roomsvc.NewLobby())
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: NewLocalRoomGateway(svc, hub, nil), Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected timeout without server reply")
	}
}

func TestHandleWebSocketIdempotencyKeyDropsReplay(t *testing.T) {
	defaultWSRateLimiter = newUserRateLimiter(1000, 1000)
	defaultWSIdemCache = newIdemCache(16)
	gateway := &joinStubGateway{joinSeat: 0}
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: gateway, Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)

	login := &clientv1.Envelope{ReqId: "login", Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{}}}
	pb, _ := proto.Marshal(login)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)))
	_ = readEnv(t, conn, msgid.LoginResp)

	join := &clientv1.Envelope{ReqId: "join", Body: &clientv1.Envelope_JoinRoomReq{JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: "r-idem"}}}
	pb, _ = proto.Marshal(join)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb)))
	_ = readEnv(t, conn, msgid.JoinRoomResp)

	ready := &clientv1.Envelope{ReqId: "ready-1", IdempotencyKey: "idem-1", Body: &clientv1.Envelope_ReadyReq{ReadyReq: &clientv1.ReadyRequest{}}}
	pb, _ = proto.Marshal(ready)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.ReadyReq, pb)))
	_ = readEnv(t, conn, msgid.ReadyResp)
	ready.ReqId = "ready-2"
	pb, _ = proto.Marshal(ready)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.ReadyReq, pb)))

	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	require.Error(t, err)
	require.Equal(t, 1, gateway.readyN)
}

func TestHandleWebSocketUnknownMsgID(t *testing.T) {
	svc := roomsvc.NewService(roomsvc.NewLobby())
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: NewLocalRoomGateway(svc, hub, nil), Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)
	login := &clientv1.Envelope{ReqId: "1", Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{}}}
	pb, _ := proto.Marshal(login)
	_ = conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb))
	_, _, _ = conn.ReadMessage()

	if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(99, []byte{1})); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("unknown msg should not respond")
	}
}

func TestHandleWebSocketRejectsCrossOriginByDefault(t *testing.T) {
	svc := roomsvc.NewService(roomsvc.NewLobby())
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: NewLocalRoomGateway(svc, hub, nil), Hub: hub})
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http")
	header := http.Header{"Origin": []string{"https://evil.example"}}
	conn, resp, err := websocket.DefaultDialer.Dial(u, header)
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err)
	if resp != nil {
		_ = resp.Body.Close()
	}
}

func TestHandleWebSocketAllowsConfiguredOrigin(t *testing.T) {
	svc := roomsvc.NewService(roomsvc.NewLobby())
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{
		Rooms:          NewLocalRoomGateway(svc, hub, nil),
		Hub:            hub,
		AllowedOrigins: []string{"https://trusted.example"},
	})
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http")
	header := http.Header{"Origin": []string{"https://trusted.example"}}
	conn, resp, err := websocket.DefaultDialer.Dial(u, header)
	require.NoError(t, err)
	if resp != nil {
		_ = resp.Body.Close()
	}
	_ = conn.Close()
}

func TestHandleWebSocketHeartbeat(t *testing.T) {
	svc := roomsvc.NewService(roomsvc.NewLobby())
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: NewLocalRoomGateway(svc, hub, nil), Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)

	req := &clientv1.Envelope{ReqId: "hb", Body: &clientv1.Envelope_HeartbeatReq{
		HeartbeatReq: &clientv1.HeartbeatRequest{ClientTsMs: 1},
	}}
	pb, _ := proto.Marshal(req)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.HeartbeatReq, pb)))
	env := readEnv(t, conn, msgid.HeartbeatResp)
	require.NotZero(t, env.GetHeartbeatResp().GetServerTsMs())
}

func TestHandleWebSocketLeaveRoom(t *testing.T) {
	rcli, _ := newTestRedisClient(t)
	sess := session.NewManager(rcli)
	lobby := roomsvc.NewLobby()
	hub := session.NewHub()
	svc := roomsvc.NewService(lobby)
	srv := wsTestServer(t, Deps{Rooms: NewLocalRoomGateway(svc, hub, sess), Hub: hub, Session: sess})
	defer srv.Close()

	conn := dialWS(t, srv)
	login := &clientv1.Envelope{ReqId: "a", Body: &clientv1.Envelope_LoginReq{
		LoginReq: &clientv1.LoginRequest{Nickname: "玩家"},
	}}
	pb, _ := proto.Marshal(login)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)))
	loginResp := readEnv(t, conn, msgid.LoginResp)
	uid := loginResp.GetLoginResp().GetUserId()

	jr := &clientv1.Envelope{ReqId: "b", Body: &clientv1.Envelope_JoinRoomReq{
		JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: "room-leave"},
	}}
	pb, _ = proto.Marshal(jr)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb)))
	readEnv(t, conn, msgid.JoinRoomResp)

	req := &clientv1.Envelope{ReqId: "c", Body: &clientv1.Envelope_LeaveRoomReq{
		LeaveRoomReq: &clientv1.LeaveRoomRequest{},
	}}
	pb, _ = proto.Marshal(req)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LeaveRoomReq, pb)))
	env := readEnv(t, conn, msgid.LeaveRoomResp)
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED, env.GetLeaveRoomResp().GetErrorCode())

	token := loginResp.GetLoginResp().GetSessionToken()
	_, rec, err := sess.Resume(context.Background(), token)
	require.NoError(t, err)
	require.Empty(t, rec.RoomID)
	stillRegistered := false
	hub.IterRoomUsers("room-leave", func(userID string) {
		if userID == uid {
			stillRegistered = true
		}
	})
	require.False(t, stillRegistered)
}

func TestHandleWebSocketJoinRoomFull(t *testing.T) {
	lobby := roomsvc.NewLobby()
	svc := roomsvc.NewService(lobby)
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: NewLocalRoomGateway(svc, hub, nil), Hub: hub})
	defer srv.Close()

	for i := 0; i < 4; i++ {
		c := dialWS(t, srv)
		login := &clientv1.Envelope{ReqId: "x", Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{}}}
		pb, _ := proto.Marshal(login)
		_ = c.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb))
		_, _, _ = c.ReadMessage()
		jr := &clientv1.Envelope{ReqId: "y", Body: &clientv1.Envelope_JoinRoomReq{
			JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: "full-room"},
		}}
		pb, _ = proto.Marshal(jr)
		_ = c.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb))
		_, _, _ = c.ReadMessage()
	}

	c5 := dialWS(t, srv)
	login := &clientv1.Envelope{ReqId: "x", Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{}}}
	pb, _ := proto.Marshal(login)
	_ = c5.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb))
	_, _, _ = c5.ReadMessage()
	jr := &clientv1.Envelope{ReqId: "y", Body: &clientv1.Envelope_JoinRoomReq{
		JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: "full-room"},
	}}
	pb, _ = proto.Marshal(jr)
	_ = c5.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb))
	env := readEnv(t, c5, msgid.JoinRoomResp)
	if env.GetJoinRoomResp().GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_ROOM_FULL {
		t.Fatalf("want ROOM_FULL got %v", env.GetJoinRoomResp().GetErrorCode())
	}
}

func TestJoinRoomErrorCodeMapsNonFullErrors(t *testing.T) {
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: &joinStubGateway{joinErr: fmt.Errorf("rpc: dial tcp connection refused")}, Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)
	login := &clientv1.Envelope{ReqId: "1", Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{}}}
	pb, _ := proto.Marshal(login)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)))
	readEnv(t, conn, msgid.LoginResp)

	jr := &clientv1.Envelope{ReqId: "2", Body: &clientv1.Envelope_JoinRoomReq{JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: "r1"}}}
	pb, _ = proto.Marshal(jr)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb)))
	env := readEnv(t, conn, msgid.JoinRoomResp)
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED, env.GetJoinRoomResp().GetErrorCode())
}

func TestJoinRoomErrorCodeRoomNotFound(t *testing.T) {
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: &joinStubGateway{joinErr: fmt.Errorf("room not found")}, Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)
	login := &clientv1.Envelope{ReqId: "1", Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{}}}
	pb, _ := proto.Marshal(login)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)))
	readEnv(t, conn, msgid.LoginResp)

	jr := &clientv1.Envelope{ReqId: "2", Body: &clientv1.Envelope_JoinRoomReq{JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: "missing"}}}
	pb, _ = proto.Marshal(jr)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb)))
	env := readEnv(t, conn, msgid.JoinRoomResp)
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_ROOM_NOT_FOUND, env.GetJoinRoomResp().GetErrorCode())
}

func TestJoinRoomBindSessionFailureReturnsInvalidState(t *testing.T) {
	rcli, mr := newTestRedisClient(t)
	hub := session.NewHub()
	sess := session.NewManager(rcli)
	srv := wsTestServer(t, Deps{Rooms: &joinStubGateway{joinSeat: 0, joinErr: nil}, Hub: hub, Session: sess})
	defer srv.Close()
	conn := dialWS(t, srv)
	login := &clientv1.Envelope{ReqId: "1", Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{Nickname: "n"}}}
	pb, _ := proto.Marshal(login)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)))
	readEnv(t, conn, msgid.LoginResp)

	mr.Close()

	jr := &clientv1.Envelope{ReqId: "2", Body: &clientv1.Envelope_JoinRoomReq{JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: "r1"}}}
	pb, _ = proto.Marshal(jr)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb)))
	env := readEnv(t, conn, msgid.JoinRoomResp)
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, env.GetJoinRoomResp().GetErrorCode())
	require.NotEmpty(t, env.GetJoinRoomResp().GetErrorMessage())
}

func TestHandleWebSocketJoinBeforeLoginSkipped(t *testing.T) {
	svc := roomsvc.NewService(roomsvc.NewLobby())
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: NewLocalRoomGateway(svc, hub, nil), Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)
	jr := &clientv1.Envelope{ReqId: "y", Body: &clientv1.Envelope_JoinRoomReq{
		JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: "r"},
	}}
	pb, _ := proto.Marshal(jr)
	_ = conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb))
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected no response")
	}
}

func TestHandleWebSocketJoinEmptyBody(t *testing.T) {
	svc := roomsvc.NewService(roomsvc.NewLobby())
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: NewLocalRoomGateway(svc, hub, nil), Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)
	login := &clientv1.Envelope{ReqId: "1", Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{}}}
	pb, _ := proto.Marshal(login)
	_ = conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb))
	readEnv(t, conn, msgid.LoginResp)

	empty := &clientv1.Envelope{ReqId: "2"}
	pb, _ = proto.Marshal(empty)
	_ = conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb))
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("join oneof 为空时不应回包")
	}
}

func TestHandleWebSocketInvalidLoginPayload(t *testing.T) {
	svc := roomsvc.NewService(roomsvc.NewLobby())
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: NewLocalRoomGateway(svc, hub, nil), Hub: hub})
	defer srv.Close()
	conn := dialWS(t, srv)
	_ = conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, []byte{0xff}))
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected no reply")
	}
}

func TestHandleWebSocketUpgradeReject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(context.Background(), Deps{}, w, r)
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL) //nolint:noctx // 测试非 WS 请求触发升级失败分支
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Fatal("unexpected upgrade")
	}
}

func TestHandleWebSocketResumeRedirect(t *testing.T) {
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: &fakeResumeGateway{resumeResult: &ResumeResult{
		UserID:   "u1",
		RoomID:   "r1",
		Redirect: &clientv1.RouteRedirectNotify{WsUrl: "ws://gate-b/ws", Reason: "moved"},
	}}, Hub: hub, Session: &session.Manager{}})
	defer srv.Close()
	conn := dialWS(t, srv)

	req := &clientv1.Envelope{ReqId: "r", Body: &clientv1.Envelope_LoginReq{
		LoginReq: &clientv1.LoginRequest{SessionToken: "tok"},
	}}
	pb, _ := proto.Marshal(req)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)))

	env := readEnv(t, conn, msgid.LoginResp)
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_ROUTE_REDIRECT, env.GetLoginResp().GetErrorCode())
	redirect := readEnv(t, conn, msgid.RouteRedirectNotify)
	require.Equal(t, "ws://gate-b/ws", redirect.GetRouteRedirect().GetWsUrl())
}

func TestHandleWebSocketResumeSettlementFallback(t *testing.T) {
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: &fakeResumeGateway{resumeResult: &ResumeResult{
		UserID:     "u1",
		RoomID:     "r1",
		Resumed:    false,
		Settlement: &clientv1.SettlementNotify{RoomId: "r1", TotalFan: 8},
	}}, Hub: hub, Session: &session.Manager{}})
	defer srv.Close()
	conn := dialWS(t, srv)

	req := &clientv1.Envelope{ReqId: "r", Body: &clientv1.Envelope_LoginReq{
		LoginReq: &clientv1.LoginRequest{SessionToken: "tok"},
	}}
	pb, _ := proto.Marshal(req)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)))

	env := readEnv(t, conn, msgid.LoginResp)
	require.False(t, env.GetLoginResp().GetResumed())
	settle := readEnv(t, conn, msgid.Settlement)
	require.Equal(t, "r1", settle.GetSettlement().GetRoomId())
}

func TestHandleWebSocketResumeErrorCode(t *testing.T) {
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: &fakeResumeGateway{
		resumeErr: fmt.Errorf("wrapped: %w", &ResumeError{Code: clientv1.ErrorCode_ERROR_CODE_RECONNECTING, Message: "recovering"}),
	}, Hub: hub, Session: &session.Manager{}})
	defer srv.Close()
	conn := dialWS(t, srv)

	req := &clientv1.Envelope{ReqId: "r", Body: &clientv1.Envelope_LoginReq{
		LoginReq: &clientv1.LoginRequest{SessionToken: "tok"},
	}}
	pb, _ := proto.Marshal(req)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)))

	env := readEnv(t, conn, msgid.LoginResp)
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_RECONNECTING, env.GetLoginResp().GetErrorCode())
}
