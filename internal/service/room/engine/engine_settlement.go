package engine

import (
	"fmt"

	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/metrics"
	codec "racoo.cn/lsp/internal/service/room/engine/codec"
)

func (rs *RoundState) finishRound() (Notification, error) {
	settlement := rs.buildSettlement()
	for _, penalty := range settlement.Penalties {
		metrics.SettlementPenaltyTotal.WithLabelValues(penalty.Reason).Inc()
	}
	settlementPayload, err := buildSettlementNotification(fmt.Sprintf("settlement-%d", rs.step), rs.roomID, settlement)
	if err != nil {
		return Notification{}, err
	}
	rs.closed = true
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.lastDiscard = 0
	rs.lastDiscardSeat = -1
	rs.clearClaimWindow()
	return Notification{Kind: KindSettlement, Payload: settlementPayload, TargetSeat: BroadcastSeat}, nil
}

func (rs *RoundState) buildSettlement() rules.SettlementResult {
	if rs == nil {
		return rules.SettlementResult{}
	}
	rs.ensureRuleRuntime()
	return rs.caps.Settlement.BuildSettlement(rules.SettlementContext{
		PlayerIDs:   rs.playerIDs,
		Hands:       rs.hands,
		RuleState:   rs.ruleState,
		WinEvents:   append([]rules.WinEvent(nil), rs.winEvents...),
		ScoreEvents: append([]rules.ScoreEvent(nil), rs.scoreEvents...),
	})
}

func buildSettlementNotification(reqID, roomID string, settlement rules.SettlementResult) ([]byte, error) {
	data := codec.SettlementData{
		WinnerUserIDs:       append([]string(nil), settlement.WinnerUserIDs...),
		TotalFan:            sumPositiveSeatScores(settlement.SeatScores),
		SeatScores:          toCodecSeatScores(settlement.SeatScores),
		Penalties:           toCodecPenalties(settlement.Penalties),
		DetailText:          settlement.DetailText,
		PerWinnerBreakdowns: toCodecWinnerBreakdowns(settlement.PerWinnerBreakdown),
	}
	return codec.BuildSettlement(reqID, roomID, data)
}

func winnerUserIDs(playerIDs [4]string, winnerSeats []Seat) []string {
	winnerIDs := make([]string, 0, len(winnerSeats))
	for _, seat := range winnerSeats {
		if seat >= 0 && int(seat) < len(playerIDs) {
			winnerIDs = append(winnerIDs, playerIDs[seat])
		}
	}
	return winnerIDs
}

func defaultSeatScores(playerIDs [4]string, scoreEvents []rules.ScoreEvent) []*rules.SeatScore {
	balances := seatBalancesFromScoreEvents(scoreEvents)
	out := make([]*rules.SeatScore, 0, len(balances))
	for seat, total := range balances {
		out = append(out, &rules.SeatScore{
			SeatIndex: int32(seat), //nolint:gosec // 固定四座位
			UserID:    playerIDs[seat],
			TotalFan:  total,
		})
	}
	return out
}

func sumPositiveSeatScores(scores []*rules.SeatScore) int32 {
	var total int32
	for _, score := range scores {
		if score.TotalFan > 0 {
			total += score.TotalFan
		}
	}
	return total
}

func toCodecSeatScores(scores []*rules.SeatScore) []codec.SeatScoreData {
	out := make([]codec.SeatScoreData, 0, len(scores))
	for _, s := range scores {
		out = append(out, codec.SeatScoreData{
			SeatIndex: s.SeatIndex,
			UserID:    s.UserID,
			TotalFan:  s.TotalFan,
			Skipped:   s.Skipped,
		})
	}
	return out
}

func toCodecPenalties(penalties []*rules.PenaltyItem) []codec.PenaltyData {
	out := make([]codec.PenaltyData, 0, len(penalties))
	for _, p := range penalties {
		out = append(out, codec.PenaltyData{
			Reason:   p.Reason,
			FromSeat: p.FromSeat,
			ToSeat:   p.ToSeat,
			Amount:   p.Amount,
		})
	}
	return out
}

func toCodecWinnerBreakdowns(breakdowns []*rules.WinnerBreakdown) []codec.WinnerBreakdownData {
	out := make([]codec.WinnerBreakdownData, 0, len(breakdowns))
	for _, b := range breakdowns {
		out = append(out, codec.WinnerBreakdownData{
			SeatIndex: b.SeatIndex,
			UserID:    b.UserID,
			Fan:       b.Fan,
			FanNames:  append([]string(nil), b.FanNames...),
		})
	}
	return out
}
