package room

import (
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func (rs *RoundState) checkSeatHu(seat Seat, target tile.Tile, source rules.HuSource) (rules.HuResult, bool) {
	if rs == nil || rs.rule == nil || target == 0 || seat < 0 || seat >= SeatCount {
		return rules.HuResult{}, false
	}
	idx := int(seat)
	if idx >= len(rs.hands) || rs.hands[idx] == nil || rs.isHued(seat) || rs.isSurrendered(seat) {
		return rules.HuResult{}, false
	}
	return rs.rule.CheckHu(rs.hands[idx], target, rs.huContextForSeat(seat, source, target))
}

func (rs *RoundState) huContextForSeat(seat Seat, source rules.HuSource, pending tile.Tile) rules.HuContext {
	if rs == nil {
		return rules.HuContext{}
	}
	wallRemaining := 0
	if rs.wall != nil {
		wallRemaining = rs.wall.Remaining()
	}
	discarder := SeatInvalid
	responsible := SeatInvalid
	if source != rules.HuSourceTsumo {
		discarder = rs.lastDiscardSeat
		responsible = rs.lastDiscardSeat
	}
	ctx := rules.HuContext{
		Seat:            seat,
		Source:          source,
		PendingTile:     pending,
		Que:             queSuits(rs.queBySeat),
		Discarder:       discarder,
		IsHaiDi:         rs.isHaiDi(),
		IsGangShangHua:  source == rules.HuSourceTsumo && rs.lastGangFollowUp,
		ResponsibleSeat: responsible,
		GangHistory:     append([]rules.GangRecord(nil), rs.gangRecords...),
		Melds:           rs.meldContexts(seat),
		WallRemaining:   wallRemaining,
	}
	if source == rules.HuSourceQiangGang || rs.qiangGangWindow {
		ctx.Source = rules.HuSourceQiangGang
	}
	if rs.hasQueSuit(seat) {
		ctx.HasQueSuit = true
		ctx.QueSuit = tile.Suit(rs.queBySeat[seat])
	}
	return ctx
}

func (rs *RoundState) hasQueSuit(seat Seat) bool {
	if rs == nil || seat < 0 || int(seat) >= len(rs.queBySeat) {
		return false
	}
	if int(seat) >= len(rs.queSubmitted) || !rs.queSubmitted[seat] {
		return false
	}
	return rs.queBySeat[seat] >= 0 && rs.queBySeat[seat] <= 2
}
