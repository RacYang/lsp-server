package room

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"
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
	rs.recordMeldFact(seat, "pong", []tile.Tile{claimedTile, claimedTile, claimedTile}, claimedFromSeat)
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
	discard := chooseDiscard(rs.hands[seat], tile.Suit(rs.missingSuitBySeat()[seat]))
	next, err := e.ApplyDiscard(ctx, rs, seat, discard.String())
	if err != nil {
		return nil, err
	}
	return append(out, next...), nil
}

// ApplyChi 处理弃牌抢答窗口中的吃牌动作。正式四川血战规则不会产生 chi 候选；
// 该链路用于规则能力打开后的统一动作契约。
func (e *Engine) ApplyChi(_ context.Context, rs *RoundState, seat Seat, tileTexts []string) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if seat < 0 || seat > 3 {
		return nil, fmt.Errorf("invalid seat")
	}
	if !rs.canClaimChi(seat) {
		return nil, fmt.Errorf("chi not allowed")
	}
	claimedTile := rs.lastDiscard
	claimedFromSeat := rs.lastDiscardSeat
	chiTiles, err := rs.resolveChiTiles(seat, claimedTile, tileTexts)
	if err != nil {
		return nil, err
	}
	metrics.ClaimWindowTotal.WithLabelValues("chi").Inc()
	rs.clearClaimWindow()
	rs.closeOpeningClaimWindow()
	if err := rs.rewindInterruptedTurn(); err != nil {
		return nil, err
	}
	consume := append([]tile.Tile(nil), chiTiles...)
	removedClaimed := false
	for i := 0; i < len(consume); i++ {
		if consume[i] == claimedTile && !removedClaimed {
			removedClaimed = true
			consume = append(consume[:i], consume[i+1:]...)
			break
		}
	}
	if !removedClaimed {
		return nil, fmt.Errorf("chi tiles missing claimed tile")
	}
	for _, t := range consume {
		if err := rs.hands[seat].Remove(t); err != nil {
			return nil, fmt.Errorf("consume chi tile: %w", err)
		}
	}
	rs.removeLastDiscard(claimedFromSeat, claimedTile)
	rs.recordMeldFact(seat, "chi", chiTiles, claimedFromSeat)
	rs.turn = seat
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.lastDiscard = 0
	rs.lastDiscardSeat = SeatInvalid
	rs.enterPhase(ReasonDiscard)
	detail := rs.actionDetail(seat, "chi", claimedTile, seat, claimedFromSeat)
	rs.rememberLastAction(detail)
	progress := rs.roundProgress()
	action := &clientv1.ActionNotify{
		SeatIndex: seat.Proto(),
		Action:    "chi",
		Tile:      claimedTile.String(),
		Detail:    detail,
	}
	progress.applyToAction(action)
	payload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("chi-%d", rs.step),
		Body:  &clientv1.Envelope_Action{Action: action},
	})
	if err != nil {
		return nil, err
	}
	return []Notification{{Kind: KindAction, Payload: payload, TargetSeat: BroadcastSeat}}, nil
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
	gangTile, parseErr := tile.Parse(tileText)
	buGang := parseErr == nil && rs.canBuGang(seat, gangTile)
	anGang := parseErr == nil && rs.canAnGang(seat, gangTile)
	if !claimGang && !buGang && !anGang {
		return nil, fmt.Errorf("gang not allowed")
	}
	if claimGang {
		metrics.ClaimWindowTotal.WithLabelValues("gang").Inc()
	}
	gangFromSeat := SeatInvalid
	meldKind := "an_gang"
	switch {
	case claimGang:
		gangTile = rs.lastDiscard
		fromSeat := rs.lastDiscardSeat
		gangFromSeat = fromSeat
		meldKind = "zhi_gang"
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
		rs.recordMeldFact(seat, "zhi_gang", []tile.Tile{gangTile, gangTile, gangTile, gangTile}, fromSeat)
		rs.lastDiscard = 0
		rs.lastDiscardSeat = SeatInvalid
		rs.appendGangScoreEvents(seat, gangTile, rules.GangKindMing, fromSeat)
	case buGang:
		rs.lastDiscard = gangTile
		rs.lastDiscardSeat = seat
		rs.pendingGangSeat = seat
		rs.pendingGangTile = gangTile
		rs.qiangGangWindow = true
		rs.claimCandidates = rs.buildClaimCandidates()
		if len(rs.claimCandidates) > 0 {
			rs.enterPhase(ReasonClaimWindow)
			return rs.claimPromptNotifications(gangTile)
		}
		rs.clearClaimWindow()
		if err := rs.completeBuGang(seat, gangTile); err != nil {
			return nil, err
		}
		meldKind = "bu_gang"
	default:
		rs.clearClaimWindow()
		for i := 0; i < 4; i++ {
			if err := rs.hands[seat].Remove(gangTile); err != nil {
				return nil, fmt.Errorf("consume self gang tiles: %w", err)
			}
		}
		rs.recordMeldFact(seat, "an_gang", []tile.Tile{gangTile, gangTile, gangTile, gangTile}, SeatInvalid)
		rs.appendGangScoreEvents(seat, gangTile, rules.GangKindAn, SeatInvalid)
	}
	return e.finishGangAction(rs, seat, gangTile, meldKind, gangFromSeat)
}

func (e *Engine) finishGangAction(rs *RoundState, seat Seat, gangTile tile.Tile, meldKind string, gangFromSeat Seat) ([]Notification, error) {
	rs.turn = seat
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.lastDiscard = 0
	rs.lastDiscardSeat = SeatInvalid
	rs.qiangGangWindow = false
	rs.pendingGangSeat = SeatInvalid
	rs.pendingGangTile = 0
	rs.enterPhase(ReasonNone)
	seatIndex := seat.Proto()
	detail := rs.actionDetail(seat, meldKind, gangTile, seat, SeatInvalid)
	if gangFromSeat.Valid() {
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
	notification := Notification{Kind: KindAction, Payload: payload, TargetSeat: BroadcastSeat}
	if meldKind == "an_gang" {
		notification.Privacy = PrivacyPerSeat
		notification.Project = func(target Seat) []byte {
			projected := proto.Clone(action).(*clientv1.ActionNotify)
			if target != seat {
				projected.Tile = ""
				if projected.Detail != nil {
					projected.Detail.Tile = ""
				}
			}
			payload, err := marshalEnvelope(&clientv1.Envelope{
				ReqId: fmt.Sprintf("gang-%d", rs.step),
				Body:  &clientv1.Envelope_Action{Action: projected},
			})
			if err != nil {
				return nil
			}
			return payload
		}
	}
	out := []Notification{notification}
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
		if rs.qiangGangWindow && rs.pendingGangSeat.Valid() && rs.pendingGangTile != 0 {
			seat := rs.pendingGangSeat
			gangTile := rs.pendingGangTile
			rs.clearClaimWindow()
			if err := rs.completeBuGang(seat, gangTile); err != nil {
				return nil, err
			}
			rs.turn = seat
			return e.finishGangAction(rs, seat, gangTile, "bu_gang", SeatInvalid)
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
	return rs != nil && !rs.isSeatOutAfterHu(seat) && rs.isTopClaimSeat(seat) && rs.hasClaimAction(seat, "pong")
}

func (rs *RoundState) canClaimGang(seat Seat) bool {
	return rs != nil && !rs.isSeatOutAfterHu(seat) && rs.isTopClaimSeat(seat) && rs.hasClaimAction(seat, "gang")
}

func (rs *RoundState) canClaimChi(seat Seat) bool {
	return rs != nil && !rs.isSeatOutAfterHu(seat) && rs.isTopClaimSeat(seat) && rs.hasClaimAction(seat, "chi")
}

func (rs *RoundState) canSelfGang(seat Seat, tileText string) bool {
	target, err := tile.Parse(tileText)
	if err != nil {
		return false
	}
	return rs.canAnGang(seat, target) || rs.canBuGang(seat, target)
}

func (rs *RoundState) canAnGang(seat Seat, target tile.Tile) bool {
	if rs == nil || seat != rs.turn || !rs.waitingDiscard || rs.isSeatOutAfterHu(seat) {
		return false
	}
	rs.ensureRuleRuntime()
	policy := rs.caps.SelfActions
	return policy.CanAnGang(rules.SelfActionContext{
		Seat:      seat,
		Tile:      target,
		Hand:      rs.hands[seat],
		RuleState: rs.ruleState,
		Melds:     rs.meldContexts(seat),
	})
}

func (rs *RoundState) canBuGang(seat Seat, target tile.Tile) bool {
	if rs == nil || seat != rs.turn || !rs.waitingDiscard || rs.isSeatOutAfterHu(seat) {
		return false
	}
	if target == 0 || !rs.hasPongMeld(seat, target) {
		return false
	}
	rs.ensureRuleRuntime()
	policy := rs.caps.SelfActions
	return policy.CanBuGang(rules.SelfActionContext{
		Seat:      seat,
		Tile:      target,
		Hand:      rs.hands[seat],
		RuleState: rs.ruleState,
		Melds:     rs.meldContexts(seat),
	})
}

func (rs *RoundState) completeBuGang(seat Seat, gangTile tile.Tile) error {
	if rs == nil {
		return fmt.Errorf("nil round state")
	}
	if err := rs.hands[seat].Remove(gangTile); err != nil {
		return fmt.Errorf("consume bu gang tile: %w", err)
	}
	if !rs.upgradePongToBuGang(seat, gangTile) {
		return fmt.Errorf("missing pong meld for bu gang")
	}
	rs.appendGangScoreEvents(seat, gangTile, rules.GangKindBu, SeatInvalid)
	rs.pendingGangSeat = SeatInvalid
	rs.pendingGangTile = 0
	return nil
}

func (rs *RoundState) resolveChiTiles(seat Seat, claimed tile.Tile, tileTexts []string) ([]tile.Tile, error) {
	if rs == nil || claimed == 0 {
		return nil, fmt.Errorf("missing chi tile")
	}
	if len(tileTexts) > 0 {
		if len(tileTexts) != 3 {
			return nil, fmt.Errorf("chi requires 3 tiles")
		}
		tiles := make([]tile.Tile, 0, 3)
		hasClaimed := false
		for _, raw := range tileTexts {
			t, err := tile.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("parse chi tile: %w", err)
			}
			if t == claimed {
				hasClaimed = true
			}
			tiles = append(tiles, t)
		}
		if !hasClaimed {
			return nil, fmt.Errorf("chi tiles missing claimed tile")
		}
		if !isChiSequence(tiles) {
			return nil, fmt.Errorf("invalid chi sequence")
		}
		if !rs.handContainsChiRemainder(seat, claimed, tiles) {
			return nil, fmt.Errorf("chi tiles not in hand")
		}
		return tiles, nil
	}
	for start := claimed.Rank() - 2; start <= claimed.Rank(); start++ {
		if start < 1 || start+2 > 9 {
			continue
		}
		a, _ := tile.New(claimed.Suit(), start)
		b, _ := tile.New(claimed.Suit(), start+1)
		c, _ := tile.New(claimed.Suit(), start+2)
		tiles := []tile.Tile{a, b, c}
		if !rs.handContainsChiRemainder(seat, claimed, tiles) {
			continue
		}
		return tiles, nil
	}
	return nil, fmt.Errorf("no valid chi sequence")
}

func (rs *RoundState) handContainsChiRemainder(seat Seat, claimed tile.Tile, tiles []tile.Tile) bool {
	need := map[tile.Tile]int{}
	removedClaimed := false
	for _, t := range tiles {
		if t == claimed && !removedClaimed {
			removedClaimed = true
			continue
		}
		need[t]++
	}
	if !removedClaimed {
		return false
	}
	for _, t := range rs.hands[seat].Tiles() {
		if need[t] > 0 {
			need[t]--
		}
	}
	for _, n := range need {
		if n > 0 {
			return false
		}
	}
	return true
}

func isChiSequence(tiles []tile.Tile) bool {
	if len(tiles) != 3 {
		return false
	}
	suit := tiles[0].Suit()
	ranks := []int{tiles[0].Rank(), tiles[1].Rank(), tiles[2].Rank()}
	for _, t := range tiles[1:] {
		if t.Suit() != suit {
			return false
		}
	}
	for i := 0; i < len(ranks)-1; i++ {
		for j := i + 1; j < len(ranks); j++ {
			if ranks[j] < ranks[i] {
				ranks[i], ranks[j] = ranks[j], ranks[i]
			}
		}
	}
	return ranks[0]+1 == ranks[1] && ranks[1]+1 == ranks[2]
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
	rs.pendingGangSeat = SeatInvalid
	rs.pendingGangTile = 0
	rs.enterPhase(ReasonNone)
}

func (rs *RoundState) buildClaimCandidates() []claimCandidate {
	if rs == nil || rs.lastDiscard == 0 || rs.lastDiscardSeat < 0 {
		return nil
	}
	rs.ensureRuleRuntime()
	out := make([]claimCandidate, 0, 3)
	claimPolicy := rs.caps.Claims
	winPolicy := rs.caps.Win
	for offset := 1; offset < 4; offset++ {
		seat := Seat((int(rs.lastDiscardSeat) + offset) % 4)
		if rs.isSurrendered(seat) {
			continue
		}
		claimActions := claimPolicy.Candidates(rules.ClaimContext{
			Seat:            seat,
			SourceSeat:      rs.lastDiscardSeat,
			Tile:            rs.lastDiscard,
			Hand:            rs.hands[seat],
			QiangGangWindow: rs.qiangGangWindow,
			Hued:            rs.isSeatOutAfterHu(seat),
			HuContext:       rs.claimHuContext(seat),
			CheckHu:         winPolicy.CheckHu,
			RuleState:       rs.ruleState,
			Melds:           rs.meldContexts(seat),
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
	if !ok {
		return false
	}
	targetPriority := candidate.claimPriority()
	for _, current := range rs.claimCandidates {
		if current.seat == seat && current.claimPriority() == targetPriority {
			return true
		}
	}
	return false
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

func (rs *RoundState) claimPriorityForSeat(seat Seat) int {
	if rs == nil || !rs.claimWindowOpen {
		return 0
	}
	for _, candidate := range rs.claimCandidates {
		if candidate.seat == seat {
			return candidate.claimPriority()
		}
	}
	return 0
}

func (rs *RoundState) hasRemainingHuClaimAtPriority(priority int) bool {
	if rs == nil || priority <= 0 {
		return false
	}
	for _, candidate := range rs.claimCandidates {
		if candidate.claimPriority() == priority && hasAction(candidate.actions, string(rules.ActionHu)) {
			return true
		}
	}
	return false
}

func (candidate claimCandidate) claimPriority() int {
	if candidate.priority > 0 {
		return candidate.priority
	}
	return claimPriority(candidate.actions)
}

func claimPriority(actions []string) int {
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
	if rs == nil || rs.lastDiscard == 0 || rs.lastDiscardSeat < 0 || seat == rs.lastDiscardSeat || rs.isSeatOutAfterHu(seat) {
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
	source := rules.HuSourceDiscard
	if rs.qiangGangWindow {
		source = rules.HuSourceQiangGang
	}
	return rs.huContextForSeat(seat, source, rs.lastDiscard)
}
