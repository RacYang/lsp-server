package main

import (
	"context"
	"errors"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// RPCCaller 用 req_id 把 WebSocket 请求和响应精确关联，避免同类型响应串包。
type RPCCaller struct {
	client *WSClient
	bus    *EventBus
}

func NewRPCCaller(client *WSClient, bus *EventBus) *RPCCaller {
	return &RPCCaller{client: client, bus: bus}
}

func (c *RPCCaller) Call(ctx context.Context, msgID uint16, env *clientv1.Envelope, match func(*clientv1.Envelope) bool) (*clientv1.Envelope, error) {
	if c == nil || c.client == nil || c.bus == nil {
		return nil, errors.New("rpc caller not initialized")
	}
	reqID := env.GetReqId()
	id, ch := c.bus.Subscribe(func(e *clientv1.Envelope) bool {
		return e.GetReqId() == reqID && (match == nil || match(e))
	}, 4)
	defer c.bus.Unsubscribe(id)
	if err := c.client.Send(ctx, msgID, env); err != nil {
		return nil, err
	}
	return awaitEnvelope(ctx, ch)
}
