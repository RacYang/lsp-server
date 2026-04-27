package room

import (
	"context"
	"fmt"

	"racoo.cn/lsp/internal/mahjong/tile"
)

// ApplyTimeout 执行服务端超时/托管动作；调用方可在定时器到期后提交到同一 room actor。
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
	if rs.waitingExchange {
		for seat, done := range rs.exchangeSubmitted {
			if !done {
				return e.ApplyExchangeThree(ctx, rs, seat, nil, defaultExchangeDirection)
			}
		}
	}
	if rs.waitingQueMen {
		for seat, done := range rs.queSubmitted {
			if !done {
				return e.ApplyQueMen(ctx, rs, seat, 0)
			}
		}
	}
	if rs.claimWindowOpen {
		candidate, ok := rs.bestClaimCandidate()
		if !ok {
			rs.clearClaimWindow()
			return e.drawForCurrentTurn(rs)
		}
		if hasAction(candidate.actions, "hu") {
			return e.ApplyHu(ctx, rs, candidate.seat)
		}
		if hasAction(candidate.actions, "gang") {
			return e.ApplyGang(ctx, rs, candidate.seat, rs.lastDiscard.String())
		}
		return e.ApplyPong(ctx, rs, candidate.seat)
	}
	if rs.waitingTsumo {
		return e.ApplyDiscard(ctx, rs, rs.turn, rs.pendingDraw.String())
	}
	if rs.waitingDiscard {
		discard := chooseDiscard(rs.hands[rs.turn], tile.Suit(rs.queBySeat[rs.turn]))
		return e.ApplyDiscard(ctx, rs, rs.turn, discard.String())
	}
	return nil, fmt.Errorf("round not waiting for action")
}
