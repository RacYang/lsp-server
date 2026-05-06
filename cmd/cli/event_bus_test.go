package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func TestEventBusFanoutToMultipleSubscribers(t *testing.T) {
	bus := NewEventBus(nil)
	src := make(chan *clientv1.Envelope, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { bus.Run(ctx, src); close(done) }()

	_, ch1 := bus.Subscribe(nil, 4)
	_, ch2 := bus.Subscribe(nil, 4)
	src <- &clientv1.Envelope{ReqId: "a"}
	src <- &clientv1.Envelope{ReqId: "b"}
	require.Equal(t, "a", recvEnv(t, ch1).GetReqId())
	require.Equal(t, "b", recvEnv(t, ch1).GetReqId())
	require.Equal(t, "a", recvEnv(t, ch2).GetReqId())
	require.Equal(t, "b", recvEnv(t, ch2).GetReqId())

	cancel()
	<-done
}

func TestEventBusMatcherFiltersEvents(t *testing.T) {
	bus := NewEventBus(nil)
	src := make(chan *clientv1.Envelope, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bus.Run(ctx, src)

	_, loginCh := bus.Subscribe(func(e *clientv1.Envelope) bool { return e.GetLoginResp() != nil }, 2)
	src <- &clientv1.Envelope{Body: &clientv1.Envelope_HeartbeatResp{HeartbeatResp: &clientv1.HeartbeatResponse{}}}
	src <- &clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u1"}}}

	got := recvEnv(t, loginCh)
	require.Equal(t, "u1", got.GetLoginResp().GetUserId())
}

func TestEventBusUnsubscribeClosesChannel(t *testing.T) {
	bus := NewEventBus(nil)
	src := make(chan *clientv1.Envelope, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bus.Run(ctx, src)

	id, ch := bus.Subscribe(nil, 1)
	bus.Unsubscribe(id)
	_, ok := <-ch
	require.False(t, ok, "Unsubscribe 后通道应被关闭")
}

func TestEventBusDropsWhenSubscriberSlow(t *testing.T) {
	bus := NewEventBus(nil)
	src := make(chan *clientv1.Envelope, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bus.Run(ctx, src)

	_, ch := bus.Subscribe(nil, 1)
	for i := 0; i < 5; i++ {
		src <- &clientv1.Envelope{ReqId: "x"}
	}
	// 给 fanout 时间投递；订阅者 ch capacity=1，会丢弃多数事件
	time.Sleep(20 * time.Millisecond)
	count := 0
LOOP:
	for {
		select {
		case <-ch:
			count++
		default:
			break LOOP
		}
	}
	require.LessOrEqualf(t, count, 1, "缓冲为 1 的订阅者最多收到 1 条,丢弃保护生效")
}

func TestEventBusAppliesToState(t *testing.T) {
	state := NewAppState("racoo")
	bus := NewEventBus(state)
	src := make(chan *clientv1.Envelope, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bus.Run(ctx, src)
	_, ch := bus.Subscribe(nil, 1)
	src <- &clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u1", SessionToken: "tok"}}}
	recvEnv(t, ch)
	require.Eventuallyf(t, func() bool { return state.Snapshot().UserID == "u1" }, time.Second, 10*time.Millisecond, "state 应在 fanout 之前被 Apply")
}

func recvEnv(t *testing.T, ch <-chan *clientv1.Envelope) *clientv1.Envelope {
	t.Helper()
	select {
	case env := <-ch:
		require.NotNil(t, env)
		return env
	case <-time.After(time.Second):
		t.Fatal("等待 envelope 超时")
		return nil
	}
}
