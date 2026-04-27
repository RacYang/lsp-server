package room

import (
	"encoding/json"
	"fmt"
	"sort"

	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/sichuanxzdd"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

const roundPersistSchemaVersion = 3

// SnapshotView 返回当前局面的最小等待态摘要。
func (rs *RoundState) SnapshotView() RoundView {
	actingSeat, waitingAction, pendingTile, available := rs.snapshotWaiting()
	return RoundView{
		ActingSeat:       actingSeat,
		WaitingAction:    waitingAction,
		PendingTile:      pendingTile,
		AvailableActions: append([]string(nil), available...),
		ClaimCandidates:  rs.roundClaimCandidates(),
	}
}

func (rs *RoundState) roundClaimCandidates() []RoundClaimCandidate {
	if rs == nil || !rs.claimWindowOpen {
		return nil
	}
	out := make([]RoundClaimCandidate, 0, len(rs.claimCandidates))
	for _, candidate := range rs.claimCandidates {
		out = append(out, RoundClaimCandidate{
			Seat:    int32(candidate.seat), //nolint:gosec // 座位范围固定
			Actions: append([]string(nil), candidate.actions...),
		})
	}
	return out
}

func (rs *RoundState) snapshotWaiting() (int32, string, string, []string) {
	if rs == nil {
		return -1, "", "", nil
	}
	if rs.waitingExchange {
		for seat, done := range rs.exchangeSubmitted {
			if !done {
				return int32(seat), "exchange_three", "", []string{"exchange_three"} //nolint:gosec // 座位范围固定
			}
		}
	}
	if rs.waitingQueMen {
		for seat, done := range rs.queSubmitted {
			if !done {
				return int32(seat), "que_men", "", []string{"que_men"} //nolint:gosec // 座位范围固定
			}
		}
	}
	if seat := rs.claimSeat(); seat >= 0 {
		actions := make([]string, 0, 2)
		if rs.hasClaimAction(seat, "gang") {
			actions = append(actions, "gang")
		}
		if rs.hasClaimAction(seat, "pong") {
			actions = append(actions, "pong")
		}
		if len(actions) > 0 {
			return int32(seat), "claim", rs.lastDiscard.String(), actions //nolint:gosec // 座位范围固定
		}
	}
	if rs.waitingTsumo {
		return int32(rs.turn), "tsumo_choice", rs.pendingDraw.String(), []string{"hu", "discard"} //nolint:gosec // 座位范围固定
	}
	if rs.waitingDiscard {
		actions := []string{"discard"}
		for _, t := range rs.hands[rs.turn].Tiles() {
			if rs.canSelfGang(rs.turn, t.String()) {
				actions = append(actions, "gang")
				break
			}
		}
		return int32(rs.turn), "discard", "", actions //nolint:gosec // 座位范围固定
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
		QueBySeat:              append([]int32(nil), rs.queBySeat...),
		WaitingExchange:        rs.waitingExchange,
		ExchangeDir:            rs.exchangeDirection,
		WaitingQueMen:          rs.waitingQueMen,
		ExchangeDone:           append([]bool(nil), rs.exchangeSubmitted...),
		QueDone:                append([]bool(nil), rs.queSubmitted...),
		Turn:                   rs.turn,
		Step:                   rs.step,
		DealerSeat:             rs.dealerSeat,
		OpeningDrawSeat:        rs.openingDrawSeat,
		DealerFirstDiscardOpen: rs.dealerFirstDiscardOpen,
		WaitingDiscard:         rs.waitingDiscard,
		WaitingTsumo:           rs.waitingTsumo,
		ClaimWindowOpen:        rs.claimWindowOpen,
		QiangGangWindow:        rs.qiangGangWindow,
		WinnerSeats:            append([]int(nil), rs.winnerSeats...),
		HuedSeats:              append([]bool(nil), rs.huedSeats...),
		Ledger:                 append([]sichuanxzdd.ScoreEntry(nil), rs.ledger...),
		GangRecords:            append([]rules.GangRecord(nil), rs.gangRecords...),
		LastGangFollowUp:       rs.lastGangFollowUp,
		LastDiscardAfterGang:   rs.lastDiscardAfterGang,
		Hands:                  make([][]string, 4),
		ExchangeTiles:          make([][]string, 4),
	}
	if rs.claimWindowOpen {
		rp.ClaimCandidates = make([]claimCandidatePersist, 0, len(rs.claimCandidates))
		for _, candidate := range rs.claimCandidates {
			rp.ClaimCandidates = append(rp.ClaimCandidates, claimCandidatePersist{
				Seat:    candidate.seat,
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
	if rs.lastDiscard != 0 {
		rp.LastDiscard = rs.lastDiscard.String()
		rp.LastDiscardSeat = rs.lastDiscardSeat
	}
	for seat := 0; seat < 4; seat++ {
		var ts []tile.Tile
		if seat < len(rs.hands) && rs.hands[seat] != nil {
			ts = append([]tile.Tile(nil), rs.hands[seat].Tiles()...)
		}
		sort.Slice(ts, func(i, j int) bool { return ts[i].Index() < ts[j].Index() })
		rp.Hands[seat] = tilesToStrings(ts)
		if seat < len(rs.exchangeSelection) {
			rp.ExchangeTiles[seat] = tilesToStrings(rs.exchangeSelection[seat])
		}
	}
	if rs.wall != nil {
		rp.WallRemaining = tilesToStrings(rs.wall.Tiles())
	}
	return json.Marshal(rp)
}

// RestoreRoundFromPersistJSON 从 JSON 恢复进行中牌局的最小运行态。
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
	if rp.SchemaVersion > roundPersistSchemaVersion {
		return nil, fmt.Errorf("%w: %d", ErrRoundPersistUnsupportedSchema, rp.SchemaVersion)
	}
	ruleID := rp.RuleID
	if ruleID == "" {
		ruleID = "sichuan_xzdd"
	}
	rule := rules.MustGet(ruleID)
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
		wall:                   wall.NewFromOrderedTiles(wallTiles),
		hands:                  hands,
		queBySeat:              append([]int32(nil), rp.QueBySeat...),
		waitingExchange:        rp.WaitingExchange,
		waitingQueMen:          rp.WaitingQueMen,
		exchangeSubmitted:      append([]bool(nil), rp.ExchangeDone...),
		exchangeDirection:      rp.ExchangeDir,
		exchangeSelection:      make([][]tile.Tile, 4),
		queSubmitted:           append([]bool(nil), rp.QueDone...),
		waitingDiscard:         rp.WaitingDiscard,
		waitingTsumo:           rp.WaitingTsumo,
		claimWindowOpen:        rp.ClaimWindowOpen,
		qiangGangWindow:        rp.QiangGangWindow,
		turn:                   rp.Turn,
		step:                   rp.Step,
		dealerSeat:             rp.DealerSeat,
		openingDrawSeat:        rp.OpeningDrawSeat,
		dealerFirstDiscardOpen: rp.DealerFirstDiscardOpen,
		winnerSeats:            append([]int(nil), rp.WinnerSeats...),
		huedSeats:              append([]bool(nil), rp.HuedSeats...),
		ledger:                 append([]sichuanxzdd.ScoreEntry(nil), rp.Ledger...),
		gangRecords:            append([]rules.GangRecord(nil), rp.GangRecords...),
		lastGangFollowUp:       rp.LastGangFollowUp,
		lastDiscardAfterGang:   rp.LastDiscardAfterGang,
	}
	for len(rs.queBySeat) < 4 {
		rs.queBySeat = append(rs.queBySeat, 0)
	}
	if rp.SchemaVersion < 3 {
		rs.dealerSeat = 0
		rs.openingDrawSeat = -1
		rs.dealerFirstDiscardOpen = false
	}
	for len(rs.exchangeSubmitted) < 4 {
		rs.exchangeSubmitted = append(rs.exchangeSubmitted, false)
	}
	if rs.exchangeDirection == 0 {
		rs.exchangeDirection = -1
	}
	for len(rs.exchangeSelection) < 4 {
		rs.exchangeSelection = append(rs.exchangeSelection, nil)
	}
	for len(rs.queSubmitted) < 4 {
		rs.queSubmitted = append(rs.queSubmitted, false)
	}
	if len(rs.winnerSeats) == 0 && rp.WinnerSeat >= 0 {
		rs.winnerSeats = append(rs.winnerSeats, rp.WinnerSeat)
	}
	for len(rs.huedSeats) < 4 {
		rs.huedSeats = append(rs.huedSeats, false)
	}
	for _, seat := range rs.winnerSeats {
		if seat >= 0 && seat < 4 {
			rs.huedSeats[seat] = true
		}
	}
	if len(rs.ledger) == 0 && len(rp.TotalFanBySeat) > 0 {
		rs.ledger = legacyLedgerFromTotals(rp.TotalFanBySeat)
	}
	if rp.PendingDraw != "" {
		t, err := tile.Parse(rp.PendingDraw)
		if err != nil {
			return nil, fmt.Errorf("parse pending draw: %w", err)
		}
		rs.pendingDraw = t
	}
	if rp.CurrentDraw != "" {
		t, err := tile.Parse(rp.CurrentDraw)
		if err != nil {
			return nil, fmt.Errorf("parse current draw: %w", err)
		}
		rs.currentDraw = t
	}
	if rp.LastDiscard != "" {
		t, err := tile.Parse(rp.LastDiscard)
		if err != nil {
			return nil, fmt.Errorf("parse last discard: %w", err)
		}
		rs.lastDiscard = t
		rs.lastDiscardSeat = rp.LastDiscardSeat
	} else {
		rs.lastDiscardSeat = -1
	}
	for _, candidate := range rp.ClaimCandidates {
		if candidate.Seat < 0 || candidate.Seat > 3 {
			return nil, fmt.Errorf("invalid claim candidate seat: %d", candidate.Seat)
		}
		actions := make([]string, 0, len(candidate.Actions))
		for _, action := range candidate.Actions {
			switch action {
			case "hu", "gang", "pong":
				actions = append(actions, action)
			default:
				return nil, fmt.Errorf("invalid claim candidate action: %s", action)
			}
		}
		if len(actions) > 0 {
			rs.claimCandidates = append(rs.claimCandidates, claimCandidate{seat: candidate.Seat, actions: actions})
		}
	}
	if rs.claimWindowOpen && len(rs.claimCandidates) == 0 {
		rs.claimCandidates = rs.buildClaimCandidates()
	}
	for seat := 0; seat < len(rp.ExchangeTiles) && seat < 4; seat++ {
		for _, raw := range rp.ExchangeTiles[seat] {
			t, err := tile.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("parse exchange tile %q: %w", raw, err)
			}
			rs.exchangeSelection[seat] = append(rs.exchangeSelection[seat], t)
		}
	}
	return rs, nil
}

func legacyLedgerFromTotals(totals []int32) []sichuanxzdd.ScoreEntry {
	out := make([]sichuanxzdd.ScoreEntry, 0, len(totals))
	for seat, total := range totals {
		if total == 0 {
			continue
		}
		from, to, amount := -1, seat, total
		if total < 0 {
			from, to, amount = seat, -1, -total
		}
		out = append(out, sichuanxzdd.ScoreEntry{
			Reason:     "legacy_total",
			FromSeat:   from,
			ToSeat:     to,
			Amount:     amount,
			Step:       0,
			WinnerSeat: -1,
		})
	}
	return out
}

// RoundViewFromPersistJSON 直接从持久化 JSON 还原等待态摘要，供快照 fallback 使用。
func RoundViewFromPersistJSON(roomID string, data []byte) (RoundView, error) {
	rs, err := RestoreRoundFromPersistJSON(roomID, data)
	if err != nil {
		return RoundView{}, err
	}
	return rs.SnapshotView(), nil
}

// QueSuitsFromPersistJSON 直接从持久化 JSON 读取定缺结果，供本地重连快照复用。
func QueSuitsFromPersistJSON(data []byte) ([]int32, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty round json")
	}
	var rp roundPersist
	if err := json.Unmarshal(data, &rp); err != nil {
		return nil, fmt.Errorf("unmarshal round json: %w", err)
	}
	if rp.SchemaVersion > roundPersistSchemaVersion {
		return nil, fmt.Errorf("%w: %d", ErrRoundPersistUnsupportedSchema, rp.SchemaVersion)
	}
	return append([]int32(nil), rp.QueBySeat...), nil
}
