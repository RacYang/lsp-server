package engine

import (
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func (rs *RoundState) checkSeatHu(seat Seat, target tile.Tile, source rules.HuSource) (rules.HuResult, bool) {
	if rs == nil || target == 0 || seat < 0 || seat >= SeatCount {
		return rules.HuResult{}, false
	}
	rs.ensureRuleRuntime()
	idx := int(seat)
	if idx >= len(rs.hands) || rs.hands[idx] == nil || rs.isSeatOutAfterHu(seat) || rs.isSurrendered(seat) {
		return rules.HuResult{}, false
	}
	winPolicy := rs.caps.Win
	return winPolicy.CheckHu(rs.hands[idx], target, rs.huContextForSeat(seat, source, target))
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
		RuleState:       rs.ruleState,
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
	return ctx
}
