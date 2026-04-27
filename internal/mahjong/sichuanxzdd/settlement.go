package sichuanxzdd

import (
	"fmt"
	"strings"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/tile"
)

const (
	ReasonHuTsumo     = "hu_tsumo"
	ReasonHuDiscard   = "hu_discard"
	ReasonHuQiangGang = "hu_qiang_gang"
	ReasonGangMing    = "gang_ming"
	ReasonGangBu      = "gang_bu"
	ReasonGangAn      = "gang_an"
	ReasonRefundMing  = "refund_ming_gang"
	ReasonRefundBu    = "refund_bu_gang"
	ReasonRefundAn    = "refund_an_gang"
	ReasonBaoPai      = "包牌"
	ReasonChaHuaZhu   = "查花猪"
	ReasonChaDaJiao   = "查大叫"
)

// ScoreEntry 是血战结算流水；FromSeat/ToSeat 为 -1 时表示系统池。
type ScoreEntry struct {
	Reason     string   `json:"reason"`
	FromSeat   int      `json:"from_seat"`
	ToSeat     int      `json:"to_seat"`
	Amount     int32    `json:"amount"`
	Step       int      `json:"step"`
	WinnerSeat int      `json:"winner_seat"`
	WinnerFan  int32    `json:"winner_fan,omitempty"`
	FanNames   []string `json:"fan_names,omitempty"`
}

// BuildSettlement 生成当前四川血战子集的结算摘要。
// 当前结算先汇总房间引擎已经计算好的胡牌与杠分，再补充查花猪、查大叫等局末罚分。
// 后续扩展番种或包牌规则时，应优先扩展规则上下文与结构化明细，而不是在传输层拼文本。
func BuildSettlement(playerIDs [4]string, hands []*hand.Hand, queBySeat []int32, ledger []ScoreEntry, winnerSeats []int) ([]*clientv1.SeatScore, []*clientv1.PenaltyItem, []*clientv1.WinnerBreakdown, string) {
	seatScores := make([]*clientv1.SeatScore, 0, 4)
	penalties := make([]*clientv1.PenaltyItem, 0)
	balances := foldLedger(ledger)
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
	breakdowns := buildWinnerBreakdowns(playerIDs, ledger, winners)
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
				appendPenalty(&penalties, balances, ReasonChaHuaZhu, seatIndex, toSeat, 2)
			}
		}
		if len(winners) > 0 {
			if _, ok := winners[seat]; !ok && !skipped && !isTing(hands[seat]) {
				for winnerSeat := range winners {
					winnerSeat32 := int32(winnerSeat) //nolint:gosec // G115：winnerSeat 仅可能为 0..3
					appendPenalty(&penalties, balances, ReasonChaDaJiao, seatIndex, winnerSeat32, 1)
				}
			}
		}
		appendRefundPenalties(&penalties, balances, ledger, seat, skipped, len(winners) == 0 && !isTing(hands[seat]))
		seatScores = append(seatScores, &clientv1.SeatScore{
			SeatIndex: seatIndex,
			UserId:    playerIDs[seat],
			TotalFan:  balances[seat],
			Skipped:   skipped,
		})
	}
	return seatScores, penalties, breakdowns, detail
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

func foldLedger(ledger []ScoreEntry) []int32 {
	balances := make([]int32, 4)
	for _, entry := range ledger {
		if entry.Amount <= 0 {
			continue
		}
		if entry.FromSeat >= 0 && entry.FromSeat < 4 {
			balances[entry.FromSeat] -= entry.Amount
		}
		if entry.ToSeat >= 0 && entry.ToSeat < 4 {
			balances[entry.ToSeat] += entry.Amount
		}
	}
	return balances
}

func appendPenalty(penalties *[]*clientv1.PenaltyItem, balances []int32, reason string, fromSeat, toSeat, amount int32) {
	*penalties = append(*penalties, &clientv1.PenaltyItem{
		Reason:   reason,
		FromSeat: fromSeat,
		ToSeat:   toSeat,
		Amount:   amount,
	})
	if fromSeat >= 0 && fromSeat < 4 {
		balances[fromSeat] -= amount
	}
	if toSeat >= 0 && toSeat < 4 {
		balances[toSeat] += amount
	}
}

func appendRefundPenalties(penalties *[]*clientv1.PenaltyItem, balances []int32, ledger []ScoreEntry, seat int, skipped, noTingDraw bool) {
	if !skipped && !noTingDraw {
		return
	}
	for _, entry := range ledger {
		if entry.ToSeat != seat || entry.FromSeat < 0 || entry.Amount <= 0 {
			continue
		}
		reason, ok := refundReason(entry.Reason, noTingDraw)
		if !ok {
			continue
		}
		appendPenalty(penalties, balances, reason, int32(seat), int32(entry.FromSeat), entry.Amount) //nolint:gosec // 座位范围固定
	}
}

func refundReason(reason string, includeAnGang bool) (string, bool) {
	switch reason {
	case ReasonGangMing:
		return ReasonRefundMing, true
	case ReasonGangBu:
		return ReasonRefundBu, true
	case ReasonGangAn:
		if includeAnGang {
			return ReasonRefundAn, true
		}
	}
	return "", false
}

func isTing(h *hand.Hand) bool {
	if h == nil {
		return false
	}
	return hu.IsTing(h.Counts())
}

func buildWinnerBreakdowns(playerIDs [4]string, ledger []ScoreEntry, winners map[int]struct{}) []*clientv1.WinnerBreakdown {
	bySeat := make(map[int]*clientv1.WinnerBreakdown, len(winners))
	seenName := make(map[int]map[string]struct{}, len(winners))
	for _, entry := range ledger {
		if entry.WinnerSeat < 0 || entry.WinnerSeat > 3 {
			continue
		}
		if _, ok := winners[entry.WinnerSeat]; !ok {
			continue
		}
		b := bySeat[entry.WinnerSeat]
		if b == nil {
			seatIndex := int32(entry.WinnerSeat) //nolint:gosec // 座位范围固定
			b = &clientv1.WinnerBreakdown{SeatIndex: seatIndex, UserId: playerIDs[entry.WinnerSeat]}
			bySeat[entry.WinnerSeat] = b
			seenName[entry.WinnerSeat] = make(map[string]struct{})
		}
		if entry.WinnerFan > b.Fan {
			b.Fan = entry.WinnerFan
		}
		for _, name := range entry.FanNames {
			if name == "" {
				continue
			}
			if _, ok := seenName[entry.WinnerSeat][name]; ok {
				continue
			}
			seenName[entry.WinnerSeat][name] = struct{}{}
			b.FanNames = append(b.FanNames, name)
		}
	}
	out := make([]*clientv1.WinnerBreakdown, 0, len(winners))
	for seat := 0; seat < 4; seat++ {
		if b := bySeat[seat]; b != nil {
			out = append(out, b)
		}
	}
	return out
}
