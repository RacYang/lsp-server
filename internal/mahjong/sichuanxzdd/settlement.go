package sichuanxzdd

import (
	"fmt"
	"strings"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// BuildSettlement 生成当前四川血战子集的结算摘要。
// 当前结算先汇总房间引擎已经计算好的胡牌与杠分，再补充查花猪、查大叫等局末罚分。
// 后续扩展番种或包牌规则时，应优先扩展规则上下文与结构化明细，而不是在传输层拼文本。
func BuildSettlement(playerIDs [4]string, hands []*hand.Hand, queBySeat []int32, winnerSeats []int, totalFanBySeat []int32) ([]*clientv1.SeatScore, []*clientv1.PenaltyItem, string) {
	seatScores := make([]*clientv1.SeatScore, 0, 4)
	penalties := make([]*clientv1.PenaltyItem, 0)
	detail := "荒牌"
	winners := map[int]struct{}{}
	if len(winnerSeats) > 0 {
		parts := make([]string, 0, len(winnerSeats))
		for _, seat := range winnerSeats {
			winners[seat] = struct{}{}
			parts = append(parts, fmt.Sprintf("座位 %d", seat))
		}
		detail = strings.Join(parts, "、") + " 胡牌"
	}
	for seat := 0; seat < 4; seat++ {
		seatIndex := int32(seat) //nolint:gosec // G115：seat 仅在 0..3 范围
		skipped := false
		if holdsQueSuit(hands[seat], tile.Suit(queBySeat[seat])) {
			skipped = true
			for to := 0; to < 4; to++ {
				toSeat := int32(to) //nolint:gosec // G115：to 仅在 0..3 范围
				if to == seat {
					continue
				}
				penalties = append(penalties, &clientv1.PenaltyItem{
					Reason:   "查花猪",
					FromSeat: seatIndex,
					ToSeat:   toSeat,
					Amount:   2,
				})
			}
		}
		if len(winners) > 0 {
			if _, ok := winners[seat]; !ok && totalFanBySeat[seat] == 0 {
				for winnerSeat := range winners {
					winnerSeat32 := int32(winnerSeat) //nolint:gosec // G115：winnerSeat 仅可能为 0..3
					penalties = append(penalties, &clientv1.PenaltyItem{
						Reason:   "查大叫",
						FromSeat: seatIndex,
						ToSeat:   winnerSeat32,
						Amount:   1,
					})
				}
			}
		}
		seatScores = append(seatScores, &clientv1.SeatScore{
			SeatIndex: seatIndex,
			UserId:    playerIDs[seat],
			TotalFan:  totalFanBySeat[seat],
			Skipped:   skipped,
		})
	}
	return seatScores, penalties, detail
}

// holdsQueSuit 判断玩家手牌是否仍持有定缺花色，用于查花猪。
func holdsQueSuit(h *hand.Hand, queSuit tile.Suit) bool {
	for _, t := range h.Tiles() {
		if t.Suit() == queSuit {
			return true
		}
	}
	return false
}
