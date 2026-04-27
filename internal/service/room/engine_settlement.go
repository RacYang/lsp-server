package room

import (
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/sichuanxzdd"
)

func (rs *RoundState) finishRound() (Notification, error) {
	seatScores, penalties, detail := sichuanxzdd.BuildSettlement(rs.playerIDs, rs.hands, rs.queBySeat, rs.winnerSeats, rs.totalFanBySeat)
	winnerIDs := make([]string, 0, len(rs.winnerSeats))
	for _, seat := range rs.winnerSeats {
		if seat >= 0 && seat < len(rs.playerIDs) {
			winnerIDs = append(winnerIDs, rs.playerIDs[seat])
		}
	}
	settlementPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "settlement",
		Body: &clientv1.Envelope_Settlement{
			Settlement: &clientv1.SettlementNotify{
				RoomId:        rs.roomID,
				WinnerUserIds: winnerIDs,
				TotalFan:      sumInt32(rs.totalFanBySeat),
				SeatScores:    seatScores,
				Penalties:     penalties,
				DetailText:    detail,
			},
		},
	})
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

func sumInt32(xs []int32) int32 {
	var total int32
	for _, x := range xs {
		total += x
	}
	return total
}
