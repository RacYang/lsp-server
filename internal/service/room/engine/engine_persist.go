package engine

import (
	"encoding/json"
	"fmt"
	"sort"

	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

const roundPersistSchemaVersion = 7

// ProjectRoundState 返回当前局面面向协议、快照和 bot 的统一事实投影。
func ProjectRoundState(rs *RoundState) RoundProjection {
	if rs == nil {
		return RoundProjection{}
	}
	actingSeat, waitingAction, pendingTile, available := rs.snapshotWaiting()
	serverNow := int64(0)
	if rs.clk != nil {
		serverNow = rs.clk.Now().UnixMilli()
	}
	return RoundProjection{
		Progress: RoundProgress{
			ActingSeat:       actingSeat,
			ActingSeats:      rs.actingSeats(),
			WaitingAction:    waitingAction,
			Phase:            rs.phase(),
			Step:             int64(rs.step),
			PendingTile:      pendingTile,
			AvailableActions: append([]string(nil), available...),
			ClaimCandidates:  rs.roundClaimCandidates(),
			WallRemaining:    rs.wallRemaining(),
			DeadlineUnixMs:   rs.deadlineUnixMs,
			Reason:           rs.phaseReason,
			ServerNowUnixMs:  serverNow,
		},
		Facts: RoundFacts{
			HandsBySeat:     rs.handStringsBySeat(),
			DiscardsBySeat:  rs.discardStringsBySeat(),
			MeldsBySeat:     cloneStringMatrix(rs.melds),
			MeldInfosBySeat: rs.meldInfosBySeat(),
			PlayerIDs:       rs.playerIDs,
			QueBySeat:       append([]int32(nil), rs.missingSuitBySeat()...),
			HuedSeats:       rs.huedSeats(),
			Closed:          rs.closed,
			LastAction:      rs.lastAction,
			RoundIndex:      0,
			HandIndex:       0,
			TotalScores:     rs.totalScores(),
			RuleMeta:        rs.ruleMeta(),
		},
	}
}

// SnapshotView 返回当前局面的最小等待态摘要。
func (rs *RoundState) SnapshotView() RoundView {
	if rs == nil {
		return RoundView{}
	}
	projection := ProjectRoundState(rs)
	progress := projection.Progress
	facts := projection.Facts
	return RoundView{
		ActingSeat:       progress.ActingSeat,
		ActingSeats:      append([]int32(nil), progress.ActingSeats...),
		WaitingAction:    progress.WaitingAction,
		Phase:            progress.Phase,
		LastStep:         progress.Step,
		PendingTile:      progress.PendingTile,
		AvailableActions: append([]string(nil), progress.AvailableActions...),
		ClaimCandidates:  append([]RoundClaimCandidate(nil), progress.ClaimCandidates...),
		HandsBySeat:      facts.HandsBySeat,
		DiscardsBySeat:   facts.DiscardsBySeat,
		MeldsBySeat:      facts.MeldsBySeat,
		MeldInfosBySeat:  facts.MeldInfosBySeat,
		PlayerIDs:        facts.PlayerIDs,
		QueBySeat:        facts.QueBySeat,
		HuedSeats:        facts.HuedSeats,
		OpeningSubmitted: rs.openingSubmittedByAction(),
		Closed:           facts.Closed,
		LastAction:       facts.LastAction,
		WallRemaining:    progress.WallRemaining,
		DeadlineUnixMs:   progress.DeadlineUnixMs,
		RoundIndex:       facts.RoundIndex,
		HandIndex:        facts.HandIndex,
		TotalScores:      facts.TotalScores,
		RuleMeta:         facts.RuleMeta,
	}
}

func (rs *RoundState) recordDiscard(seat Seat, t tile.Tile) {
	if rs == nil || seat < 0 || seat > 3 {
		return
	}
	if len(rs.discards) < 4 {
		rs.discards = make([][]tile.Tile, 4)
	}
	rs.discards[seat] = append(rs.discards[seat], t)
}

func (rs *RoundState) removeLastDiscard(seat Seat, t tile.Tile) {
	if rs == nil || seat < 0 || seat > 3 || int(seat) >= len(rs.discards) {
		return
	}
	ds := rs.discards[seat]
	if len(ds) == 0 || ds[len(ds)-1] != t {
		return
	}
	rs.discards[seat] = ds[:len(ds)-1]
}

func (rs *RoundState) handStringsBySeat() [][]string {
	out := make([][]string, 4)
	if rs == nil {
		return out
	}
	for seat := 0; seat < 4; seat++ {
		if seat >= len(rs.hands) || rs.hands[seat] == nil {
			continue
		}
		ts := rs.hands[seat].Tiles()
		sort.Slice(ts, func(i, j int) bool { return ts[i].Index() < ts[j].Index() })
		out[seat] = tilesToStrings(ts)
	}
	return out
}

func (rs *RoundState) discardStringsBySeat() [][]string {
	out := make([][]string, 4)
	if rs == nil {
		return out
	}
	for seat := 0; seat < 4 && seat < len(rs.discards); seat++ {
		out[seat] = tilesToStrings(rs.discards[seat])
	}
	return out
}

func cloneStringMatrix(in [][]string) [][]string {
	out := make([][]string, 4)
	for i := 0; i < len(out) && i < len(in); i++ {
		out[i] = append([]string(nil), in[i]...)
	}
	return out
}

func cloneTileMatrix(in [][]tile.Tile) [][]tile.Tile {
	out := make([][]tile.Tile, 4)
	for i := 0; i < len(out) && i < len(in); i++ {
		out[i] = append([]tile.Tile(nil), in[i]...)
	}
	return out
}

func (rs *RoundState) roundClaimCandidates() []RoundClaimCandidate {
	if rs == nil || !rs.claimWindowOpen {
		return nil
	}
	out := make([]RoundClaimCandidate, 0, len(rs.claimCandidates))
	for _, candidate := range rs.claimCandidates {
		out = append(out, RoundClaimCandidate{
			Seat:    candidate.seat.Proto(),
			Actions: append([]string(nil), candidate.actions...),
		})
	}
	return out
}

func (rs *RoundState) snapshotWaiting() (int32, string, string, []string) {
	if rs == nil {
		return -1, "", "", nil
	}
	if rs.waitingOpening {
		step, ok := rs.currentOpeningStep()
		if !ok {
			return -1, "opening", "", nil
		}
		for seat, done := range rs.openingSubmitted(step.Action) {
			if !done && !rs.isSurrendered(Seat(seat)) {
				return Seat(seat).Proto(), step.Action, "", []string{step.Action}
			}
		}
	}
	if seat := rs.claimSeat(); seat >= 0 {
		actions := make([]string, 0, 3)
		if rs.hasClaimAction(seat, "hu") {
			actions = append(actions, "hu")
		}
		if rs.hasClaimAction(seat, "gang") {
			actions = append(actions, "gang")
		}
		if rs.hasClaimAction(seat, "pong") {
			actions = append(actions, "pong")
		}
		if len(actions) > 0 {
			return seat.Proto(), "claim_window", rs.lastDiscard.String(), actions
		}
	}
	if rs.waitingTsumo {
		if rs.isSurrendered(rs.turn) {
			return -1, "tsumo_window", rs.pendingDraw.String(), nil
		}
		return rs.turn.Proto(), "tsumo_window", rs.pendingDraw.String(), []string{"hu", "pass"}
	}
	if rs.waitingDiscard {
		if rs.isSurrendered(rs.turn) {
			return -1, "discard", "", nil
		}
		actions := []string{"discard"}
		for _, t := range rs.hands[rs.turn].Tiles() {
			if rs.canSelfGang(rs.turn, t.String()) {
				actions = append(actions, "gang")
				break
			}
		}
		return rs.turn.Proto(), "discard", "", actions
	}
	return -1, "", "", nil
}

// MarshalRoundPersistJSON 将当前局内状态序列化为 JSON，供 Redis snapmeta 保存。
func (rs *RoundState) MarshalRoundPersistJSON() ([]byte, error) {
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, nil
	}
	rp := roundPersist{
		SchemaVersion:          roundPersistSchemaVersion,
		RuleID:                 rs.ruleID,
		PlayerIDs:              rs.playerIDs,
		WaitingOpening:         rs.waitingOpening,
		Turn:                   int(rs.turn),
		Step:                   rs.step,
		DealerSeat:             int(rs.dealerSeat),
		OpeningDrawSeat:        int(rs.openingDrawSeat),
		DealerFirstDiscardOpen: rs.dealerFirstDiscardOpen,
		WaitingDiscard:         rs.waitingDiscard,
		WaitingTsumo:           rs.waitingTsumo,
		ClaimWindowOpen:        rs.claimWindowOpen,
		QiangGangWindow:        rs.qiangGangWindow,
		PendingGangSeat:        int(rs.pendingGangSeat),
		RuleState:              rs.ruleState,
		WinEvents:              append([]rules.WinEvent(nil), rs.winEvents...),
		ScoreEvents:            append([]rules.ScoreEvent(nil), rs.scoreEvents...),
		SurrenderedSeats:       append([]bool(nil), rs.surrendered...),
		GangRecords:            append([]rules.GangRecord(nil), rs.gangRecords...),
		LastGangFollowUp:       rs.lastGangFollowUp,
		LastDiscardAfterGang:   rs.lastDiscardAfterGang,
		Hands:                  make([][]string, 4),
		Discards:               make([][]string, 4),
		Flowers:                make([][]string, 4),
		Melds:                  make([][]string, 4),
		PhaseReason:            int(rs.phaseReason),
		PhaseStartUnixMs:       rs.phaseStartUnixMs,
	}
	if rs.claimWindowOpen {
		rp.ClaimCandidates = make([]claimCandidatePersist, 0, len(rs.claimCandidates))
		for _, candidate := range rs.claimCandidates {
			rp.ClaimCandidates = append(rp.ClaimCandidates, claimCandidatePersist{
				Seat:    int(candidate.seat),
				Actions: append([]string(nil), candidate.actions...),
			})
		}
	}
	if rs.pendingDraw != 0 {
		rp.PendingDraw = rs.pendingDraw.String()
	}
	if rs.currentDraw != 0 {
		rp.CurrentDraw = rs.currentDraw.String()
	}
	if rs.pendingGangTile != 0 {
		rp.PendingGangTile = rs.pendingGangTile.String()
	}
	if rs.lastDiscard != 0 {
		rp.LastDiscard = rs.lastDiscard.String()
		rp.LastDiscardSeat = int(rs.lastDiscardSeat)
	}
	for seat := 0; seat < 4; seat++ {
		var ts []tile.Tile
		if seat < len(rs.hands) && rs.hands[seat] != nil {
			ts = append([]tile.Tile(nil), rs.hands[seat].Tiles()...)
		}
		sort.Slice(ts, func(i, j int) bool { return ts[i].Index() < ts[j].Index() })
		rp.Hands[seat] = tilesToStrings(ts)
		if seat < len(rs.discards) {
			rp.Discards[seat] = tilesToStrings(rs.discards[seat])
		}
		if seat < len(rs.flowers) {
			rp.Flowers[seat] = tilesToStrings(rs.flowers[seat])
		}
		if seat < len(rs.melds) {
			rp.Melds[seat] = append([]string(nil), rs.melds[seat]...)
		}
	}
	if rs.wall != nil {
		rp.WallRemaining = tilesToStrings(rs.wall.Tiles())
	}
	rp.WallSeed = rs.wallSeed
	return json.Marshal(rp)
}

// RestoreRoundFromPersistJSON 从当前 schema JSON 恢复进行中牌局的最小运行态。
func RestoreRoundFromPersistJSON(roomID string, data []byte) (*RoundState, error) {
	if roomID == "" {
		return nil, fmt.Errorf("empty room_id")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty round json")
	}
	var rp roundPersist
	if err := json.Unmarshal(data, &rp); err != nil {
		return nil, fmt.Errorf("unmarshal round json: %w", err)
	}
	if rp.SchemaVersion != roundPersistSchemaVersion {
		return nil, fmt.Errorf("%w: %d", ErrRoundPersistUnsupportedSchema, rp.SchemaVersion)
	}

	rs, err := buildRoundStateFromPersist(roomID, &rp)
	if err != nil {
		return nil, err
	}
	if err := decodeTileFieldsIntoRound(rs, &rp); err != nil {
		return nil, err
	}
	if err := decodeDiscardsIntoRound(rs, &rp); err != nil {
		return nil, err
	}
	if err := decodeFlowersIntoRound(rs, &rp); err != nil {
		return nil, err
	}
	if err := decodeClaimCandidatesIntoRound(rs, &rp); err != nil {
		return nil, err
	}
	finalizeRoundInvariants(rs)
	return rs, nil
}

// buildRoundStateFromPersist 把已升级到当前 schema 的持久化结构映射为最小 RoundState 骨架。
func buildRoundStateFromPersist(roomID string, rp *roundPersist) (*RoundState, error) {
	ruleID := rp.RuleID
	if ruleID == "" {
		ruleID = "sichuan_xuezhandaodi_huansanzhang"
	}
	rule := rules.MustGet(ruleID)
	caps := rules.CapabilitiesOf(rule)

	wallTiles := make([]tile.Tile, 0, len(rp.WallRemaining))
	for _, raw := range rp.WallRemaining {
		t, err := tile.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse wall tile %q: %w", raw, err)
		}
		wallTiles = append(wallTiles, t)
	}

	hands := make([]*hand.Hand, 4)
	for seat := 0; seat < 4; seat++ {
		hands[seat] = hand.New()
		if seat >= len(rp.Hands) {
			continue
		}
		for _, raw := range rp.Hands[seat] {
			t, err := tile.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("parse hand tile %q: %w", raw, err)
			}
			hands[seat].Add(t)
		}
	}

	rs := &RoundState{
		roomID:                 roomID,
		ruleID:                 ruleID,
		playerIDs:              rp.PlayerIDs,
		rule:                   rule,
		caps:                   caps,
		wallSeed:               rp.WallSeed,
		wall:                   wall.NewFromOrderedTiles(wallTiles),
		hands:                  hands,
		discards:               make([][]tile.Tile, 4),
		flowers:                make([][]tile.Tile, 4),
		melds:                  cloneStringMatrix(rp.Melds),
		ruleState:              restoreRuleState(caps, rp),
		waitingOpening:         rp.WaitingOpening,
		waitingDiscard:         rp.WaitingDiscard,
		waitingTsumo:           rp.WaitingTsumo,
		claimWindowOpen:        rp.ClaimWindowOpen,
		qiangGangWindow:        rp.QiangGangWindow,
		pendingGangSeat:        SeatFromInt(rp.PendingGangSeat),
		turn:                   SeatFromInt(rp.Turn),
		step:                   rp.Step,
		dealerSeat:             SeatFromInt(rp.DealerSeat),
		openingDrawSeat:        SeatFromInt(rp.OpeningDrawSeat),
		dealerFirstDiscardOpen: rp.DealerFirstDiscardOpen,
		surrendered:            append([]bool(nil), rp.SurrenderedSeats...),
		winEvents:              restoreWinEvents(rp),
		scoreEvents:            restoreScoreEvents(rp),
		gangRecords:            append([]rules.GangRecord(nil), rp.GangRecords...),
		lastGangFollowUp:       rp.LastGangFollowUp,
		lastDiscardAfterGang:   rp.LastDiscardAfterGang,
		phaseReason:            WaitingReason(rp.PhaseReason),
		phaseStartUnixMs:       rp.PhaseStartUnixMs,
	}
	return rs, nil
}

// decodeTileFieldsIntoRound 把 pendingDraw / currentDraw / lastDiscard 等字符串字段还原为牌。
func decodeTileFieldsIntoRound(rs *RoundState, rp *roundPersist) error {
	if rp.PendingDraw != "" {
		t, err := tile.Parse(rp.PendingDraw)
		if err != nil {
			return fmt.Errorf("parse pending draw: %w", err)
		}
		rs.pendingDraw = t
	}
	if rp.CurrentDraw != "" {
		t, err := tile.Parse(rp.CurrentDraw)
		if err != nil {
			return fmt.Errorf("parse current draw: %w", err)
		}
		rs.currentDraw = t
	}
	if rp.PendingGangTile != "" {
		t, err := tile.Parse(rp.PendingGangTile)
		if err != nil {
			return fmt.Errorf("parse pending gang tile: %w", err)
		}
		rs.pendingGangTile = t
	}
	if rp.LastDiscard != "" {
		t, err := tile.Parse(rp.LastDiscard)
		if err != nil {
			return fmt.Errorf("parse last discard: %w", err)
		}
		rs.lastDiscard = t
		rs.lastDiscardSeat = SeatFromInt(rp.LastDiscardSeat)
	} else {
		rs.lastDiscardSeat = SeatInvalid
	}
	return nil
}

func decodeDiscardsIntoRound(rs *RoundState, rp *roundPersist) error {
	if rs == nil {
		return nil
	}
	if len(rs.discards) < 4 {
		rs.discards = make([][]tile.Tile, 4)
	}
	for seat := 0; seat < len(rp.Discards) && seat < 4; seat++ {
		for _, raw := range rp.Discards[seat] {
			t, err := tile.Parse(raw)
			if err != nil {
				return fmt.Errorf("parse discard tile %q: %w", raw, err)
			}
			rs.discards[seat] = append(rs.discards[seat], t)
		}
	}
	return nil
}

func decodeFlowersIntoRound(rs *RoundState, rp *roundPersist) error {
	if rs == nil || rp == nil {
		return nil
	}
	for seat := 0; seat < len(rp.Flowers) && seat < 4; seat++ {
		for _, raw := range rp.Flowers[seat] {
			t, err := tile.Parse(raw)
			if err != nil {
				return fmt.Errorf("parse flower tile %q: %w", raw, err)
			}
			rs.recordFlower(Seat(seat), t)
		}
	}
	return nil
}

// decodeClaimCandidatesIntoRound 重建抢答候选列表，并校验座位与动作合法性。
func decodeClaimCandidatesIntoRound(rs *RoundState, rp *roundPersist) error {
	for _, candidate := range rp.ClaimCandidates {
		if candidate.Seat < 0 || candidate.Seat > 3 {
			return fmt.Errorf("invalid claim candidate seat: %d", candidate.Seat)
		}
		actions := make([]string, 0, len(candidate.Actions))
		for _, action := range candidate.Actions {
			switch action {
			case "hu", "gang", "pong":
				actions = append(actions, action)
			default:
				return fmt.Errorf("invalid claim candidate action: %s", action)
			}
		}
		if len(actions) > 0 {
			rs.claimCandidates = append(rs.claimCandidates, claimCandidate{
				seat:         Seat(candidate.Seat),
				actions:      actions,
				priority:     claimPriority(actions),
				choiceAction: claimCandidate{actions: actions}.claimChoiceAction(),
			})
		}
	}
	if rs.claimWindowOpen && len(rs.claimCandidates) == 0 {
		rs.claimCandidates = rs.buildClaimCandidates()
	}
	return nil
}

func restoreRuleState(caps rules.CapabilitySet, rp *roundPersist) rules.RuleState {
	state := rp.RuleState
	return caps.State.NormalizeRuleState(state)
}

func restoreScoreEvents(rp *roundPersist) []rules.ScoreEvent {
	return append([]rules.ScoreEvent(nil), rp.ScoreEvents...)
}

func restoreWinEvents(rp *roundPersist) []rules.WinEvent {
	return append([]rules.WinEvent(nil), rp.WinEvents...)
}

// finalizeRoundInvariants 把切片字段补齐到固定长度，并恢复运行时派生状态。
func finalizeRoundInvariants(rs *RoundState) {
	rs.normalizeRuleState()
	for len(rs.surrendered) < 4 {
		rs.surrendered = append(rs.surrendered, false)
	}
	for len(rs.discards) < 4 {
		rs.discards = append(rs.discards, nil)
	}
	for len(rs.melds) < 4 {
		rs.melds = append(rs.melds, nil)
	}
	if !rs.pendingGangSeat.Valid() && rs.qiangGangWindow {
		rs.pendingGangSeat = rs.lastDiscardSeat
	}
	// 当前 schema 持久化 PhaseReason；这里保留不变量兜底，保证 enterPhase 与 Deadline()
	// 在恢复后保持自洽（详见 ADR-0045）。
	if rs.phaseReason == ReasonNone {
		switch {
		case rs.waitingOpening:
			if _, ok := rs.currentOpeningStep(); ok {
				rs.phaseReason = ReasonOpening
			}
		case rs.claimWindowOpen:
			rs.phaseReason = ReasonClaimWindow
		case rs.waitingTsumo:
			rs.phaseReason = ReasonTsumo
		case rs.waitingDiscard:
			rs.phaseReason = ReasonDiscard
		}
	}
}

// RoundViewFromPersistJSON 从当前 schema 持久化 JSON 投影等待态摘要。
func RoundViewFromPersistJSON(roomID string, data []byte) (RoundView, error) {
	rs, err := RestoreRoundFromPersistJSON(roomID, data)
	if err != nil {
		return RoundView{}, err
	}
	return rs.SnapshotView(), nil
}
