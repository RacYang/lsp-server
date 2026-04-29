package room

import (
	"context"
	"fmt"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/metrics"
)

// ApplyPong 处理弃牌抢答窗口中的碰牌动作，并中断原本轮到的座位。
func (e *Engine) ApplyPong(_ context.Context, rs *RoundState, seat int) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if seat < 0 || seat > 3 {
		return nil, fmt.Errorf("invalid seat")
	}
	if !rs.canClaimPong(seat) {
		return nil, fmt.Errorf("pong not allowed")
	}
	metrics.ClaimWindowTotal.WithLabelValues("pong").Inc()
	claimedTile := rs.lastDiscard
	claimedFromSeat := rs.lastDiscardSeat
	rs.clearClaimWindow()
	rs.closeOpeningClaimWindow()
	if err := rs.rewindInterruptedTurn(); err != nil {
		return nil, err
	}
	for i := 0; i < 2; i++ {
		if err := rs.hands[seat].Remove(claimedTile); err != nil {
			return nil, fmt.Errorf("consume pong tiles: %w", err)
		}
	}
	rs.removeLastDiscard(claimedFromSeat, claimedTile)
	rs.recordMeld(seat, "pong:"+claimedTile.String())
	rs.turn = seat
	rs.waitingDiscard = true
	rs.waitingTsumo = false
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.lastDiscard = 0
	rs.lastDiscardSeat = -1
	seatIndex := int32(seat) //nolint:gosec // seat 范围固定
	payload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("pong-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "pong", Tile: claimedTile.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	out := []Notification{{Kind: KindAction, Payload: payload, TargetSeat: BroadcastSeat}}
	discard := chooseDiscard(rs.hands[seat], tile.Suit(rs.queBySeat[seat]))
	next, err := e.ApplyDiscard(context.Background(), rs, seat, discard.String())
	if err != nil {
		return nil, err
	}
	return append(out, next...), nil
}

// ApplyGang 处理弃牌抢杠或当前座位自杠，并继续摸补牌。
func (e *Engine) ApplyGang(_ context.Context, rs *RoundState, seat int, tileText string) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if seat < 0 || seat > 3 {
		return nil, fmt.Errorf("invalid seat")
	}
	claimGang := rs.canClaimGang(seat)
	selfGang := rs.canSelfGang(seat, tileText)
	if !claimGang && !selfGang {
		return nil, fmt.Errorf("gang not allowed")
	}
	if claimGang {
		metrics.ClaimWindowTotal.WithLabelValues("gang").Inc()
	}
	var gangTile tile.Tile
	var err error
	var out []Notification
	if claimGang {
		gangTile = rs.lastDiscard
		fromSeat := rs.lastDiscardSeat
		rs.clearClaimWindow()
		rs.closeOpeningClaimWindow()
		if err := rs.rewindInterruptedTurn(); err != nil {
			return nil, err
		}
		for i := 0; i < 3; i++ {
			if err := rs.hands[seat].Remove(gangTile); err != nil {
				return nil, fmt.Errorf("consume gang tiles: %w", err)
			}
		}
		rs.removeLastDiscard(fromSeat, gangTile)
		rs.recordMeld(seat, "gang:"+gangTile.String())
		rs.lastDiscard = 0
		rs.lastDiscardSeat = -1
		appendGangEntries(rs, seat, gangTile, rules.GangKindMing, fromSeat)
	} else {
		gangTile, err = tile.Parse(tileText)
		if err != nil {
			return nil, fmt.Errorf("parse gang tile: %w", err)
		}
		rs.lastDiscard = gangTile
		rs.lastDiscardSeat = seat
		rs.qiangGangWindow = true
		rs.claimCandidates = rs.buildClaimCandidates()
		if len(rs.claimCandidates) > 0 {
			rs.claimWindowOpen = true
			return rs.claimPromptNotifications(gangTile)
		}
		rs.clearClaimWindow()
		for i := 0; i < 4; i++ {
			if err := rs.hands[seat].Remove(gangTile); err != nil {
				return nil, fmt.Errorf("consume self gang tiles: %w", err)
			}
		}
		rs.recordMeld(seat, "gang:"+gangTile.String())
		appendGangEntries(rs, seat, gangTile, rules.GangKindAn, -1)
	}
	rs.turn = seat
	rs.waitingDiscard = false
	rs.waitingTsumo = false
	rs.pendingDraw = 0
	rs.currentDraw = 0
	seatIndex := int32(seat) //nolint:gosec // seat 范围固定
	payload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("gang-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "gang", Tile: gangTile.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	out = append(out, Notification{Kind: KindAction, Payload: payload, TargetSeat: BroadcastSeat})
	next, err := e.drawForCurrentTurn(rs)
	if err != nil {
		return nil, err
	}
	return append(out, next...), nil
}

func (rs *RoundState) canClaimPong(seat int) bool {
	return rs != nil && !rs.isHued(seat) && rs.isTopClaimSeat(seat) && rs.hasClaimAction(seat, "pong")
}

func (rs *RoundState) canClaimGang(seat int) bool {
	return rs != nil && !rs.isHued(seat) && rs.isTopClaimSeat(seat) && rs.hasClaimAction(seat, "gang")
}

func (rs *RoundState) canSelfGang(seat int, tileText string) bool {
	if rs == nil || seat != rs.turn || !rs.waitingDiscard || rs.isHued(seat) {
		return false
	}
	target, err := tile.Parse(tileText)
	if err != nil {
		return false
	}
	count := 0
	for _, t := range rs.hands[seat].Tiles() {
		if t == target {
			count++
		}
	}
	return count >= 4
}

func (rs *RoundState) rewindInterruptedTurn() error {
	if rs == nil || rs.currentDraw == 0 {
		return nil
	}
	if rs.waitingTsumo {
		rs.pendingDraw = 0
		rs.waitingTsumo = false
	} else if rs.waitingDiscard {
		if err := rs.hands[rs.turn].Remove(rs.currentDraw); err != nil {
			return fmt.Errorf("rewind current draw: %w", err)
		}
		rs.waitingDiscard = false
	}
	if err := rs.wall.PushFront(rs.currentDraw); err != nil {
		return fmt.Errorf("restore draw to wall: %w", err)
	}
	rs.currentDraw = 0
	return nil
}

func (rs *RoundState) claimSeat() int {
	candidate, ok := rs.bestClaimCandidate()
	if !ok {
		return -1
	}
	return candidate.seat
}

func (rs *RoundState) openClaimWindow() {
	if rs == nil {
		return
	}
	rs.claimWindowOpen = true
	rs.claimCandidates = rs.buildClaimCandidates()
}

func (rs *RoundState) clearClaimWindow() {
	if rs == nil {
		return
	}
	rs.claimWindowOpen = false
	rs.claimCandidates = nil
	rs.qiangGangWindow = false
}

func (rs *RoundState) buildClaimCandidates() []claimCandidate {
	if rs == nil || rs.lastDiscard == 0 || rs.lastDiscardSeat < 0 {
		return nil
	}
	out := make([]claimCandidate, 0, 3)
	for offset := 1; offset < 4; offset++ {
		seat := (rs.lastDiscardSeat + offset) % 4
		if rs.isHued(seat) {
			continue
		}
		actions := make([]string, 0, 3)
		if rs.rawCanClaimHu(seat) {
			actions = append(actions, "hu")
		}
		if !rs.qiangGangWindow && rs.rawCanClaimGang(seat) {
			actions = append(actions, "gang")
		}
		if !rs.qiangGangWindow && rs.rawCanClaimPong(seat) {
			actions = append(actions, "pong")
		}
		if len(actions) > 0 {
			out = append(out, claimCandidate{seat: seat, actions: actions})
		}
	}
	return out
}

func (rs *RoundState) bestClaimCandidate() (claimCandidate, bool) {
	if rs == nil || !rs.claimWindowOpen || len(rs.claimCandidates) == 0 {
		return claimCandidate{}, false
	}
	best := rs.claimCandidates[0]
	bestPriority := claimPriority(best.actions)
	for _, candidate := range rs.claimCandidates[1:] {
		priority := claimPriority(candidate.actions)
		if priority > bestPriority {
			best = candidate
			bestPriority = priority
		}
	}
	return best, true
}

func (rs *RoundState) isTopClaimSeat(seat int) bool {
	candidate, ok := rs.bestClaimCandidate()
	return ok && candidate.seat == seat
}

func (rs *RoundState) hasClaimAction(seat int, action string) bool {
	if rs == nil || !rs.claimWindowOpen {
		return false
	}
	for _, candidate := range rs.claimCandidates {
		if candidate.seat == seat {
			return hasAction(candidate.actions, action)
		}
	}
	return false
}

func claimPriority(actions []string) int {
	if hasAction(actions, "hu") {
		return 3
	}
	if hasAction(actions, "gang") {
		return 2
	}
	if hasAction(actions, "pong") {
		return 1
	}
	return 0
}

func hasAction(actions []string, action string) bool {
	for _, current := range actions {
		if current == action {
			return true
		}
	}
	return false
}

func (rs *RoundState) claimPromptNotifications(discard tile.Tile) ([]Notification, error) {
	out := make([]Notification, 0, len(rs.claimCandidates))
	for _, candidate := range rs.claimCandidates {
		claimAction := "pong_choice"
		if hasAction(candidate.actions, "hu") {
			if rs.qiangGangWindow {
				claimAction = "qiang_gang_choice"
			} else {
				claimAction = "hu_choice"
			}
		} else if hasAction(candidate.actions, "gang") {
			claimAction = "gang_choice"
		}
		claimSeatIndex := int32(candidate.seat) //nolint:gosec // 座位范围固定
		claimPayload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("claim-%d-%d", rs.step, candidate.seat),
			Body: &clientv1.Envelope_Action{
				Action: &clientv1.ActionNotify{SeatIndex: claimSeatIndex, Action: claimAction, Tile: discard.String()},
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, Notification{Kind: KindAction, Payload: claimPayload, TargetSeat: BroadcastSeat})
	}
	return out, nil
}

func (rs *RoundState) rawCanClaimPong(seat int) bool {
	if rs == nil || rs.lastDiscard == 0 || rs.lastDiscardSeat < 0 || seat == rs.lastDiscardSeat || rs.isHued(seat) {
		return false
	}
	count := 0
	for _, t := range rs.hands[seat].Tiles() {
		if t == rs.lastDiscard {
			count++
		}
	}
	return count >= 2
}

func (rs *RoundState) rawCanClaimGang(seat int) bool {
	if rs == nil || rs.lastDiscard == 0 || rs.lastDiscardSeat < 0 || seat == rs.lastDiscardSeat || rs.isHued(seat) {
		return false
	}
	count := 0
	for _, t := range rs.hands[seat].Tiles() {
		if t == rs.lastDiscard {
			count++
		}
	}
	return count >= 3
}

func (rs *RoundState) rawCanClaimHu(seat int) bool {
	if rs == nil || rs.lastDiscard == 0 || rs.lastDiscardSeat < 0 || seat == rs.lastDiscardSeat || rs.isHued(seat) {
		return false
	}
	ctx := rules.HuContext{
		Source:          rules.HuSourceDiscard,
		PendingTile:     rs.lastDiscard,
		Discarder:       rs.lastDiscardSeat,
		ResponsibleSeat: rs.lastDiscardSeat,
		GangHistory:     append([]rules.GangRecord(nil), rs.gangRecords...),
		WallRemaining:   rs.wall.Remaining(),
	}
	if rs.qiangGangWindow {
		ctx.Source = rules.HuSourceQiangGang
	}
	_, ok := rs.rule.CheckHu(rs.hands[seat], rs.lastDiscard, ctx)
	return ok
}
