package room

import (
	"context"
	"sync"
	"time"

	"racoo.cn/lsp/internal/clock"
)

type roomScheduler struct {
	roomID string
	clk    clock.Clock
	cfg    TimeoutConfig
	actor  *roomActor

	mu    sync.Mutex
	timer clock.Timer
}

func newRoomScheduler(roomID string, clk clock.Clock, cfg TimeoutConfig, actor *roomActor) *roomScheduler {
	if clk == nil {
		clk = clock.NewReal()
	}
	return &roomScheduler{
		roomID: roomID,
		clk:    clk,
		cfg:    cfg.withDefaults(),
		actor:  actor,
	}
}

func (s *roomScheduler) reset(rs *RoundState) {
	if s == nil {
		return
	}
	d := s.durationFor(rs)
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if d > 0 {
		s.timer = s.clk.AfterFunc(d, s.fire)
	}
	s.mu.Unlock()
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

func (s *roomScheduler) durationFor(rs *RoundState) time.Duration {
	if rs == nil || rs.closed {
		return 0
	}
	switch {
	case rs.waitingExchange:
		return s.cfg.ExchangeThree
	case rs.waitingQueMen:
		return s.cfg.QueMen
	case rs.claimWindowOpen:
		return s.cfg.ClaimWindow
	case rs.waitingTsumo:
		return s.cfg.TsumoWindow
	case rs.waitingDiscard:
		return s.cfg.Discard
	default:
		return 0
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

func (a *roomActor) resetScheduler() {
	if a == nil || a.scheduler == nil {
		return
	}
	a.scheduler.reset(a.round)
}
