package room

import (
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/metrics"
)

func (rs *RoundState) finishRound() (Notification, error) {
	settlement := rs.buildSettlement()
	for _, penalty := range settlement.Penalties {
		metrics.SettlementPenaltyTotal.WithLabelValues(penalty.Reason).Inc()
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
				SeatScores:         toProtoSeatScores(settlement.SeatScores),
				Penalties:          toProtoPenalties(settlement.Penalties),
				DetailText:         settlement.DetailText,
				PerWinnerBreakdown: toProtoWinnerBreakdowns(settlement.PerWinnerBreakdown),
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

// toProtoRuleMeta 将内部 RuleMeta 转换为传输层 proto 类型（序列化边界）。
func toProtoRuleMeta(m *RuleMeta) *clientv1.RuleMeta {
	if m == nil {
		return nil
	}
	return &clientv1.RuleMeta{
		RuleId:          m.RuleID,
		DisplayName:     m.DisplayName,
		ShortDesc:       m.ShortDesc,
		EnabledFeatures: append([]string(nil), m.EnabledFeatures...),
		MaxHands:        m.MaxHands,
	}
}

// toProtoSeatScores 将规则层内部类型转换为传输层 proto 类型（唯一序列化边界）。
func toProtoSeatScores(scores []*rules.SeatScore) []*clientv1.SeatScore {
	out := make([]*clientv1.SeatScore, 0, len(scores))
	for _, s := range scores {
		out = append(out, &clientv1.SeatScore{
			SeatIndex: s.SeatIndex,
			UserId:    s.UserID,
			TotalFan:  s.TotalFan,
			Skipped:   s.Skipped,
		})
	}
	return out
}

// toProtoPenalties 将规则层内部类型转换为传输层 proto 类型（唯一序列化边界）。
func toProtoPenalties(penalties []*rules.PenaltyItem) []*clientv1.PenaltyItem {
	out := make([]*clientv1.PenaltyItem, 0, len(penalties))
	for _, p := range penalties {
		out = append(out, &clientv1.PenaltyItem{
			Reason:   p.Reason,
			FromSeat: p.FromSeat,
			ToSeat:   p.ToSeat,
			Amount:   p.Amount,
		})
	}
	return out
}

// toProtoWinnerBreakdowns 将规则层内部类型转换为传输层 proto 类型（唯一序列化边界）。
func toProtoWinnerBreakdowns(breakdowns []*rules.WinnerBreakdown) []*clientv1.WinnerBreakdown {
	out := make([]*clientv1.WinnerBreakdown, 0, len(breakdowns))
	for _, b := range breakdowns {
		out = append(out, &clientv1.WinnerBreakdown{
			SeatIndex: b.SeatIndex,
			UserId:    b.UserID,
			Fan:       b.Fan,
			FanNames:  b.FanNames,
		})
	}
	return out
}
