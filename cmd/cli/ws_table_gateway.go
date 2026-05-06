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
}

// NewWSTableGateway 用 WSClient + EventBus 构造 TableGateway。
func NewWSTableGateway(client *WSClient, bus *EventBus) TableGateway {
	return &wsTableGateway{client: client, bus: bus}
}

// Ready 把准备请求发给服务端,然后阻塞等待 ReadyResp。错误码非空时返回错误。
func (g *wsTableGateway) Ready(ctx context.Context) error {
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetReadyResp() != nil }, 2)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.ReadyReq, &clientv1.Envelope{
		ReqId:          newReqID("ready"),
		IdempotencyKey: newReqID("idem-ready"),
		Body:           &clientv1.Envelope_ReadyReq{ReadyReq: &clientv1.ReadyRequest{}},
	}); err != nil {
		return err
	}
	env, err := awaitEnvelope(ctx, ch)
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
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetDiscardResp() != nil }, 2)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.DiscardReq, &clientv1.Envelope{
		ReqId:          newReqID("discard"),
		IdempotencyKey: newReqID("idem-discard"),
		Body:           &clientv1.Envelope_DiscardReq{DiscardReq: &clientv1.DiscardRequest{Tile: tile}},
	}); err != nil {
		return err
	}
	env, err := awaitEnvelope(ctx, ch)
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetDiscardResp().GetErrorCode(), env.GetDiscardResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// ExchangeThree 提交换三张请求,要求恰好 3 张牌,否则在网络往返之前直接拒绝。
func (g *wsTableGateway) ExchangeThree(ctx context.Context, tiles []string) error {
	if len(tiles) != 3 {
		return fmt.Errorf("换三张需要 3 张牌,当前 %d", len(tiles))
	}
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetExchangeThreeResp() != nil }, 2)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.ExchangeThreeReq, &clientv1.Envelope{
		ReqId:          newReqID("exchange"),
		IdempotencyKey: newReqID("idem-exchange"),
		Body:           &clientv1.Envelope_ExchangeThreeReq{ExchangeThreeReq: &clientv1.ExchangeThreeRequest{Tiles: tiles, Direction: 3}},
	}); err != nil {
		return err
	}
	env, err := awaitEnvelope(ctx, ch)
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetExchangeThreeResp().GetErrorCode(), env.GetExchangeThreeResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsTableGateway) QueMen(ctx context.Context, suit int32) error {
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetQueMenResp() != nil }, 2)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.QueMenReq, &clientv1.Envelope{
		ReqId:          newReqID("que"),
		IdempotencyKey: newReqID("idem-que"),
		Body:           &clientv1.Envelope_QueMenReq{QueMenReq: &clientv1.QueMenRequest{Suit: suit}},
	}); err != nil {
		return err
	}
	env, err := awaitEnvelope(ctx, ch)
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetQueMenResp().GetErrorCode(), env.GetQueMenResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsTableGateway) Pong(ctx context.Context) error {
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetPongResp() != nil }, 2)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.PongReq, &clientv1.Envelope{
		ReqId:          newReqID("pong"),
		IdempotencyKey: newReqID("idem-pong"),
		Body:           &clientv1.Envelope_PongReq{PongReq: &clientv1.PongRequest{}},
	}); err != nil {
		return err
	}
	env, err := awaitEnvelope(ctx, ch)
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetPongResp().GetErrorCode(), env.GetPongResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsTableGateway) Gang(ctx context.Context, tile string) error {
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetGangResp() != nil }, 2)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.GangReq, &clientv1.Envelope{
		ReqId:          newReqID("gang"),
		IdempotencyKey: newReqID("idem-gang"),
		Body:           &clientv1.Envelope_GangReq{GangReq: &clientv1.GangRequest{Tile: tile}},
	}); err != nil {
		return err
	}
	env, err := awaitEnvelope(ctx, ch)
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetGangResp().GetErrorCode(), env.GetGangResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsTableGateway) Hu(ctx context.Context) error {
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetHuResp() != nil }, 2)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.HuReq, &clientv1.Envelope{
		ReqId:          newReqID("hu"),
		IdempotencyKey: newReqID("idem-hu"),
		Body:           &clientv1.Envelope_HuReq{HuReq: &clientv1.HuRequest{}},
	}); err != nil {
		return err
	}
	env, err := awaitEnvelope(ctx, ch)
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetHuResp().GetErrorCode(), env.GetHuResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *wsTableGateway) Pass(ctx context.Context) error {
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetPassResp() != nil }, 2)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.PassReq, &clientv1.Envelope{
		ReqId:          newReqID("pass"),
		IdempotencyKey: newReqID("idem-pass"),
		Body:           &clientv1.Envelope_PassReq{PassReq: &clientv1.PassRequest{}},
	}); err != nil {
		return err
	}
	env, err := awaitEnvelope(ctx, ch)
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
	id, ch := g.bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetLeaveRoomResp() != nil }, 2)
	defer g.bus.Unsubscribe(id)
	if err := g.client.Send(ctx, msgid.LeaveRoomReq, &clientv1.Envelope{
		ReqId:          newReqID("leave"),
		IdempotencyKey: newReqID("idem-leave"),
		Body:           &clientv1.Envelope_LeaveRoomReq{LeaveRoomReq: &clientv1.LeaveRoomRequest{}},
	}); err != nil {
		return err
	}
	env, err := awaitEnvelope(ctx, ch)
	if err != nil {
		return err
	}
	if errStr := envelopeError(env.GetLeaveRoomResp().GetErrorCode(), env.GetLeaveRoomResp().GetErrorMessage()); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}
