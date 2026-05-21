package room

import (
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/metrics"
)

func (rs *RoundState) finishRound() (Notification, error) {
	settlement := rs.buildSettlement()
	for _, penalty := range settlement.Penalties {
		metrics.SettlementPenaltyTotal.WithLabelValues(penalty.GetReason()).Inc()
	}
	settlementPayload, err := buildSettlementNotification(rs.roomID, settlement)
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

func buildSettlementNotification(roomID string, settlement rules.SettlementResult) ([]byte, error) {
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: "settlement",
		Body: &clientv1.Envelope_Settlement{
			Settlement: &clientv1.SettlementNotify{
				RoomId:             roomID,
				WinnerUserIds:      settlement.WinnerUserIDs,
				TotalFan:           sumPositiveSeatScores(settlement.SeatScores),
				SeatScores:         settlement.SeatScores,
				Penalties:          settlement.Penalties,
				DetailText:         settlement.DetailText,
				PerWinnerBreakdown: settlement.PerWinnerBreakdown,
			},
		},
	})
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

func defaultSeatScores(playerIDs [4]string, scoreEvents []rules.ScoreEvent) []*clientv1.SeatScore {
	balances := seatBalancesFromScoreEvents(scoreEvents)
	out := make([]*clientv1.SeatScore, 0, len(balances))
	for seat, total := range balances {
		out = append(out, &clientv1.SeatScore{
			SeatIndex: int32(seat), //nolint:gosec // 固定四座位
			UserId:    playerIDs[seat],
			TotalFan:  total,
		})
	}
	return out
}

func sumPositiveSeatScores(scores []*clientv1.SeatScore) int32 {
	var total int32
	for _, score := range scores {
		if score.GetTotalFan() > 0 {
			total += score.GetTotalFan()
		}
	}
	return total
}
