package room

import (
	"context"
	"fmt"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/metrics"
)

// ApplyPong 兼容旧调用方，按玩家显式碰牌处理。
func (e *Engine) ApplyPong(ctx context.Context, rs *RoundState, seat Seat) ([]Notification, error) {
	return e.ApplyPongByPlayer(ctx, rs, seat)
}

// ApplyPongByPlayer 处理玩家显式碰牌，并中断原本轮到的座位。
func (e *Engine) ApplyPongByPlayer(_ context.Context, rs *RoundState, seat Seat) ([]Notification, error) {
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
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.lastDiscard = 0
	rs.lastDiscardSeat = SeatInvalid
	rs.enterPhase(ReasonDiscard)
	seatIndex := seat.Proto()
	detail := rs.actionDetail(seat, "pong", claimedTile, seat, claimedFromSeat)
	rs.rememberLastAction(detail)
	progress := rs.roundProgress()
	action := &clientv1.ActionNotify{
		SeatIndex: seatIndex,
		Action:    "pong",
		Tile:      claimedTile.String(),
		Detail:    detail,
	}
	progress.applyToAction(action)
	payload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("pong-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: action,
		},
	})
	if err != nil {
		return nil, err
	}
	return []Notification{{Kind: KindAction, Payload: payload, TargetSeat: BroadcastSeat}}, nil
}

// ApplyPongByTimeout 为托管座位处理碰牌，并立即选择一张牌托管打出。
func (e *Engine) ApplyPongByTimeout(ctx context.Context, rs *RoundState, seat Seat) ([]Notification, error) {
	out, err := e.ApplyPongByPlayer(ctx, rs, seat)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115：queBySeat 仅在 0..2 范围（三种花色），不会溢出 byte
	discard := chooseDiscard(rs.hands[seat], tile.Suit(rs.queBySeat[seat]))
	next, err := e.ApplyDiscard(ctx, rs, seat, discard.String())
	if err != nil {
		return nil, err
	}
	return append(out, next...), nil
}

// ApplyGang 处理弃牌抢杠或当前座位自杠，并继续摸补牌。
func (e *Engine) ApplyGang(_ context.Context, rs *RoundState, seat Seat, tileText string) ([]Notification, error) {
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
	gangFromSeat := SeatInvalid
	if claimGang {
		gangTile = rs.lastDiscard
		fromSeat := rs.lastDiscardSeat
		gangFromSeat = fromSeat
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
		rs.lastDiscardSeat = SeatInvalid
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
			rs.enterPhase(ReasonClaimWindow)
			return rs.claimPromptNotifications(gangTile)
		}
		rs.clearClaimWindow()
		for i := 0; i < 4; i++ {
			if err := rs.hands[seat].Remove(gangTile); err != nil {
				return nil, fmt.Errorf("consume self gang tiles: %w", err)
			}
		}
		rs.recordMeld(seat, "gang:"+gangTile.String())
		appendGangEntries(rs, seat, gangTile, rules.GangKindAn, SeatInvalid)
	}
	rs.turn = seat
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.enterPhase(ReasonNone)
	seatIndex := seat.Proto()
	detail := rs.actionDetail(seat, "gang", gangTile, seat, SeatInvalid)
	if claimGang {
		detail.SourceSeat = gangFromSeat.Proto()
	}
	rs.rememberLastAction(detail)
	progress := rs.drawTransitionProgress()
	action := &clientv1.ActionNotify{
		SeatIndex: seatIndex,
		Action:    "gang",
		Tile:      gangTile.String(),
		Detail:    detail,
	}
	progress.applyToAction(action)
	payload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("gang-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: action,
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

// ApplyPass 处理玩家主动放弃当前抢答或自摸选择。
func (e *Engine) ApplyPass(_ context.Context, rs *RoundState, seat Seat) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if seat < 0 || seat > 3 {
		return nil, fmt.Errorf("invalid seat")
	}
	switch {
	case rs.claimWindowOpen:
		if !rs.isTopClaimSeat(seat) {
			return nil, fmt.Errorf("pass not allowed")
		}
		metrics.ClaimWindowTotal.WithLabelValues("pass").Inc()
		rs.removeClaimCandidate(seat)
		if _, ok := rs.bestClaimCandidate(); ok {
			metrics.ClaimWindowTotal.WithLabelValues("relay").Inc()
			return rs.claimPromptNotifications(rs.lastDiscard)
		}
		rs.clearClaimWindow()
		rs.closeOpeningClaimWindow()
		return e.drawForCurrentTurn(rs)
	case seat == rs.turn && rs.waitingTsumo:
		if rs.pendingDraw == 0 {
			return nil, fmt.Errorf("missing pending draw")
		}
		metrics.ClaimWindowTotal.WithLabelValues("pass").Inc()
		rs.hands[seat].Add(rs.pendingDraw)
		rs.pendingDraw = 0
		rs.enterPhase(ReasonDiscard)
		return nil, nil
	default:
		return nil, fmt.Errorf("pass not allowed")
	}
}

func (rs *RoundState) canClaimPong(seat Seat) bool {
	return rs != nil && !rs.isHued(seat) && rs.isTopClaimSeat(seat) && rs.hasClaimAction(seat, "pong")
}

func (rs *RoundState) canClaimGang(seat Seat) bool {
	return rs != nil && !rs.isHued(seat) && rs.isTopClaimSeat(seat) && rs.hasClaimAction(seat, "gang")
}

func (rs *RoundState) canSelfGang(seat Seat, tileText string) bool {
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

// rewindInterruptedTurn 把 rs.turn 那家尚未完成的摸牌回滚到牌墙。
// 不再依赖 waitingDiscard / waitingTsumo 标志（这些已被 enterPhase 在
// 上一步 clearClaimWindow / openClaimWindow 时清空）；
// 改为以 pendingDraw 区分自摸窗口（仅清 pendingDraw）和正常摸牌（从手牌移除）。
func (rs *RoundState) rewindInterruptedTurn() error {
	if rs == nil || rs.currentDraw == 0 {
		return nil
	}
	if rs.pendingDraw != 0 {
		rs.pendingDraw = 0
	} else {
		if err := rs.hands[rs.turn].Remove(rs.currentDraw); err != nil {
			return fmt.Errorf("rewind current draw: %w", err)
		}
	}
	if err := rs.wall.PushFront(rs.currentDraw); err != nil {
		return fmt.Errorf("restore draw to wall: %w", err)
	}
	rs.currentDraw = 0
	return nil
}

func (rs *RoundState) claimSeat() Seat {
	candidate, ok := rs.bestClaimCandidate()
	if !ok {
		return SeatInvalid
	}
	return candidate.seat
}

func (rs *RoundState) openClaimWindow() {
	if rs == nil {
		return
	}
	rs.claimCandidates = rs.buildClaimCandidates()
	rs.enterPhase(ReasonClaimWindow)
}

func (rs *RoundState) clearClaimWindow() {
	if rs == nil {
		return
	}
	rs.claimCandidates = nil
	rs.qiangGangWindow = false
	rs.enterPhase(ReasonNone)
}

func (rs *RoundState) buildClaimCandidates() []claimCandidate {
	if rs == nil || rs.lastDiscard == 0 || rs.lastDiscardSeat < 0 {
		return nil
	}
	out := make([]claimCandidate, 0, 3)
	claimPolicy := rs.caps.Claims
	if claimPolicy == nil {
		claimPolicy = rules.CapabilitiesOf(rs.rule).Claims
	}
	for offset := 1; offset < 4; offset++ {
		seat := Seat((int(rs.lastDiscardSeat) + offset) % 4)
		claimActions := claimPolicy.Candidates(rules.ClaimContext{
			Seat:            seat,
			SourceSeat:      rs.lastDiscardSeat,
			Tile:            rs.lastDiscard,
			Hand:            rs.hands[seat],
			QiangGangWindow: rs.qiangGangWindow,
			Hued:            rs.isHued(seat),
			HuContext:       rs.claimHuContext(seat),
			CheckHu:         rs.rule.CheckHu,
		})
		actions, priority, choiceAction := normalizeClaimActions(claimActions)
		if len(actions) > 0 {
			out = append(out, claimCandidate{seat: seat, actions: actions, priority: priority, choiceAction: choiceAction})
		}
	}
	return out
}

func (rs *RoundState) bestClaimCandidate() (claimCandidate, bool) {
	if rs == nil || !rs.claimWindowOpen || len(rs.claimCandidates) == 0 {
		return claimCandidate{}, false
	}
	best := rs.claimCandidates[0]
	bestPriority := best.claimPriority()
	for _, candidate := range rs.claimCandidates[1:] {
		priority := candidate.claimPriority()
		if priority > bestPriority {
			best = candidate
			bestPriority = priority
		}
	}
	return best, true
}

func (rs *RoundState) isTopClaimSeat(seat Seat) bool {
	candidate, ok := rs.bestClaimCandidate()
	return ok && candidate.seat == seat
}

func (rs *RoundState) removeClaimCandidate(seat Seat) {
	if rs == nil || len(rs.claimCandidates) == 0 {
		return
	}
	out := rs.claimCandidates[:0]
	for _, candidate := range rs.claimCandidates {
		if candidate.seat != seat {
			out = append(out, candidate)
		}
	}
	rs.claimCandidates = out
}

func (rs *RoundState) hasClaimAction(seat Seat, action string) bool {
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

func (candidate claimCandidate) claimPriority() int {
	if candidate.priority > 0 {
		return candidate.priority
	}
	return legacyClaimPriority(candidate.actions)
}

func legacyClaimPriority(actions []string) int {
	if hasAction(actions, string(rules.ActionHu)) {
		return 3
	}
	if hasAction(actions, string(rules.ActionGang)) {
		return 2
	}
	if hasAction(actions, string(rules.ActionPong)) {
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
	candidate, ok := rs.bestClaimCandidate()
	if !ok {
		return nil, nil
	}
	claimAction := candidate.claimChoiceAction()
	claimSeatIndex := candidate.seat.Proto()
	progress := rs.roundProgress()
	action := &clientv1.ActionNotify{
		SeatIndex: claimSeatIndex,
		Action:    claimAction,
		Tile:      discard.String(),
	}
	progress.applyToAction(action)
	claimPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("claim-%d-%d", rs.step, candidate.seat),
		Body: &clientv1.Envelope_Action{
			Action: action,
		},
	})
	if err != nil {
		return nil, err
	}
	return []Notification{{Kind: KindAction, Payload: claimPayload, TargetSeat: BroadcastSeat}}, nil
}

func normalizeClaimActions(in []rules.ClaimAction) ([]string, int, string) {
	if len(in) == 0 {
		return nil, 0, ""
	}
	actions := make([]string, 0, len(in))
	priority := 0
	choiceAction := ""
	for _, action := range in {
		name := string(action.Name)
		if name == "" {
			continue
		}
		actions = append(actions, name)
		if action.Priority > priority {
			priority = action.Priority
			choiceAction = action.ChoiceAction
		}
	}
	return actions, priority, choiceAction
}

func (candidate claimCandidate) claimChoiceAction() string {
	if candidate.choiceAction != "" {
		return candidate.choiceAction
	}
	switch {
	case hasAction(candidate.actions, string(rules.ActionHu)):
		return "hu_choice"
	case hasAction(candidate.actions, string(rules.ActionGang)):
		return "gang_choice"
	case hasAction(candidate.actions, string(rules.ActionPong)):
		return "pong_choice"
	case hasAction(candidate.actions, string(rules.ActionChi)):
		return "chi_choice"
	default:
		return "claim_choice"
	}
}

func (rs *RoundState) rawCanClaimGang(seat Seat) bool {
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

func (rs *RoundState) claimHuContext(seat Seat) rules.HuContext {
	if rs == nil {
		return rules.HuContext{}
	}
	wallRemaining := 0
	if rs.wall != nil {
		wallRemaining = rs.wall.Remaining()
	}
	ctx := rules.HuContext{
		Source:          rules.HuSourceDiscard,
		PendingTile:     rs.lastDiscard,
		Discarder:       rs.lastDiscardSeat,
		ResponsibleSeat: rs.lastDiscardSeat,
		GangHistory:     append([]rules.GangRecord(nil), rs.gangRecords...),
		WallRemaining:   wallRemaining,
	}
	if rs.qiangGangWindow {
		ctx.Source = rules.HuSourceQiangGang
	}
	_ = seat
	return ctx
}
