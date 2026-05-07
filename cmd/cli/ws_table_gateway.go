package main

import (
	"context"
	"errors"
	"fmt"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/msgid"
)

// wsTableGateway 是 TableGateway 的生产实现，复用 EventBus 等待响应。
type wsTableGateway struct {
	client *WSClient
	bus    *EventBus
	rpc    *RPCCaller
}

// NewWSTableGateway 用 WSClient + EventBus 构造 TableGateway。
func NewWSTableGateway(client *WSClient, bus *EventBus) TableGateway {
	return &wsTableGateway{client: client, bus: bus, rpc: NewRPCCaller(client, bus)}
}

// Ready 把准备请求发给服务端,然后阻塞等待 ReadyResp。错误码非空时返回错误。
func (g *wsTableGateway) Ready(ctx context.Context) error {
	reqID := newReqID("ready")
	env, err := g.rpc.Call(ctx, msgid.ReadyReq, &clientv1.Envelope{
		ReqId:          reqID,
		IdempotencyKey: newReqID("idem-ready"),
		Body:           &clientv1.Envelope_ReadyReq{ReadyReq: &clientv1.ReadyRequest{}},
	}, func(e *clientv1.Envelope) bool { return e.GetReadyResp() != nil })
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetReadyResp().GetErrorCode(), env.GetReadyResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// Discard 提交出牌请求并等待 DiscardResp 落地;调用方负责保证 tile 是合法的协议牌名。
func (g *wsTableGateway) Discard(ctx context.Context, tile string) error {
	reqID := newReqID("discard")
	env, err := g.rpc.Call(ctx, msgid.DiscardReq, &clientv1.Envelope{
		ReqId:          reqID,
		IdempotencyKey: newReqID("idem-discard"),
		Body:           &clientv1.Envelope_DiscardReq{DiscardReq: &clientv1.DiscardRequest{Tile: tile}},
	}, func(e *clientv1.Envelope) bool { return e.GetDiscardResp() != nil })
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetDiscardResp().GetErrorCode(), env.GetDiscardResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// ExchangeThree 提交换三张请求,要求恰好 3 张牌,否则在网络往返之前直接拒绝。
func (g *wsTableGateway) ExchangeThree(ctx context.Context, tiles []string, direction int32) error {
	if len(tiles) != 3 {
		return fmt.Errorf("换三张需要 3 张牌,当前 %d", len(tiles))
	}
	reqID := newReqID("exchange")
	env, err := g.rpc.Call(ctx, msgid.ExchangeThreeReq, &clientv1.Envelope{
		ReqId:          reqID,
		IdempotencyKey: newReqID("idem-exchange"),
		Body:           &clientv1.Envelope_ExchangeThreeReq{ExchangeThreeReq: &clientv1.ExchangeThreeRequest{Tiles: tiles, Direction: direction}},
	}, func(e *clientv1.Envelope) bool { return e.GetExchangeThreeResp() != nil })
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetExchangeThreeResp().GetErrorCode(), env.GetExchangeThreeResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsTableGateway) QueMen(ctx context.Context, suit int32) error {
	reqID := newReqID("que")
	env, err := g.rpc.Call(ctx, msgid.QueMenReq, &clientv1.Envelope{
		ReqId:          reqID,
		IdempotencyKey: newReqID("idem-que"),
		Body:           &clientv1.Envelope_QueMenReq{QueMenReq: &clientv1.QueMenRequest{Suit: suit}},
	}, func(e *clientv1.Envelope) bool { return e.GetQueMenResp() != nil })
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetQueMenResp().GetErrorCode(), env.GetQueMenResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsTableGateway) Pong(ctx context.Context) error {
	reqID := newReqID("pong")
	env, err := g.rpc.Call(ctx, msgid.PongReq, &clientv1.Envelope{
		ReqId:          reqID,
		IdempotencyKey: newReqID("idem-pong"),
		Body:           &clientv1.Envelope_PongReq{PongReq: &clientv1.PongRequest{}},
	}, func(e *clientv1.Envelope) bool { return e.GetPongResp() != nil })
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetPongResp().GetErrorCode(), env.GetPongResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsTableGateway) Gang(ctx context.Context, tile string) error {
	reqID := newReqID("gang")
	env, err := g.rpc.Call(ctx, msgid.GangReq, &clientv1.Envelope{
		ReqId:          reqID,
		IdempotencyKey: newReqID("idem-gang"),
		Body:           &clientv1.Envelope_GangReq{GangReq: &clientv1.GangRequest{Tile: tile}},
	}, func(e *clientv1.Envelope) bool { return e.GetGangResp() != nil })
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetGangResp().GetErrorCode(), env.GetGangResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsTableGateway) Hu(ctx context.Context) error {
	reqID := newReqID("hu")
	env, err := g.rpc.Call(ctx, msgid.HuReq, &clientv1.Envelope{
		ReqId:          reqID,
		IdempotencyKey: newReqID("idem-hu"),
		Body:           &clientv1.Envelope_HuReq{HuReq: &clientv1.HuRequest{}},
	}, func(e *clientv1.Envelope) bool { return e.GetHuResp() != nil })
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetHuResp().GetErrorCode(), env.GetHuResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsTableGateway) Pass(ctx context.Context) error {
	reqID := newReqID("pass")
	env, err := g.rpc.Call(ctx, msgid.PassReq, &clientv1.Envelope{
		ReqId:          reqID,
		IdempotencyKey: newReqID("idem-pass"),
		Body:           &clientv1.Envelope_PassReq{PassReq: &clientv1.PassRequest{}},
	}, func(e *clientv1.Envelope) bool { return e.GetPassResp() != nil })
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetPassResp().GetErrorCode(), env.GetPassResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// LeaveRoom 主动离开当前房间,通常由牌桌"返回大厅"或结算后回大厅的流程调用。
func (g *wsTableGateway) LeaveRoom(ctx context.Context) error {
	reqID := newReqID("leave")
	env, err := g.rpc.Call(ctx, msgid.LeaveRoomReq, &clientv1.Envelope{
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

func (g *wsTableGateway) AddBot(ctx context.Context, count int32) ([]*clientv1.SeatInfo, error) {
	opID := newReqID("addbot")
	env, err := g.rpc.Call(ctx, msgid.AddBotReq, &clientv1.Envelope{
		ReqId:          opID,
		IdempotencyKey: opID,
		Body: &clientv1.Envelope_AddBotReq{AddBotReq: &clientv1.AddBotRequest{
			Count:      count,
			Difficulty: "normal",
			OpId:       opID,
		}},
	}, func(e *clientv1.Envelope) bool { return e.GetAddBotResp() != nil })
	if err != nil {
		return nil, err
	}
	resp := env.GetAddBotResp()
	if errStr := envelopeError(resp.GetErrorCode(), resp.GetErrorMessage()); errStr != "" {
		return nil, errors.New(errStr)
	}
	return resp.GetAdded(), nil
}
