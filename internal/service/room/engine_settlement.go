package room

import (
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/sichuanxzdd"
	"racoo.cn/lsp/internal/metrics"
)

func (rs *RoundState) finishRound() (Notification, error) {
	seatScores, penalties, breakdowns, detail := sichuanxzdd.BuildSettlement(rs.playerIDs, rs.hands, rs.queBySeat, rs.ledger, rs.winnerSeats)
	for _, penalty := range penalties {
		metrics.SettlementPenaltyTotal.WithLabelValues(penalty.GetReason()).Inc()
	}
	settlementPayload, err := buildSettlementNotification(rs.roomID, rs.playerIDs, rs.winnerSeats, seatScores, penalties, breakdowns, detail)
	if err != nil {
		return Notification{}, err
	}
	rs.closed = true
	rs.waitingDiscard = false
	rs.waitingTsumo = false
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.lastDiscard = 0
	rs.lastDiscardSeat = -1
	rs.clearClaimWindow()
	return Notification{Kind: KindSettlement, Payload: settlementPayload}, nil
}

func buildSettlementNotification(roomID string, playerIDs [4]string, winnerSeats []int, seatScores []*clientv1.SeatScore, penalties []*clientv1.PenaltyItem, breakdowns []*clientv1.WinnerBreakdown, detail string) ([]byte, error) {
	winnerIDs := make([]string, 0, len(winnerSeats))
	for _, seat := range winnerSeats {
		if seat >= 0 && seat < len(playerIDs) {
			winnerIDs = append(winnerIDs, playerIDs[seat])
		}
	}
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: "settlement",
		Body: &clientv1.Envelope_Settlement{
			Settlement: &clientv1.SettlementNotify{
				RoomId:             roomID,
				WinnerUserIds:      winnerIDs,
				TotalFan:           sumPositiveSeatScores(seatScores),
				SeatScores:         seatScores,
				Penalties:          penalties,
				DetailText:         detail,
				PerWinnerBreakdown: breakdowns,
			},
		},
	})
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
