package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/msgid"
)

// wsLobbyGateway 是 LobbyGateway 接口的生产实现，
// 通过 EventBus 等待响应，避免与牌桌主循环抢 events 通道。
type wsLobbyGateway struct {
	client *WSClient
	bus    *EventBus
}

// NewWSLobbyGateway 把 WSClient + EventBus 包装成 LobbyGateway。
func NewWSLobbyGateway(client *WSClient, bus *EventBus) LobbyGateway {
	return &wsLobbyGateway{client: client, bus: bus}
}

func (g *wsLobbyGateway) AutoMatch(ctx context.Context, ruleID string) (LobbyJoinResult, error) {
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetAutoMatchResp() != nil }, 4)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.AutoMatchReq, &clientv1.Envelope{
		ReqId: newReqID("automatch"),
		Body:  &clientv1.Envelope_AutoMatchReq{AutoMatchReq: &clientv1.AutoMatchRequest{RuleId: ruleID, PadWithBots: true}},
	}); err != nil {
		return LobbyJoinResult{}, err
	}
	env, err := awaitEnvelope(ctx, ch)
	if err != nil {
		return LobbyJoinResult{}, err
	}
	resp := env.GetAutoMatchResp()
	if errStr := envelopeError(resp.GetErrorCode(), resp.GetErrorMessage()); errStr != "" {
		return LobbyJoinResult{}, errors.New(errStr)
	}
	return LobbyJoinResult{
		RoomID:      resp.GetRoomId(),
		SeatIndex:   resp.GetSeatIndex(),
		DisplayName: resp.GetDisplayName(),
		RuleID:      resp.GetRuleId(),
	}, nil
}

func (g *wsLobbyGateway) ListRooms(ctx context.Context, pageToken string) (LobbyRoomList, error) {
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetListRoomsResp() != nil }, 4)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.ListRoomsReq, &clientv1.Envelope{
		ReqId: newReqID("listrooms"),
		Body:  &clientv1.Envelope_ListRoomsReq{ListRoomsReq: &clientv1.ListRoomsRequest{PageToken: pageToken, PageSize: 20}},
	}); err != nil {
		return LobbyRoomList{}, err
	}
	env, err := awaitEnvelope(ctx, ch)
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
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetCreateRoomResp() != nil }, 4)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.CreateRoomReq, &clientv1.Envelope{
		ReqId: newReqID("createroom"),
		Body: &clientv1.Envelope_CreateRoomReq{CreateRoomReq: &clientv1.CreateRoomRequest{
			RuleId:      opts.RuleID,
			DisplayName: opts.DisplayName,
			Private:     opts.Private,
		}},
	}); err != nil {
		return LobbyJoinResult{}, err
	}
	env, err := awaitEnvelope(ctx, ch)
	if err != nil {
		return LobbyJoinResult{}, err
	}
	resp := env.GetCreateRoomResp()
	if errStr := envelopeError(resp.GetErrorCode(), resp.GetErrorMessage()); errStr != "" {
		return LobbyJoinResult{}, errors.New(errStr)
	}
	return LobbyJoinResult{
		RoomID:      resp.GetRoomId(),
		SeatIndex:   resp.GetSeatIndex(),
		DisplayName: resp.GetDisplayName(),
		RuleID:      resp.GetRuleId(),
	}, nil
}

func (g *wsLobbyGateway) JoinRoom(ctx context.Context, roomID string) (LobbyJoinResult, error) {
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetJoinRoomResp() != nil }, 4)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.JoinRoomReq, &clientv1.Envelope{
		ReqId: newReqID("joinroom"),
		Body:  &clientv1.Envelope_JoinRoomReq{JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: roomID}},
	}); err != nil {
		return LobbyJoinResult{}, err
	}
	env, err := awaitEnvelope(ctx, ch)
	if err != nil {
		return LobbyJoinResult{}, err
	}
	resp := env.GetJoinRoomResp()
	if errStr := envelopeError(resp.GetErrorCode(), resp.GetErrorMessage()); errStr != "" {
		return LobbyJoinResult{}, errors.New(errStr)
	}
	return LobbyJoinResult{
		RoomID:      roomID,
		SeatIndex:   resp.GetSeatIndex(),
		DisplayName: resp.GetDisplayName(),
		RuleID:      resp.GetRuleId(),
	}, nil
}

func (g *wsLobbyGateway) ChangeNickname(name string) {
	g.client.SetConfig("", name)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetRenameResp() != nil }, 2)
		defer g.bus.Unsubscribe(id)
		_ = g.client.Send(ctx, msgid.RenameReq, &clientv1.Envelope{
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
