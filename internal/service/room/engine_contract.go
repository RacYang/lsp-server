package room

import (
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func (rs *RoundState) wallRemaining() int32 {
	if rs == nil || rs.wall == nil {
		return 0
	}
	return int32(rs.wall.Remaining()) //nolint:gosec // 四川牌墙剩余张数小于 int32 上限
}

func (rs *RoundState) ruleMeta() *clientv1.RuleMeta {
	if rs == nil {
		return nil
	}
	meta := rules.CapabilitiesOf(rs.rule).Metadata
	if meta.DisplayName == "" && rs.rule != nil {
		meta.DisplayName = rs.rule.Name()
	}
	return &clientv1.RuleMeta{
		RuleId:          rs.ruleID,
		DisplayName:     meta.DisplayName,
		ShortDesc:       meta.ShortDesc,
		EnabledFeatures: append([]string(nil), meta.EnabledFeatures...),
		MaxHands:        meta.MaxHands,
	}
}

func (rs *RoundState) totalScores() []*clientv1.SeatScore {
	if rs == nil {
		return nil
	}
	balances := seatBalancesFromLedger(rs.ledger)
	out := make([]*clientv1.SeatScore, 0, 4)
	for seat := 0; seat < 4; seat++ {
		userID := ""
		if seat < len(rs.playerIDs) {
			userID = rs.playerIDs[seat]
		}
		out = append(out, &clientv1.SeatScore{
			SeatIndex: int32(seat), //nolint:gosec // 固定座位范围 0..3
			UserId:    userID,
			TotalFan:  balances[seat],
		})
	}
	return out
}

func (rs *RoundState) roundProgress() RoundProgress {
	return ProjectRoundState(rs).Progress
}

func (rs *RoundState) drawTransitionProgress() RoundProgress {
	progress := rs.roundProgress()
	progress.Phase = clientv1.Phase_PHASE_DRAW
	progress.WaitingAction = "none"
	if rs != nil {
		progress.ActingSeat = rs.turn.Proto()
		progress.ActingSeats = []int32{rs.turn.Proto()}
		progress.Step = int64(rs.step)
		progress.WallRemaining = rs.wallRemaining()
		progress.DeadlineUnixMs = rs.deadlineUnixMs
		progress.Reason = rs.phaseReason
	}
	progress.AvailableActions = nil
	progress.ClaimCandidates = nil
	progress.PendingTile = ""
	return progress
}

func (p RoundProgress) applyToAction(action *clientv1.ActionNotify) {
	if action == nil {
		return
	}
	action.Phase = p.Phase
	action.Step = p.Step
	action.ActingSeats = append([]int32(nil), p.ActingSeats...)
	action.WallRemaining = p.WallRemaining
	action.DeadlineUnixMs = p.DeadlineUnixMs
	action.PhaseUpdate = p.toPhaseUpdate()
}

func (p RoundProgress) applyToDraw(draw *clientv1.DrawTileNotify) {
	if draw == nil {
		return
	}
	draw.Phase = p.Phase
	draw.Step = p.Step
	draw.ActingSeats = append([]int32(nil), p.ActingSeats...)
	draw.WallRemaining = p.WallRemaining
	draw.DeadlineUnixMs = p.DeadlineUnixMs
	draw.PhaseUpdate = p.toPhaseUpdate()
}

func (p RoundProgress) applyToStart(start *clientv1.StartGameNotify) {
	if start == nil {
		return
	}
	start.Phase = p.Phase
	start.Step = p.Step
	start.ActingSeats = append([]int32(nil), p.ActingSeats...)
	start.WallRemaining = p.WallRemaining
	start.PhaseUpdate = p.toPhaseUpdate()
}

func (p RoundProgress) applyToExchangeDone(done *clientv1.ExchangeThreeDoneNotify) {
	if done == nil {
		return
	}
	done.Phase = p.Phase
	done.Step = p.Step
	done.ActingSeats = append([]int32(nil), p.ActingSeats...)
	done.PhaseUpdate = p.toPhaseUpdate()
}

func (p RoundProgress) applyToQueMenDone(done *clientv1.QueMenDoneNotify) {
	if done == nil {
		return
	}
	done.Phase = p.Phase
	done.Step = p.Step
	done.ActingSeats = append([]int32(nil), p.ActingSeats...)
	done.PhaseUpdate = p.toPhaseUpdate()
}

func (rs *RoundState) actionDetail(actor Seat, action string, t tile.Tile, target Seat, source Seat) *clientv1.ActionDetail {
	if rs == nil {
		return nil
	}
	tileText := ""
	if t != 0 {
		tileText = t.String()
	}
	return &clientv1.ActionDetail{
		Step:       int64(rs.step),
		ActorSeat:  actor.Proto(),
		Action:     action,
		Tile:       tileText,
		TargetSeat: target.Proto(),
		SourceSeat: source.Proto(),
	}
}

func (rs *RoundState) rememberLastAction(detail *clientv1.ActionDetail) {
	if rs == nil || detail == nil {
		return
	}
	rs.lastAction = &clientv1.LastActionInfo{
		Step:        detail.GetStep(),
		ActorSeat:   detail.GetActorSeat(),
		Action:      detail.GetAction(),
		Tile:        detail.GetTile(),
		TargetSeat:  detail.GetTargetSeat(),
		SourceSeat:  detail.GetSourceSeat(),
		CreatedAtMs: detail.GetCreatedAtMs(),
	}
}

func (rs *RoundState) meldInfosBySeat() []*clientv1.SeatMelds {
	out := make([]*clientv1.SeatMelds, 0, 4)
	if rs == nil {
		return out
	}
	for seat := 0; seat < 4; seat++ {
		seatMelds := &clientv1.SeatMelds{SeatIndex: int32(seat)} //nolint:gosec // 固定座位范围 0..3
		if seat < len(rs.melds) {
			for _, encoded := range rs.melds[seat] {
				if info := meldInfoFromEncoded(Seat(seat), encoded, rs.step); info != nil {
					seatMelds.Melds = append(seatMelds.Melds, info)
				}
			}
		}
		out = append(out, seatMelds)
	}
	return out
}

func meldInfoFromEncoded(seat Seat, encoded string, step int) *clientv1.MeldInfo {
	fact, ok := parseMeldFact(encoded)
	if !ok || fact.Kind == "" {
		return nil
	}
	var count int
	switch fact.Kind {
	case "pong":
		count = 3
	case "gang", "zhi_gang", "an_gang", "bu_gang":
		count = 4
	case "chi", "chow":
		count = 3
	default:
		return nil
	}
	tiles := make([]string, 0, count)
	if len(fact.Tiles) == 1 && count > 1 {
		for i := 0; i < count; i++ {
			tiles = append(tiles, fact.Tiles[0].String())
		}
	} else {
		for _, t := range fact.Tiles {
			tiles = append(tiles, t.String())
		}
	}
	return &clientv1.MeldInfo{
		SeatIndex:       seat.Proto(),
		Kind:            fact.Kind,
		Tiles:           tiles,
		ClaimedFromSeat: fact.ClaimedFrom.Proto(),
		Concealed:       fact.Concealed,
		Step:            int64(step),
	}
}
