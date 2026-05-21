package room

import (
	"context"
	"fmt"

	"racoo.cn/lsp/internal/mahjong/rules"
)

// ApplyTimeout 执行服务端超时兜底；真人座位超时不得替玩家选择收益动作。
func (e *Engine) ApplyTimeout(ctx context.Context, rs *RoundState) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, fmt.Errorf("round closed")
	}
	rs.ensureRuleRuntime()
	if rs.waitingOpening {
		step, ok := rs.currentOpeningStep()
		if !ok {
			return nil, fmt.Errorf("round not waiting for opening action")
		}
		for seat, done := range rs.openingSubmitted(step.Action) {
			if !done && !rs.isSurrendered(Seat(seat)) {
				return e.surrenderOpeningSeat(rs, Seat(seat), step)
			}
		}
	}
	if rs.claimWindowOpen {
		candidate, ok := rs.bestClaimCandidate()
		if !ok {
			rs.clearClaimWindow()
			rs.closeOpeningClaimWindow()
			return e.drawForCurrentTurn(rs)
		}
		rs.markSurrendered(candidate.seat)
		return e.ApplyPass(ctx, rs, candidate.seat)
	}
	if rs.waitingTsumo {
		return e.surrenderTurnAndContinue(ctx, rs, rs.turn)
	}
	if rs.waitingDiscard {
		return e.surrenderTurnAndContinue(ctx, rs, rs.turn)
	}
	return nil, fmt.Errorf("round not waiting for action")
}

func (e *Engine) surrenderOpeningSeat(rs *RoundState, seat Seat, step *rules.OpeningStep) ([]Notification, error) {
	if step == nil {
		return nil, fmt.Errorf("round not waiting for opening action")
	}
	rs.markSurrendered(seat)
	return e.applyOpeningAction(rs, seat, rules.OpeningActionName(step.Action), nil, 0, 0, nil, true, true)
}

func (rs *RoundState) isSurrendered(seat Seat) bool {
	return rs != nil && int(seat) >= 0 && int(seat) < len(rs.surrendered) && rs.surrendered[seat]
}

func (rs *RoundState) markSurrendered(seat Seat) {
	if rs == nil || seat < 0 || seat > 3 {
		return
	}
	for len(rs.surrendered) < 4 {
		rs.surrendered = append(rs.surrendered, false)
	}
	rs.surrendered[seat] = true
}

func (e *Engine) surrenderTurnAndContinue(ctx context.Context, rs *RoundState, seat Seat) ([]Notification, error) {
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	rs.markSurrendered(seat)
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.clearClaimWindow()
	rs.closeOpeningClaimWindow()
	if rs.activeCount() <= 1 || rs.shouldFinishRound() {
		settlement, err := rs.finishRound()
		if err != nil {
			return nil, err
		}
		return []Notification{settlement}, nil
	}
	rs.turn = rs.nextActiveSeat(seat)
	rs.enterPhase(ReasonNone)
	return e.drawForCurrentTurn(rs)
}
