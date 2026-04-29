package main

import (
	"context"
	"fmt"
	"strings"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/msgid"
)

// CommandHandler 把命令栏文本与快捷键映射为对服务器的请求帧。
type CommandHandler struct {
	client *WSClient
	state  *AppState
}

// NewCommandHandler 绑定 WSClient 与 AppState，便于命令既能发送也能写本地视图。
func NewCommandHandler(client *WSClient, state *AppState) *CommandHandler {
	return &CommandHandler{client: client, state: state}
}

// Handle 解析单行命令并调度具体处理；返回 false 表示请求退出 TUI。
func (h *CommandHandler) Handle(ctx context.Context, line string) bool {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return true
	}
	cmd := strings.ToLower(fields[0])
	var err error
	switch cmd {
	case "quit", "q", "exit":
		return false
	case "help":
		h.state.AddLog("命令: login/list/match/create/join/ready/d <牌>/p/g [牌]/h/ex <3牌>/que <m|p|s>/leave/quit")
	case "login":
		err = h.client.login(ctx)
	case "list", "refresh":
		err = h.listRooms(ctx)
	case "match":
		err = h.match(ctx, fields)
	case "create":
		err = h.createRoom(ctx, fields)
	case "join":
		err = h.join(ctx, fields)
	case "ready":
		err = h.client.Send(ctx, msgid.ReadyReq, &clientv1.Envelope{ReqId: newReqID("ready"), Body: &clientv1.Envelope_ReadyReq{ReadyReq: &clientv1.ReadyRequest{}}})
	case "d", "discard":
		err = h.discard(ctx, fields)
	case "p", "pong":
		err = h.client.Send(ctx, msgid.PongReq, &clientv1.Envelope{ReqId: newReqID("pong"), IdempotencyKey: newReqID("idem-pong"), Body: &clientv1.Envelope_PongReq{PongReq: &clientv1.PongRequest{}}})
	case "g", "gang":
		err = h.gang(ctx, fields)
	case "h", "hu":
		err = h.client.Send(ctx, msgid.HuReq, &clientv1.Envelope{ReqId: newReqID("hu"), IdempotencyKey: newReqID("idem-hu"), Body: &clientv1.Envelope_HuReq{HuReq: &clientv1.HuRequest{}}})
	case "ex":
		err = h.exchange(ctx, fields)
	case "que":
		err = h.que(ctx, fields)
	case "leave":
		err = h.client.Send(ctx, msgid.LeaveRoomReq, &clientv1.Envelope{ReqId: newReqID("leave"), IdempotencyKey: newReqID("idem-leave"), Body: &clientv1.Envelope_LeaveRoomReq{LeaveRoomReq: &clientv1.LeaveRoomRequest{}}})
	case "status":
		h.state.AddLog("状态已刷新")
	default:
		err = fmt.Errorf("未知命令: %s", cmd)
	}
	if err != nil {
		h.state.AddLog("命令失败: " + err.Error())
	}
	return true
}

func (h *CommandHandler) listRooms(ctx context.Context) error {
	return h.client.Send(ctx, msgid.ListRoomsReq, &clientv1.Envelope{
		ReqId: newReqID("list"),
		Body:  &clientv1.Envelope_ListRoomsReq{ListRoomsReq: &clientv1.ListRoomsRequest{PageSize: 20}},
	})
}

func (h *CommandHandler) match(ctx context.Context, fields []string) error {
	ruleID := ""
	if len(fields) > 1 {
		ruleID = fields[1]
	}
	return h.client.Send(ctx, msgid.AutoMatchReq, &clientv1.Envelope{
		ReqId:          newReqID("match"),
		IdempotencyKey: newReqID("idem-match"),
		Body:           &clientv1.Envelope_AutoMatchReq{AutoMatchReq: &clientv1.AutoMatchRequest{RuleId: ruleID}},
	})
}

func (h *CommandHandler) createRoom(ctx context.Context, fields []string) error {
	ruleID := ""
	displayName := ""
	private := false
	if len(fields) > 1 {
		ruleID = fields[1]
	}
	if len(fields) > 2 {
		displayName = strings.Join(fields[2:], " ")
	}
	if strings.Contains(displayName, "--private") {
		private = true
		displayName = strings.TrimSpace(strings.ReplaceAll(displayName, "--private", ""))
	}
	return h.sendCreateRoom(ctx, ruleID, displayName, private)
}

func (h *CommandHandler) sendCreateRoom(ctx context.Context, ruleID, displayName string, private bool) error {
	return h.client.Send(ctx, msgid.CreateRoomReq, &clientv1.Envelope{
		ReqId:          newReqID("create"),
		IdempotencyKey: newReqID("idem-create"),
		Body: &clientv1.Envelope_CreateRoomReq{CreateRoomReq: &clientv1.CreateRoomRequest{
			RuleId:      ruleID,
			DisplayName: displayName,
			Private:     private,
		}},
	})
}

// join 入房：先把房间号写入本地视图再发出 JoinRoomRequest，便于失败重试时仍可见上下文。
func (h *CommandHandler) join(ctx context.Context, fields []string) error {
	if len(fields) < 2 {
		return fmt.Errorf("用法: join <room_id>")
	}
	roomID := fields[1]
	h.state.Mutate(func(v *RoomView) { v.RoomID = roomID })
	return h.client.Send(ctx, msgid.JoinRoomReq, &clientv1.Envelope{
		ReqId: newReqID("join"),
		Body:  &clientv1.Envelope_JoinRoomReq{JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: roomID}},
	})
}

// discard 出牌：用户必须显式给出牌名，避免在多手牌情况下误打。
func (h *CommandHandler) discard(ctx context.Context, fields []string) error {
	if len(fields) < 2 {
		return fmt.Errorf("用法: d <tile>")
	}
	tile := fields[1]
	return h.client.Send(ctx, msgid.DiscardReq, &clientv1.Envelope{
		ReqId:          newReqID("discard"),
		IdempotencyKey: newReqID("idem-discard"),
		Body:           &clientv1.Envelope_DiscardReq{DiscardReq: &clientv1.DiscardRequest{Tile: tile}},
	})
}

// gang 杠牌：自杠时由用户指定牌名，抢杠场景下牌名会被服务端忽略。
func (h *CommandHandler) gang(ctx context.Context, fields []string) error {
	tile := ""
	if len(fields) > 1 {
		tile = fields[1]
	}
	return h.client.Send(ctx, msgid.GangReq, &clientv1.Envelope{
		ReqId:          newReqID("gang"),
		IdempotencyKey: newReqID("idem-gang"),
		Body:           &clientv1.Envelope_GangReq{GangReq: &clientv1.GangRequest{Tile: tile}},
	})
}

// exchange 换三张：本地先把三张从手牌移除以便立即更新 UI，待 ExchangeThreeDone 再补回新牌。
func (h *CommandHandler) exchange(ctx context.Context, fields []string) error {
	if len(fields) < 4 {
		return fmt.Errorf("用法: ex <t1> <t2> <t3> [direction]")
	}
	direction := int32(3)
	if len(fields) > 4 {
		switch fields[4] {
		case "1", "cw":
			direction = 1
		case "2", "opp":
			direction = 2
		case "3", "ccw":
			direction = 3
		}
	}
	tiles := append([]string(nil), fields[1:4]...)
	h.state.Mutate(func(v *RoomView) {
		if v.SeatIndex >= 0 && v.SeatIndex < 4 {
			for _, tile := range tiles {
				v.Players[v.SeatIndex].Hand = removeOneTile(v.Players[v.SeatIndex].Hand, tile)
			}
			v.Players[v.SeatIndex].HandCnt = len(v.Players[v.SeatIndex].Hand)
		}
	})
	return h.client.Send(ctx, msgid.ExchangeThreeReq, &clientv1.Envelope{
		ReqId:          newReqID("exchange"),
		IdempotencyKey: newReqID("idem-exchange"),
		Body:           &clientv1.Envelope_ExchangeThreeReq{ExchangeThreeReq: &clientv1.ExchangeThreeRequest{Tiles: tiles, Direction: direction}},
	})
}

func (h *CommandHandler) que(ctx context.Context, fields []string) error {
	if len(fields) < 2 {
		return fmt.Errorf("用法: que <m|p|s>")
	}
	suit, ok := suitFromShortCode(fields[1])
	if !ok {
		return fmt.Errorf("未知花色 %q，仅支持 m/p/s", fields[1])
	}
	return h.client.Send(ctx, msgid.QueMenReq, &clientv1.Envelope{
		ReqId:          newReqID("que"),
		IdempotencyKey: newReqID("idem-que"),
		Body:           &clientv1.Envelope_QueMenReq{QueMenReq: &clientv1.QueMenRequest{Suit: suit}},
	})
}

// suitFromShortCode 把命令行的 m/p/s 短码映射为协议层 suit 值。
//
// 与服务端 internal/mahjong/tile.Suit 严格对齐：
// m=SuitCharacters(0=万)、p=SuitDots(1=筒)、s=SuitBamboo(2=条)。
// 历史上曾因为 proto 注释把 1/2 写反，导致客户端把"筒"发成了"条"，
// 这里集中维护一处映射避免再次发散。
func suitFromShortCode(raw string) (int32, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "m":
		return 0, true
	case "p":
		return 1, true
	case "s":
		return 2, true
	default:
		return 0, false
	}
}

// DiscardIndex 通过手牌索引出牌，用于命令栏数字快捷键；越界静默忽略，避免误触。
func (h *CommandHandler) DiscardIndex(ctx context.Context, idx int) {
	view := h.state.Snapshot()
	if view.SeatIndex < 0 || view.SeatIndex > 3 {
		return
	}
	hand := view.Players[view.SeatIndex].Hand
	if idx < 0 || idx >= len(hand) {
		return
	}
	_ = h.discard(ctx, []string{"d", hand[idx]})
}
