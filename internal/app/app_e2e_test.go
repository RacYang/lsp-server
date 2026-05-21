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

type wsTestMessage struct {
	msgID uint16
	env   *clientv1.Envelope
}

type wsTestBacklog struct {
	mu    sync.Mutex
	items []wsTestMessage
}

var wsTestBacklogs sync.Map

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
	t.Cleanup(func() { wsTestBacklogs.Delete(c) })
	return c
}

func stashWSMessage(conn *websocket.Conn, msg wsTestMessage) {
	backlog := backlogForWS(conn)
	backlog.mu.Lock()
	defer backlog.mu.Unlock()
	backlog.items = append(backlog.items, msg)
}

func popWSMessage(conn *websocket.Conn) (wsTestMessage, bool) {
	backlog := backlogForWS(conn)
	backlog.mu.Lock()
	defer backlog.mu.Unlock()
	if len(backlog.items) == 0 {
		return wsTestMessage{}, false
	}
	msg := backlog.items[0]
	copy(backlog.items, backlog.items[1:])
	backlog.items = backlog.items[:len(backlog.items)-1]
	return msg, true
}

func backlogForWS(conn *websocket.Conn) *wsTestBacklog {
	actual, _ := wsTestBacklogs.LoadOrStore(conn, &wsTestBacklog{})
	return actual.(*wsTestBacklog)
}

func readWSClientMessage(conn *websocket.Conn, timeout time.Duration) (wsTestMessage, error) {
	if msg, ok := popWSMessage(conn); ok {
		return msg, nil
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return wsTestMessage{}, fmt.Errorf("读消息失败: %w", err)
	}
	h, err := frame.ReadFrame(bytes.NewReader(data))
	if err != nil {
		return wsTestMessage{}, err
	}
	var env clientv1.Envelope
	if err := proto.Unmarshal(h.Payload, &env); err != nil {
		return wsTestMessage{}, err
	}
	return wsTestMessage{msgID: h.MsgID, env: &env}, nil
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
		msg, err := readWSClientMessage(conn, 4*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if msg.msgID != msgid.ReadyResp {
			stashWSMessage(conn, msg)
			continue
		}
		if msg.env.GetReadyResp().GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			t.Fatalf("准备失败: %v %s", msg.env.GetReadyResp().GetErrorCode(), msg.env.GetReadyResp().GetErrorMessage())
		}
		return
	}
	t.Fatal("准备阶段未收到 ReadyResp")
}

func driveConnUntilSettlement(conn *websocket.Conn, seat int32, max int) (*clientv1.SettlementNotify, error) {
	hand := make([]string, 0, 14)
	lastProgress := "no message"
	for n := 0; n < max; n++ {
		msg, err := readWSClientMessage(conn, 8*time.Second)
		if err != nil {
			return nil, fmt.Errorf("%w; seat=%d hand=%v last=%s", err, seat, hand, lastProgress)
		}
		lastProgress = fmt.Sprintf("msg=%d action=%s waiting_tile=%s hand_len=%d", msg.msgID, msg.env.GetAction().GetAction(), msg.env.GetDrawTile().GetTile(), len(hand))
		switch msg.msgID {
		case msgid.InitialDealNotify:
			if deal := msg.env.GetInitialDeal(); deal != nil && deal.GetSeatIndex() == seat {
				hand = append(hand[:0], deal.GetTiles()...)
			}
		case msgid.DrawTile:
			draw := msg.env.GetDrawTile()
			if draw != nil && draw.GetSeatIndex() == seat && draw.GetTile() != "" {
				hand = append(hand, draw.GetTile())
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
			action := msg.env.GetAction()
			if action == nil || action.GetSeatIndex() != seat {
				break
			}
			switch action.GetAction() {
			case "exchange_three":
				if len(hand) < 3 {
					break
				}
				tiles := append([]string(nil), hand[:3]...)
				hand = append([]string(nil), hand[3:]...)
				req := &clientv1.Envelope{
					ReqId: fmt.Sprintf("exchange-%d", n),
					Body: &clientv1.Envelope_OpeningActionReq{
						OpeningActionReq: &clientv1.OpeningActionRequest{
							Action: "exchange_three",
							Tiles:  tiles,
						},
					},
				}
				pb, err := proto.Marshal(req)
				if err != nil {
					return nil, err
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.OpeningActionReq, pb)); err != nil {
					return nil, err
				}
			case "que_men":
				req := &clientv1.Envelope{
					ReqId: fmt.Sprintf("que-%d", n),
					Body: &clientv1.Envelope_OpeningActionReq{
						OpeningActionReq: &clientv1.OpeningActionRequest{
							Action: "que_men",
							Suit:   0,
						},
					},
				}
				pb, err := proto.Marshal(req)
				if err != nil {
					return nil, err
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.OpeningActionReq, pb)); err != nil {
					return nil, err
				}
			case "pong_choice":
				req := &clientv1.Envelope{
					ReqId: fmt.Sprintf("pass-%d", n),
					Body: &clientv1.Envelope_PassReq{
						PassReq: &clientv1.PassRequest{},
					},
				}
				pb, err := proto.Marshal(req)
				if err != nil {
					return nil, err
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.PassReq, pb)); err != nil {
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
			case "hu_choice", "qiang_gang_choice", "tsumo_choice":
				req := &clientv1.Envelope{
					ReqId: fmt.Sprintf("hu-%d", n),
					Body: &clientv1.Envelope_HuReq{
						HuReq: &clientv1.HuRequest{},
					},
				}
				pb, err := proto.Marshal(req)
				if err != nil {
					return nil, err
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.HuReq, pb)); err != nil {
					return nil, err
				}
			}
		case msgid.Settlement:
			if sn := msg.env.GetSettlement(); sn != nil {
				return sn, nil
			}
		}
	}
	return nil, fmt.Errorf("未收到结算推送; seat=%d hand=%v last=%s", seat, hand, lastProgress)
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
			sn, err := driveConnUntilSettlement(c, int32(idx), 512) //nolint:gosec // 测试固定 4 个连接，idx 仅为 0..3
			results[idx] = result{sn: sn, err: err}
		}(i, conn)
	}
	wg.Wait()
	var last *clientv1.SettlementNotify
	var errs []error
	for _, result := range results {
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}
		last = result.sn
	}
	if len(errs) > 0 {
		t.Fatalf("驱动座位失败: %v", errs)
	}
	return last
}

func TestE2EFourPlayersReceiveSettlement(t *testing.T) {
	cfg := config.Config{ServerAddr: "127.0.0.1:0", RuleID: "sichuan_xuezhandaodi_huansanzhang"}
	a, err := app.New(t.Context(), cfg)
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
