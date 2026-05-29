package room

import "racoo.cn/lsp/internal/mahjong/rules"

// IsClosed 报告局面是否已关闭（结算完成或弃局）。
func (rs *RoundState) IsClosed() bool {
	if rs == nil {
		return false
	}
	return rs.closed
}

// Step 返回当前步骤序号，仅用于日志与 PhaseToken 校验。
func (rs *RoundState) Step() int {
	if rs == nil {
		return 0
	}
	return rs.step
}

// WaitingKind 返回当前等待态的可读名称；供 actor 日志使用。
func (rs *RoundState) WaitingKind() string {
	return rs.waitingKind()
}

// ValidatePhaseToken 校验客户端 PhaseToken 与当前局面阶段是否一致。
func (rs *RoundState) ValidatePhaseToken(tok *PhaseToken) error {
	return rs.validatePhaseToken(tok)
}

// MarkSeatSurrendered 将指定座位标记为已投降（离线/托管），并确保 slice 已初始化。
func (rs *RoundState) MarkSeatSurrendered(seat Seat) {
	if rs == nil || seat < 0 || seat >= SeatCount {
		return
	}
	if len(rs.surrendered) < int(SeatCount) {
		rs.surrendered = make([]bool, SeatCount)
	}
	rs.surrendered[seat] = true
}

// SurrenderedAt 报告指定座位是否已标记为投降。
func (rs *RoundState) SurrenderedAt(seat int) bool {
	if rs == nil || seat < 0 || seat >= int(SeatCount) || seat >= len(rs.surrendered) {
		return false
	}
	return rs.surrendered[seat]
}

// RoundScoreBalances 返回各座位在本局的净得分（索引对应座位号）。
func (rs *RoundState) RoundScoreBalances() []int32 {
	if rs == nil {
		return make([]int32, 4)
	}
	return seatBalancesFromScoreEvents(rs.scoreEvents)
}

// ScoreEvents 返回本局计分事件切片的只读副本。
func (rs *RoundState) ScoreEvents() []rules.ScoreEvent {
	if rs == nil {
		return nil
	}
	return append([]rules.ScoreEvent(nil), rs.scoreEvents...)
}
