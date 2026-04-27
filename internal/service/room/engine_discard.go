package room

import (
	"context"
	"fmt"
	"sort"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/metrics"
)

// ApplyDiscard 推进当前轮次出牌，并在需要时继续发出下一次摸牌或结算。
func (e *Engine) ApplyDiscard(ctx context.Context, rs *RoundState, seat int, tileText string) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, fmt.Errorf("round closed")
	}
	if seat != rs.turn {
		return nil, fmt.Errorf("not your turn")
	}
	discard, err := tile.Parse(tileText)
	if err != nil {
		return nil, fmt.Errorf("parse discard tile: %w", err)
	}
	if rs.waitingTsumo {
		rs.hands[seat].Add(rs.pendingDraw)
		rs.pendingDraw = 0
		rs.waitingTsumo = false
		rs.waitingDiscard = true
	}
	if !rs.waitingDiscard {
		return nil, fmt.Errorf("round not waiting discard")
	}
	if err := rs.hands[seat].Remove(discard); err != nil {
		return nil, fmt.Errorf("discard tile from hand: %w", err)
	}
	rs.waitingDiscard = false
	rs.currentDraw = 0
	seatIndex := int32(seat) //nolint:gosec // seat 范围固定
	actionPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("discard-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "discard", Tile: discard.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	out := []Notification{{Kind: KindAction, Payload: actionPayload}}
	rs.step++
	if rs.shouldFinishRound() {
		settlement, err := rs.finishRound()
		if err != nil {
			return nil, err
		}
		out = append(out, settlement)
		return out, nil
	}
	rs.lastDiscard = discard
	rs.lastDiscardSeat = seat
	rs.turn = rs.nextActiveSeat(seat)
	rs.openClaimWindow()
	if len(rs.claimCandidates) > 0 {
		metrics.ClaimWindowTotal.WithLabelValues("open").Inc()
		claimPrompts, err := rs.claimPromptNotifications(discard)
		if err != nil {
			return nil, err
		}
		return append(out, claimPrompts...), nil
	}
	rs.clearClaimWindow()
	next, err := e.drawForCurrentTurn(rs)
	if err != nil {
		return nil, err
	}
	_ = ctx
	return append(out, next...), nil
}

// ApplyHu 处理当前轮次的自摸胡牌或弃牌抢答胡牌。
func (e *Engine) ApplyHu(ctx context.Context, rs *RoundState, seat int) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, fmt.Errorf("round closed")
	}
	if rs.isHued(seat) {
		return nil, fmt.Errorf("hu already done")
	}
	var (
		winTile      tile.Tile
		source       = rules.HuSourceTsumo
		nextTurnFrom int
	)
	switch {
	case rs.claimWindowOpen && rs.hasClaimAction(seat, "hu"):
		if !rs.isTopClaimSeat(seat) {
			return nil, fmt.Errorf("hu not allowed")
		}
		winTile = rs.lastDiscard
		source = rules.HuSourceDiscard
		if rs.qiangGangWindow {
			source = rules.HuSourceQiangGang
		}
		nextTurnFrom = rs.lastDiscardSeat
	case seat == rs.turn && rs.waitingTsumo:
		winTile = rs.pendingDraw
		nextTurnFrom = seat
	default:
		return nil, fmt.Errorf("hu not allowed")
	}
	result, ok := rs.rule.CheckHu(rs.hands[seat], winTile, rules.HuContext{
		Source:        source,
		PendingTile:   winTile,
		Discarder:     rs.lastDiscardSeat,
		WallRemaining: rs.wall.Remaining(),
	})
	if !ok {
		return nil, fmt.Errorf("hu not allowed")
	}
	fanTotal := rs.rule.ScoreFans(result, rules.ScoreContext{IsTsumo: source == rules.HuSourceTsumo, WallRemaining: rs.wall.Remaining()}).Total
	rs.totalFanBySeat[seat] = int32(fanTotal) //nolint:gosec // 番数极小
	rs.markHued(seat)
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.waitingTsumo = false
	rs.waitingDiscard = false
	rs.clearClaimWindow()
	if source != rules.HuSourceTsumo {
		metrics.ClaimWindowTotal.WithLabelValues("hu").Inc()
	}
	seatIndex := int32(seat) //nolint:gosec // seat 范围固定
	huPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("hu-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "hu", Tile: winTile.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	out := []Notification{{Kind: KindAction, Payload: huPayload}}
	if rs.shouldFinishRound() {
		settlement, err := rs.finishRound()
		if err != nil {
			return nil, err
		}
		return append(out, settlement), nil
	}
	rs.turn = rs.nextActiveSeat(nextTurnFrom)
	next, err := e.drawForCurrentTurn(rs)
	if err != nil {
		return nil, err
	}
	_ = ctx
	return append(out, next...), nil
}

func (e *Engine) drawForCurrentTurn(rs *RoundState) ([]Notification, error) {
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.shouldFinishRound() {
		settlement, err := rs.finishRound()
		if err != nil {
			return nil, err
		}
		return []Notification{settlement}, nil
	}
	drawn, err := rs.wall.Draw()
	if err != nil {
		return nil, err
	}
	seatIndex := int32(rs.turn) //nolint:gosec // turn 范围固定
	drawPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("draw-%d", rs.step),
		Body: &clientv1.Envelope_DrawTile{
			DrawTile: &clientv1.DrawTileNotify{SeatIndex: seatIndex, Tile: drawn.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	rs.currentDraw = drawn
	out := []Notification{{Kind: KindDrawTile, Payload: drawPayload}}
	if _, ok := rs.rule.CheckHu(rs.hands[rs.turn], drawn, rules.HuContext{}); ok {
		rs.pendingDraw = drawn
		rs.waitingTsumo = true
		rs.waitingDiscard = false
		choicePayload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("tsumo-choice-%d", rs.step),
			Body: &clientv1.Envelope_Action{
				Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "tsumo_choice", Tile: drawn.String()},
			},
		})
		if err != nil {
			return nil, err
		}
		return append(out, Notification{Kind: KindAction, Payload: choicePayload}), nil
	}
	rs.hands[rs.turn].Add(drawn)
	rs.waitingDiscard = true
	return out, nil
}

func (rs *RoundState) isHued(seat int) bool {
	return rs != nil && seat >= 0 && seat < len(rs.huedSeats) && rs.huedSeats[seat]
}

func (rs *RoundState) markHued(seat int) {
	if rs == nil || seat < 0 || seat > 3 || rs.isHued(seat) {
		return
	}
	for len(rs.huedSeats) < 4 {
		rs.huedSeats = append(rs.huedSeats, false)
	}
	rs.huedSeats[seat] = true
	rs.winnerSeats = append(rs.winnerSeats, seat)
}

func (rs *RoundState) huedCount() int {
	if rs == nil {
		return 0
	}
	n := 0
	for _, hued := range rs.huedSeats {
		if hued {
			n++
		}
	}
	return n
}

func (rs *RoundState) nextActiveSeat(from int) int {
	if rs == nil {
		return from
	}
	for offset := 1; offset <= 4; offset++ {
		seat := (from + offset) % 4
		if !rs.isHued(seat) {
			return seat
		}
	}
	return from
}

func (rs *RoundState) shouldFinishRound() bool {
	if rs == nil {
		return true
	}
	return rs.rule.GameOver(rules.GameState{HuedPlayers: rs.huedCount(), WallRemaining: rs.wall.Remaining()})
}

func (rs *RoundState) waitingKind() string {
	if rs == nil {
		return "none"
	}
	switch {
	case rs.waitingExchange:
		return "exchange_three"
	case rs.waitingQueMen:
		return "que_men"
	case rs.claimWindowOpen:
		return "claim_window"
	case rs.waitingTsumo:
		return "tsumo_window"
	case rs.waitingDiscard:
		return "discard"
	default:
		return "none"
	}
}

func chooseDiscard(h *hand.Hand, queSuit tile.Suit) tile.Tile {
	ts := append([]tile.Tile(nil), h.Tiles()...)
	sort.Slice(ts, func(i, j int) bool {
		if ts[i].Suit() != ts[j].Suit() {
			return ts[i].Suit() < ts[j].Suit()
		}
		return ts[i].Rank() > ts[j].Rank()
	})
	for _, t := range ts {
		if t.Suit() == queSuit {
			return t
		}
	}
	return ts[0]
}
