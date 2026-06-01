package room

import (
	"context"
	"sync"
	"time"

	"racoo.cn/lsp/internal/clock"
)

// roomScheduler 仅负责按 RoundState.Deadline() 对齐 OS 定时器；
// 不再持有等待态推理或时长配置（参见 ADR-0045，时长来源是 RoundState.tmo.DurationFor）。
type roomScheduler struct {
	roomID string
	clk    clock.Clock
	actor  *roomActor

	mu    sync.Mutex
	timer clock.Timer
}

func newRoomScheduler(roomID string, clk clock.Clock, actor *roomActor) *roomScheduler {
	if clk == nil {
		clk = clock.NewReal()
	}
	return &roomScheduler{roomID: roomID, clk: clk, actor: actor}
}

// armUntil 把 OS 定时器对齐到 deadlineUnixMs；deadlineUnixMs<=0 表示停表。
// 该方法是 scheduler 与 RoundState 之间唯一的耦合点：只读 deadlineUnixMs，不再写 RoundState。
func (s *roomScheduler) armUntil(deadlineUnixMs int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if deadlineUnixMs <= 0 {
		return
	}
	d := time.Duration(deadlineUnixMs-s.clk.Now().UnixMilli()) * time.Millisecond
	if d <= 0 {
		// 已过期：立即触发 fire，避免向客户端发出"已是过去时间"的 deadline。
		go s.fire()
		return
	}
	s.timer = s.clk.AfterFunc(d, s.fire)
}

func (s *roomScheduler) stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

func (s *roomScheduler) fire() {
	if s == nil || s.actor == nil {
		return
	}
	notifications, err := s.actor.submitAutoTimeout(context.Background())
	if err != nil || len(notifications) == 0 {
		return
	}
	if s.actor.onAuto != nil {
		s.actor.onAuto(context.Background(), s.roomID, notifications)
	}
}

// resetScheduler 读取 RoundState 当前派生 deadline 对齐 OS 定时器。
// engine 在 enterPhase 时已写入 deadlineUnixMs，actor 仅需 arm。
func (a *roomActor) resetScheduler() {
	if a == nil || a.scheduler == nil {
		return
	}
	deadline := int64(0)
	if a.round != nil && !a.round.IsClosed() {
		deadline = a.round.Deadline()
	}
	a.scheduler.armUntil(deadline)
}
