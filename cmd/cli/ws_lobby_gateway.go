package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/protocol"
)

// wsLobbyGateway 是 LobbyGateway 接口的生产实现，
// 通过 EventBus 等待响应，避免与牌桌主循环抢 events 通道。
type wsLobbyGateway struct {
	client *WSClient
	bus    *EventBus
	state  *AppState
	rpc    *RPCCaller
}

// NewWSLobbyGateway 把 WSClient + EventBus 包装成 LobbyGateway。
// state 用于在 JoinRoomResp 这类 protobuf 不带 room_id 的响应里
// 由客户端侧补齐 RoomID/Phase，否则主循环看不到入桌信号会回退到 lobby。
func NewWSLobbyGateway(client *WSClient, bus *EventBus, state *AppState) LobbyGateway {
	return &wsLobbyGateway{client: client, bus: bus, state: state, rpc: NewRPCCaller(client, bus)}
}

func (g *wsLobbyGateway) AutoMatch(ctx context.Context, ruleID string) (LobbyJoinResult, error) {
	reqID := newReqID("automatch")
	env, err := g.rpc.Call(ctx, protocol.AutoMatchReq, &clientv1.Envelope{
		ReqId: reqID,
		Body:  &clientv1.Envelope_AutoMatchReq{AutoMatchReq: &clientv1.AutoMatchRequest{RuleId: ruleID, PadWithBots: true}},
	}, func(e *clientv1.Envelope) bool { return e.GetAutoMatchResp() != nil })
	if err != nil {
		return LobbyJoinResult{}, err
	}
	resp := env.GetAutoMatchResp()
	if errStr := envelopeError(resp.GetErrorCode(), resp.GetErrorMessage()); errStr != "" {
		return LobbyJoinResult{}, errors.New(errStr)
	}
	result := LobbyJoinResult{
		RoomID:      resp.GetRoomId(),
		SeatIndex:   resp.GetSeatIndex(),
		DisplayName: resp.GetDisplayName(),
		RuleID:      resp.GetRuleId(),
		Seats:       resp.GetSeats(),
	}
	applyJoinResultToState(g.state, result)
	return result, nil
}

func (g *wsLobbyGateway) LeaveRoom(ctx context.Context) error {
	reqID := newReqID("leave")
	env, err := g.rpc.Call(ctx, protocol.LeaveRoomReq, &clientv1.Envelope{
		ReqId:          reqID,
		IdempotencyKey: newReqID("idem-leave"),
		Body:           &clientv1.Envelope_LeaveRoomReq{LeaveRoomReq: &clientv1.LeaveRoomRequest{}},
	}, func(e *clientv1.Envelope) bool { return e.GetLeaveRoomResp() != nil })
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetLeaveRoomResp().GetErrorCode(), env.GetLeaveRoomResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsLobbyGateway) ListRules(ctx context.Context) ([]LobbyRuleMeta, error) {
	reqID := newReqID("listrules")
	env, err := g.rpc.Call(ctx, protocol.ListRulesReq, &clientv1.Envelope{
		ReqId: reqID,
		Body:  &clientv1.Envelope_ListRulesReq{ListRulesReq: &clientv1.ListRulesRequest{}},
	}, func(e *clientv1.Envelope) bool { return e.GetListRulesResp() != nil })
	if err != nil {
		return nil, err
	}
	resp := env.GetListRulesResp()
	if errStr := envelopeError(resp.GetErrorCode(), resp.GetErrorMessage()); errStr != "" {
		return nil, errors.New(errStr)
	}
	out := make([]LobbyRuleMeta, 0, len(resp.GetRules()))
	for _, rule := range resp.GetRules() {
		out = append(out, LobbyRuleMeta{
			RuleID:          rule.GetRuleId(),
			DisplayName:     rule.GetDisplayName(),
			ShortDesc:       rule.GetShortDesc(),
			EnabledFeatures: append([]string(nil), rule.GetEnabledFeatures()...),
			MaxHands:        rule.GetMaxHands(),
		})
	}
	return out, nil
}

func (g *wsLobbyGateway) ListRooms(ctx context.Context, pageToken string) (LobbyRoomList, error) {
	reqID := newReqID("listrooms")
	env, err := g.rpc.Call(ctx, protocol.ListRoomsReq, &clientv1.Envelope{
		ReqId: reqID,
		Body:  &clientv1.Envelope_ListRoomsReq{ListRoomsReq: &clientv1.ListRoomsRequest{PageToken: pageToken, PageSize: 20}},
	}, func(e *clientv1.Envelope) bool { return e.GetListRoomsResp() != nil })
	if err != nil {
		return LobbyRoomList{}, err
	}
	resp := env.GetListRoomsResp()
	if errStr := envelopeError(resp.GetErrorCode(), resp.GetErrorMessage()); errStr != "" {
		return LobbyRoomList{}, errors.New(errStr)
	}
	out := LobbyRoomList{NextPageToken: resp.GetNextPageToken()}
	for _, m := range resp.GetRooms() {
		seatCount := int(m.GetSeatCount())
		capacity := int(m.GetMaxSeats())
		if capacity == 0 {
			capacity = 4
		}
		out.Rooms = append(out.Rooms, LobbyRoomMeta{
			RoomID:      m.GetRoomId(),
			DisplayName: m.GetDisplayName(),
			Players:     seatCount,
			Capacity:    capacity,
			RuleID:      m.GetRuleId(),
		})
	}
	return out, nil
}

func (g *wsLobbyGateway) CreateRoom(ctx context.Context, opts LobbyCreateOpts) (LobbyJoinResult, error) {
	reqID := newReqID("createroom")
	env, err := g.rpc.Call(ctx, protocol.CreateRoomReq, &clientv1.Envelope{
		ReqId: reqID,
		Body: &clientv1.Envelope_CreateRoomReq{CreateRoomReq: &clientv1.CreateRoomRequest{
			RuleId:      opts.RuleID,
			DisplayName: opts.DisplayName,
			Private:     opts.Private,
		}},
	}, func(e *clientv1.Envelope) bool { return e.GetCreateRoomResp() != nil })
	if err != nil {
		return LobbyJoinResult{}, err
	}
	resp := env.GetCreateRoomResp()
	if errStr := envelopeError(resp.GetErrorCode(), resp.GetErrorMessage()); errStr != "" {
		return LobbyJoinResult{}, errors.New(errStr)
	}
	result := LobbyJoinResult{
		RoomID:      resp.GetRoomId(),
		SeatIndex:   resp.GetSeatIndex(),
		DisplayName: resp.GetDisplayName(),
		RuleID:      resp.GetRuleId(),
		Seats:       resp.GetSeats(),
		Private:     opts.Private,
	}
	applyJoinResultToState(g.state, result)
	return result, nil
}

func (g *wsLobbyGateway) JoinRoom(ctx context.Context, roomID string) (LobbyJoinResult, error) {
	reqID := newReqID("joinroom")
	env, err := g.rpc.Call(ctx, protocol.JoinRoomReq, &clientv1.Envelope{
		ReqId: reqID,
		Body:  &clientv1.Envelope_JoinRoomReq{JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: roomID}},
	}, func(e *clientv1.Envelope) bool { return e.GetJoinRoomResp() != nil })
	if err != nil {
		return LobbyJoinResult{}, err
	}
	resp := env.GetJoinRoomResp()
	if errStr := envelopeError(resp.GetErrorCode(), resp.GetErrorMessage()); errStr != "" {
		return LobbyJoinResult{}, errors.New(errStr)
	}
	result := LobbyJoinResult{
		RoomID:      roomID,
		SeatIndex:   resp.GetSeatIndex(),
		DisplayName: resp.GetDisplayName(),
		RuleID:      resp.GetRuleId(),
		Seats:       resp.GetSeats(),
	}
	// JoinRoomResp 不携带 room_id，state.Apply 走 envelope 分支也写不进 RoomID；
	// 这里由客户端把它补上，并切到 phaseTable，让 main 主循环识别"已入桌"。
	applyJoinResultToState(g.state, result)
	return result, nil
}

// applyJoinResultToState 把成功的 LobbyJoinResult 落到本地 view，
// 主要为补齐 JoinRoomResp 缺失的 RoomID 字段，统一切换 phaseTable。
func applyJoinResultToState(state *AppState, res LobbyJoinResult) {
	if state == nil || res.RoomID == "" {
		return
	}
	state.Mutate(func(v *RoomView) {
		v.RoomID = res.RoomID
		v.SeatIndex = res.SeatIndex
		if res.RuleID != "" {
			v.RuleID = res.RuleID
		}
		if res.DisplayName != "" {
			v.DisplayName = res.DisplayName
		}
		applySeatRoster(v, res.Seats)
		v.Phase = phaseTable
		// [L5.2]/[P4.2] 只有本次"成功创建私密房"这一种语义来源写 Private=true；
		// AutoMatch/JoinRoom 与 LeaveRoom 路径走 false，避免私密标签泄漏到下一房。
		v.Private = res.Private
	})
}

func (g *wsLobbyGateway) ChangeNickname(name string) {
	g.client.SetConfig("", name)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetRenameResp() != nil }, 2)
		defer g.bus.Unsubscribe(id)
		_ = g.client.Send(ctx, protocol.RenameReq, &clientv1.Envelope{
			ReqId: newReqID("rename"),
			Body:  &clientv1.Envelope_RenameReq{RenameReq: &clientv1.RenameRequest{Nickname: name}},
		})
		if env, err := awaitEnvelope(ctx, ch); err == nil {
			if applied := env.GetRenameResp().GetAppliedNickname(); applied != "" {
				g.client.SetConfig("", applied)
			}
		}
	}()
}

// awaitEnvelope 在 ctx 与 ch 上 select，封装通用的"等待响应"语义。
func awaitEnvelope(ctx context.Context, ch <-chan *clientv1.Envelope) (*clientv1.Envelope, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case env, ok := <-ch:
		if !ok {
			return nil, errors.New("事件总线已关闭")
		}
		return env, nil
	}
}

// envelopeError 把 ErrorCode + 文案合并为一个简短可读字符串；正常时返回空串。
func envelopeError(code clientv1.ErrorCode, msg string) string {
	if code == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return ""
	}
	if msg == "" {
		return fmt.Sprintf("错误码 %d", code)
	}
	return msg
}
