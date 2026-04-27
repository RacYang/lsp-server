// 端到端冒烟：四客户端进房、准备并收到结算推送。
package app_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
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

func driveConnUntilSettlement(conn *websocket.Conn, seat int32, max int) (*clientv1.SettlementNotify, error) {
	for n := 0; n < max; n++ {
		_ = conn.SetReadDeadline(time.Now().Add(8 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("读消息失败: %w", err)
		}
		h, err := frame.ReadFrame(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(h.Payload, &env); err != nil {
			return nil, err
		}
		switch h.MsgID {
		case msgid.DrawTile:
			draw := env.GetDrawTile()
			if draw != nil && draw.GetSeatIndex() == seat {
				req := &clientv1.Envelope{
					ReqId: fmt.Sprintf("discard-%d", n),
					Body: &clientv1.Envelope_DiscardReq{
						DiscardReq: &clientv1.DiscardRequest{Tile: draw.GetTile()},
					},
				}
				pb, err := proto.Marshal(req)
				if err != nil {
					return nil, err
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.DiscardReq, pb)); err != nil {
					return nil, err
				}
			}
		case msgid.ActionNotify:
			action := env.GetAction()
			if action == nil || action.GetSeatIndex() != seat {
				break
			}
			switch action.GetAction() {
			case "exchange_three":
				req := &clientv1.Envelope{
					ReqId: fmt.Sprintf("exchange-%d", n),
					Body: &clientv1.Envelope_ExchangeThreeReq{
						ExchangeThreeReq: &clientv1.ExchangeThreeRequest{},
					},
				}
				pb, err := proto.Marshal(req)
				if err != nil {
					return nil, err
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.ExchangeThreeReq, pb)); err != nil {
					return nil, err
				}
			case "que_men":
				req := &clientv1.Envelope{
					ReqId: fmt.Sprintf("que-%d", n),
					Body: &clientv1.Envelope_QueMenReq{
						QueMenReq: &clientv1.QueMenRequest{Suit: 0},
					},
				}
				pb, err := proto.Marshal(req)
				if err != nil {
					return nil, err
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.QueMenReq, pb)); err != nil {
					return nil, err
				}
			case "pong_choice":
				req := &clientv1.Envelope{
					ReqId: fmt.Sprintf("pong-%d", n),
					Body: &clientv1.Envelope_PongReq{
						PongReq: &clientv1.PongRequest{},
					},
				}
				pb, err := proto.Marshal(req)
				if err != nil {
					return nil, err
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.PongReq, pb)); err != nil {
					return nil, err
				}
			case "gang_choice":
				req := &clientv1.Envelope{
					ReqId: fmt.Sprintf("gang-%d", n),
					Body: &clientv1.Envelope_GangReq{
						GangReq: &clientv1.GangRequest{Tile: action.GetTile()},
					},
				}
				pb, err := proto.Marshal(req)
				if err != nil {
					return nil, err
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.GangReq, pb)); err != nil {
					return nil, err
				}
			}
		case msgid.Settlement:
			if sn := env.GetSettlement(); sn != nil {
				return sn, nil
			}
		}
	}
	return nil, fmt.Errorf("未收到结算推送")
}

func drivePlayersUntilSettlement(t *testing.T, conns []*websocket.Conn) *clientv1.SettlementNotify {
	t.Helper()
	type result struct {
		sn  *clientv1.SettlementNotify
		err error
	}
	results := make([]result, len(conns))
	var wg sync.WaitGroup
	for i, conn := range conns {
		wg.Add(1)
		go func(idx int, c *websocket.Conn) {
			defer wg.Done()
			sn, err := driveConnUntilSettlement(c, int32(idx), 128) //nolint:gosec // 测试固定 4 个连接，idx 仅为 0..3
			results[idx] = result{sn: sn, err: err}
		}(i, conn)
	}
	wg.Wait()
	var last *clientv1.SettlementNotify
	for _, result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		last = result.sn
	}
	return last
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

	lastSn := drivePlayersUntilSettlement(t, conns)
	if lastSn == nil || lastSn.GetRoomId() != roomID {
		t.Fatalf("结算房间号不一致: %+v", lastSn)
	}
}
