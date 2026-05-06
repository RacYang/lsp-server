package main

import (
	"context"
	"sync"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// EventBus 把 WSClient.Events 这条单一事件流扇出给多个消费者。
//
// 设计目标：
//   - 始终把 envelope 应用到 AppState，保证全局视图最新；
//   - 让 lobby gateway / 牌桌主循环 / 紧急事件 goroutine 各自只看关心的子集；
//   - 订阅者来不及消费时丢弃事件而非阻塞总线，由订阅者调高 capacity 自行兜底。
//
// 总线本身不持有 src 通道；调用方启动一个 goroutine 跑 Run 把外部 channel 的事件喂进来。
type EventBus struct {
	state *AppState

	mu        sync.RWMutex
	subs      map[uint64]*subscription
	nextSubID uint64
	closed    bool
}

type subscription struct {
	id    uint64
	match func(*clientv1.Envelope) bool
	ch    chan *clientv1.Envelope
}

// NewEventBus 创建总线；state 用于在每条 envelope 上自动 Apply。
func NewEventBus(state *AppState) *EventBus {
	return &EventBus{
		state: state,
		subs:  make(map[uint64]*subscription),
	}
}

// Subscribe 注册一个订阅，返回一个 id（用于 Unsubscribe）和 receive-only 通道。
//
// match 为 nil 时接收所有 envelope；capacity 决定订阅者通道的缓冲，
// 缓冲满时新事件会被丢弃以保护总线吞吐。
func (b *EventBus) Subscribe(match func(*clientv1.Envelope) bool, capacity int) (uint64, <-chan *clientv1.Envelope) {
	if capacity <= 0 {
		capacity = 16
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextSubID++
	id := b.nextSubID
	sub := &subscription{
		id:    id,
		match: match,
		ch:    make(chan *clientv1.Envelope, capacity),
	}
	b.subs[id] = sub
	return id, sub.ch
}

// Unsubscribe 取消订阅；调用后通道由 EventBus 关闭，订阅者读到 zero envelope 即终止。
func (b *EventBus) Unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(sub.ch)
	}
}

// Run 阻塞地把 src 上的 envelope 喂进总线，直到 ctx 取消或 src 关闭。
//
// 退出时关闭所有订阅通道，订阅者据此清理资源。
func (b *EventBus) Run(ctx context.Context, src <-chan *clientv1.Envelope) {
	defer b.shutdown()
	for {
		select {
		case <-ctx.Done():
			return
		case env, ok := <-src:
			if !ok {
				return
			}
			if b.state != nil {
				b.state.Apply(env)
			}
			b.fanout(env)
		}
	}
}

// fanout 在读锁保护下把 envelope 推给所有匹配的订阅者。
//
// 推送时使用非阻塞 send：订阅者 channel 满则丢弃。
func (b *EventBus) fanout(env *clientv1.Envelope) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subs {
		if sub.match != nil && !sub.match(env) {
			continue
		}
		select {
		case sub.ch <- env:
		default:
		}
	}
}

func (b *EventBus) shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, sub := range b.subs {
		close(sub.ch)
		delete(b.subs, id)
	}
}
