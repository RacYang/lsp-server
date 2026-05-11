package room

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/metrics"
)

// ApplyDiscard 推进当前轮次出牌，并在需要时继续发出下一次摸牌或结算。
func (e *Engine) ApplyDiscard(ctx context.Context, rs *RoundState, seat Seat, tileText string) ([]Notification, error) {
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
	if seat == rs.dealerSeat && rs.openingDrawSeat == seat {
		rs.dealerFirstDiscardOpen = true
		rs.openingDrawSeat = -1
	}
	rs.lastDiscardAfterGang = rs.lastGangFollowUp
	rs.lastGangFollowUp = false
	seatIndex := seat.Proto()
	detail := rs.actionDetail(seat, "discard", discard, SeatInvalid, seat)
	rs.rememberLastAction(detail)
	actionPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("discard-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{
				SeatIndex:     seatIndex,
				Action:        "discard",
				Tile:          discard.String(),
				Phase:         clientv1.Phase_PHASE_CLAIM,
				Step:          int64(rs.step),
				Detail:        detail,
				WallRemaining: rs.wallRemaining(),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	rs.recordDiscard(seat, discard)
	out := []Notification{{Kind: KindAction, Payload: actionPayload, TargetSeat: BroadcastSeat}}
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
	progress := rs.drawTransitionProgress()
	if len(rs.claimCandidates) > 0 {
		progress = rs.roundProgress()
		if actionEnv := new(clientv1.Envelope); proto.Unmarshal(actionPayload, actionEnv) == nil {
			if action := actionEnv.GetAction(); action != nil {
				progress.applyToAction(action)
				if payload, err := marshalEnvelope(actionEnv); err == nil {
					out[0].Payload = payload
				}
			}
		}
		metrics.ClaimWindowTotal.WithLabelValues("open").Inc()
		claimPrompts, err := rs.claimPromptNotifications(discard)
		if err != nil {
			return nil, err
		}
		return append(out, claimPrompts...), nil
	}
	rs.clearClaimWindow()
	rs.closeOpeningClaimWindow()
	if actionEnv := new(clientv1.Envelope); proto.Unmarshal(actionPayload, actionEnv) == nil {
		if action := actionEnv.GetAction(); action != nil {
			progress.applyToAction(action)
			if payload, err := marshalEnvelope(actionEnv); err == nil {
				out[0].Payload = payload
			}
		}
	}
	next, err := e.drawForCurrentTurn(rs)
	if err != nil {
		return nil, err
	}
	_ = ctx
	return append(out, next...), nil
}

// ApplyHu 处理当前轮次的自摸胡牌或弃牌抢答胡牌。
func (e *Engine) ApplyHu(ctx context.Context, rs *RoundState, seat Seat) ([]Notification, error) {
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
		nextTurnFrom Seat
		payer        = SeatInvalid
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
		payer = rs.lastDiscardSeat
		nextTurnFrom = rs.lastDiscardSeat
	case seat == rs.turn && rs.waitingTsumo:
		winTile = rs.pendingDraw
		nextTurnFrom = seat
	default:
		return nil, fmt.Errorf("hu not allowed")
	}
	result, ok := rs.rule.CheckHu(rs.hands[seat], winTile, rules.HuContext{
		Source:          source,
		PendingTile:     winTile,
		Que:             queSuits(rs.queBySeat),
		Discarder:       rs.lastDiscardSeat,
		IsHaiDi:         rs.isHaiDi(),
		IsGangShangHua:  source == rules.HuSourceTsumo && rs.lastGangFollowUp,
		ResponsibleSeat: payer,
		GangHistory:     append([]rules.GangRecord(nil), rs.gangRecords...),
		WallRemaining:   rs.wall.Remaining(),
	})
	if !ok {
		return nil, fmt.Errorf("hu not allowed")
	}
	breakdown := rs.rule.ScoreFans(result, rules.ScoreContext{
		HuSeat:               seat,
		DealerSeat:           rs.dealerSeat,
		GangRecords:          append([]rules.GangRecord(nil), rs.gangRecords...),
		IsTsumo:              source == rules.HuSourceTsumo,
		IsOpeningDraw:        rs.isOpeningDrawHu(seat, source),
		IsDealerFirstDiscard: rs.isDealerFirstDiscardHu(source),
		IsHaiDi:              rs.isHaiDi(),
		IsGangShangHua:       source == rules.HuSourceTsumo && rs.lastGangFollowUp,
		IsGangShangPao:       source != rules.HuSourceTsumo && rs.lastDiscardAfterGang,
		Que:                  append([]tile.Suit(nil), queSuits(rs.queBySeat)...),
		ResponsibleSeat:      payer,
		WallRemaining:        rs.wall.Remaining(),
	})
	appendHuEntries(rs, seat, breakdown.Total, source, payer, breakdown)
	rs.markHued(seat)
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.lastGangFollowUp = false
	rs.lastDiscardAfterGang = false
	rs.closeOpeningHuWindow(source)
	rs.waitingTsumo = false
	rs.waitingDiscard = false
	rs.clearClaimWindow()
	if source != rules.HuSourceTsumo {
		metrics.ClaimWindowTotal.WithLabelValues("hu").Inc()
	}
	seatIndex := seat.Proto()
	detail := rs.actionDetail(seat, "hu", winTile, seat, payer)
	rs.rememberLastAction(detail)
	progress := rs.roundProgress()
	huPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("hu-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{
				SeatIndex: seatIndex,
				Action:    "hu",
				Tile:      winTile.String(),
				Detail:    detail,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	out := []Notification{{Kind: KindAction, Payload: huPayload, TargetSeat: BroadcastSeat}}
	if actionEnv := new(clientv1.Envelope); proto.Unmarshal(out[0].Payload, actionEnv) == nil {
		if action := actionEnv.GetAction(); action != nil {
			progress.applyToAction(action)
			if payload, err := marshalEnvelope(actionEnv); err == nil {
				out[0].Payload = payload
			}
		}
	}
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

func (rs *RoundState) isOpeningDrawHu(seat Seat, source rules.HuSource) bool {
	return rs != nil && source == rules.HuSourceTsumo && seat == rs.openingDrawSeat
}

func (rs *RoundState) isDealerFirstDiscardHu(source rules.HuSource) bool {
	return rs != nil && source != rules.HuSourceTsumo && rs.dealerFirstDiscardOpen
}

func (rs *RoundState) closeOpeningHuWindow(source rules.HuSource) {
	if rs == nil {
		return
	}
	if source == rules.HuSourceTsumo {
		rs.openingDrawSeat = -1
		return
	}
	rs.closeOpeningClaimWindow()
}

func (rs *RoundState) closeOpeningClaimWindow() {
	if rs == nil {
		return
	}
	rs.dealerFirstDiscardOpen = false
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
	seatIndex := rs.turn.Proto()
	rs.currentDraw = drawn
	if _, ok := rs.rule.CheckHu(rs.hands[rs.turn], drawn, rules.HuContext{}); ok {
		rs.pendingDraw = drawn
		rs.waitingTsumo = true
		rs.waitingDiscard = false
		progress := rs.roundProgress()
		drawPayload, err := drawTilePayload(fmt.Sprintf("draw-%d", rs.step), seatIndex, drawn.String(), progress, true)
		if err != nil {
			return nil, err
		}
		out := []Notification{{
			Kind:       KindDrawTile,
			Payload:    drawPayload,
			TargetSeat: BroadcastSeat,
			Privacy:    PrivacyPerSeat,
			Project: func(target Seat) []byte {
				visible := target == rs.turn
				payload, err := drawTilePayload(fmt.Sprintf("draw-%d", rs.step), seatIndex, drawn.String(), progress, visible)
				if err != nil {
					return drawPayload
				}
				return payload
			},
		}}
		choice := &clientv1.ActionNotify{
			SeatIndex: seatIndex,
			Action:    "tsumo_choice",
			Tile:      drawn.String(),
		}
		progress.applyToAction(choice)
		choicePayload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("tsumo-choice-%d", rs.step),
			Body:  &clientv1.Envelope_Action{Action: choice},
		})
		if err != nil {
			return nil, err
		}
		return append(out, Notification{Kind: KindAction, Payload: choicePayload, TargetSeat: BroadcastSeat}), nil
	}
	rs.hands[rs.turn].Add(drawn)
	rs.waitingDiscard = true
	progress := rs.roundProgress()
	drawPayload, err := drawTilePayload(fmt.Sprintf("draw-%d", rs.step), seatIndex, drawn.String(), progress, true)
	if err != nil {
		return nil, err
	}
	out := []Notification{{
		Kind:       KindDrawTile,
		Payload:    drawPayload,
		TargetSeat: BroadcastSeat,
		Privacy:    PrivacyPerSeat,
		Project: func(target Seat) []byte {
			visible := target == rs.turn
			payload, err := drawTilePayload(fmt.Sprintf("draw-%d", rs.step), seatIndex, drawn.String(), progress, visible)
			if err != nil {
				return drawPayload
			}
			return payload
		},
	}}
	return out, nil
}

func drawTilePayload(reqID string, seatIndex int32, tileText string, progress RoundProgress, visible bool) ([]byte, error) {
	if !visible {
		tileText = ""
	}
	draw := &clientv1.DrawTileNotify{
		SeatIndex: seatIndex,
		Tile:      tileText,
	}
	progress.applyToDraw(draw)
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: reqID,
		Body: &clientv1.Envelope_DrawTile{
			DrawTile: draw,
		},
	})
}

func (rs *RoundState) isHaiDi() bool {
	return rs != nil && rs.wall != nil && rs.wall.Remaining() == 0
}

func queSuits(raw []int32) []tile.Suit {
	out := make([]tile.Suit, 0, len(raw))
	for _, suit := range raw {
		if suit >= 0 && suit <= 2 {
			out = append(out, tile.Suit(suit))
		}
	}
	return out
}

func (rs *RoundState) isHued(seat Seat) bool {
	return rs != nil && seat >= 0 && int(seat) < len(rs.huedSeats) && rs.huedSeats[seat]
}

func (rs *RoundState) markHued(seat Seat) {
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

func (rs *RoundState) nextActiveSeat(from Seat) Seat {
	if rs == nil {
		return from
	}
	for offset := 1; offset <= 4; offset++ {
		seat := Seat((int(from) + offset) % 4)
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
