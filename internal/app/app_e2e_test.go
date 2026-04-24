// 端到端冒烟：四客户端进房、准备并收到结算推送。
package app_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/app"
	"racoo.cn/lsp/internal/config"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
)

func dialWS(t *testing.T, base string) *websocket.Conn {
	t.Helper()
	u := "ws://" + base + "/ws"
	c, resp, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("关闭握手响应体失败: %v", err)
		}
	}
	return c
}

func readUntilSettlement(t *testing.T, conn *websocket.Conn, max int) *clientv1.SettlementNotify {
	t.Helper()
	for n := 0; n < max; n++ {
		_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("读消息失败: %v", err)
		}
		h, err := frame.ReadFrame(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if h.MsgID != msgid.Settlement {
			continue
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(h.Payload, &env); err != nil {
			t.Fatal(err)
		}
		if sn := env.GetSettlement(); sn != nil {
			return sn
		}
	}
	t.Fatal("未收到结算推送")
	return nil
}

// loginJoinReturnSessionToken 与 loginJoin 相同，但返回登录响应中的 session_token（需 gate 启用 Redis）。
func loginJoinReturnSessionToken(t *testing.T, conn *websocket.Conn, roomID string) string {
	t.Helper()
	login := &clientv1.Envelope{ReqId: "l", Body: &clientv1.Envelope_LoginReq{
		LoginReq: &clientv1.LoginRequest{Nickname: "测试玩家"},
	}}
	pb, err := proto.Marshal(login)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)); err != nil {
		t.Fatal(err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	h, err := frame.ReadFrame(bytes.NewReader(data))
	if err != nil || h.MsgID != msgid.LoginResp {
		t.Fatal("登录响应异常")
	}
	var env clientv1.Envelope
	if err := proto.Unmarshal(h.Payload, &env); err != nil {
		t.Fatal(err)
	}
	tok := env.GetLoginResp().GetSessionToken()
	if tok == "" {
		t.Fatal("登录未返回会话令牌")
	}

	jr := &clientv1.Envelope{ReqId: "j", Body: &clientv1.Envelope_JoinRoomReq{
		JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: roomID},
	}}
	pb, err = proto.Marshal(jr)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb)); err != nil {
		t.Fatal(err)
	}
	_, data, err = conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	h, err = frame.ReadFrame(bytes.NewReader(data))
	if err != nil || h.MsgID != msgid.JoinRoomResp {
		t.Fatal("进房响应异常")
	}
	var joinEnv clientv1.Envelope
	if err := proto.Unmarshal(h.Payload, &joinEnv); err != nil {
		t.Fatal(err)
	}
	if joinEnv.GetJoinRoomResp().GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		t.Fatalf("进房失败: %v %s", joinEnv.GetJoinRoomResp().GetErrorCode(), joinEnv.GetJoinRoomResp().GetErrorMessage())
	}
	return tok
}

func loginJoin(t *testing.T, conn *websocket.Conn, roomID string) {
	t.Helper()
	login := &clientv1.Envelope{ReqId: "l", Body: &clientv1.Envelope_LoginReq{
		LoginReq: &clientv1.LoginRequest{Nickname: "测试玩家"},
	}}
	pb, _ := proto.Marshal(login)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)); err != nil {
		t.Fatal(err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	h, err := frame.ReadFrame(bytes.NewReader(data))
	if err != nil || h.MsgID != msgid.LoginResp {
		t.Fatal("登录响应异常")
	}

	jr := &clientv1.Envelope{ReqId: "j", Body: &clientv1.Envelope_JoinRoomReq{
		JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: roomID},
	}}
	pb, _ = proto.Marshal(jr)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.JoinRoomReq, pb)); err != nil {
		t.Fatal(err)
	}
	_, data, err = conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	h, err = frame.ReadFrame(bytes.NewReader(data))
	if err != nil || h.MsgID != msgid.JoinRoomResp {
		t.Fatal("进房响应异常")
	}
	var joinEnv clientv1.Envelope
	if err := proto.Unmarshal(h.Payload, &joinEnv); err != nil {
		t.Fatal(err)
	}
	if joinEnv.GetJoinRoomResp().GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		t.Fatalf("进房失败: %v %s", joinEnv.GetJoinRoomResp().GetErrorCode(), joinEnv.GetJoinRoomResp().GetErrorMessage())
	}
}

func sendReadyAndReadResp(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	rd := &clientv1.Envelope{ReqId: "r", Body: &clientv1.Envelope_ReadyReq{ReadyReq: &clientv1.ReadyRequest{}}}
	pb, err := proto.Marshal(rd)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.ReadyReq, pb)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		h, err := frame.ReadFrame(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if h.MsgID != msgid.ReadyResp {
			continue
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(h.Payload, &env); err != nil {
			t.Fatal(err)
		}
		if env.GetReadyResp().GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			t.Fatalf("准备失败: %v %s", env.GetReadyResp().GetErrorCode(), env.GetReadyResp().GetErrorMessage())
		}
		return
	}
	t.Fatal("准备阶段未收到 ReadyResp")
}

func TestE2EFourPlayersReceiveSettlement(t *testing.T) {
	cfg := config.Config{ServerAddr: "127.0.0.1:0", RuleID: "sichuan_xzdd"}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = a.Run(ctx)
	}()

	addr := a.Addr()
	if addr == nil {
		t.Fatal("监听地址为空")
	}
	host := addr.String()
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}

	roomID := "room-smoke-1"
	conns := make([]*websocket.Conn, 4)
	for i := range conns {
		conns[i] = dialWS(t, host)
		t.Cleanup(func() { _ = conns[i].Close() })
	}
	for _, c := range conns {
		loginJoin(t, c, roomID)
	}
	for i := range conns {
		sendReadyAndReadResp(t, conns[i])
	}

	var lastSn *clientv1.SettlementNotify
	for _, c := range conns {
		lastSn = readUntilSettlement(t, c, 64)
	}
	if lastSn == nil || lastSn.GetRoomId() != roomID {
		t.Fatalf("结算房间号不一致: %+v", lastSn)
	}
}
