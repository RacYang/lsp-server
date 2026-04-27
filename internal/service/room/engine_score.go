package room

import (
	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/sichuanxzdd"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// appendHuEntries 把一次胡牌转换为可审计结算流水；自摸三家支付，点炮与抢杠只由责任座位支付。
func appendHuEntries(rs *RoundState, winner, fanTotal int, source rules.HuSource, payer int, breakdown fan.Breakdown) {
	if rs == nil || winner < 0 || winner > 3 || fanTotal <= 0 {
		return
	}
	reason := sichuanxzdd.ReasonHuTsumo
	switch source {
	case rules.HuSourceDiscard:
		reason = sichuanxzdd.ReasonHuDiscard
	case rules.HuSourceQiangGang:
		reason = sichuanxzdd.ReasonHuQiangGang
	}
	amount := int32(fanTotal) //nolint:gosec // 番数很小
	names := fanLabels(breakdown)
	if source == rules.HuSourceQiangGang && payer >= 0 {
		names = append(names, sichuanxzdd.ReasonBaoPai)
	}
	if source == rules.HuSourceTsumo {
		for other := 0; other < 4; other++ {
			if other == winner || rs.isHued(other) {
				continue
			}
			rs.ledger = append(rs.ledger, huScoreEntry(reason, other, winner, amount, rs.step, winner, names))
		}
		return
	}
	if payer >= 0 && payer < 4 && payer != winner {
		rs.ledger = append(rs.ledger, huScoreEntry(reason, payer, winner, amount, rs.step, winner, names))
	}
}

// appendGangEntries 记录一笔杠分流水，并保留原始 GangRecord 供抢杠、退税与番种计算使用。
func appendGangEntries(rs *RoundState, seat int, gangTile tile.Tile, kind rules.GangKind, fromSeat int) {
	if rs == nil || seat < 0 || seat > 3 {
		return
	}
	amount := int32(1)
	reason := sichuanxzdd.ReasonGangMing
	switch kind {
	case rules.GangKindAn:
		amount = 2
		reason = sichuanxzdd.ReasonGangAn
	case rules.GangKindBu:
		reason = sichuanxzdd.ReasonGangBu
	}
	for other := 0; other < 4; other++ {
		if other == seat || rs.isHued(other) {
			continue
		}
		rs.ledger = append(rs.ledger, sichuanxzdd.ScoreEntry{
			Reason:     reason,
			FromSeat:   other,
			ToSeat:     seat,
			Amount:     amount,
			Step:       rs.step,
			WinnerSeat: -1,
		})
	}
	rs.gangRecords = append(rs.gangRecords, rules.GangRecord{
		Seat:            seat,
		Kind:            kind,
		Tile:            gangTile,
		FromSeat:        fromSeat,
		ResponsibleSeat: fromSeat,
		Step:            rs.step,
	})
	rs.lastGangFollowUp = true
}

// huScoreEntry 统一胡牌流水字段，避免各个动作分支分别拼装结算结构。
func huScoreEntry(reason string, from, to int, amount int32, step, winner int, names []string) sichuanxzdd.ScoreEntry {
	return sichuanxzdd.ScoreEntry{
		Reason:     reason,
		FromSeat:   from,
		ToSeat:     to,
		Amount:     amount,
		Step:       step,
		WinnerSeat: winner,
		WinnerFan:  amount,
		FanNames:   append([]string(nil), names...),
	}
}

// fanLabels 提取中文番种名；若规则实现未给 label，则回退为稳定的 fan.Kind 字符串。
func fanLabels(b fan.Breakdown) []string {
	out := make([]string, 0, len(b.Items))
	for _, item := range b.Items {
		if item.Label != "" {
			out = append(out, item.Label)
			continue
		}
		out = append(out, string(item.Kind))
	}
	return out
}

// seatBalancesFromLedger 将流水折叠成四个座位的当前分数，用于推送与测试断言。
func seatBalancesFromLedger(ledger []sichuanxzdd.ScoreEntry) []int32 {
	out := make([]int32, 4)
	for _, entry := range ledger {
		if entry.Amount <= 0 {
			continue
		}
		if entry.FromSeat >= 0 && entry.FromSeat < 4 {
			out[entry.FromSeat] -= entry.Amount
		}
		if entry.ToSeat >= 0 && entry.ToSeat < 4 {
			out[entry.ToSeat] += entry.Amount
		}
	}
	return out
}
