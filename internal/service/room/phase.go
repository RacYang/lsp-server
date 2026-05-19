// Package room：阶段所有权与 deadline 派生工具，详见 ADR-0045。
//
// 本文件落地"deadline 单一所有权"契约：
//   - WaitingReason 是 RoundState 等待态的唯一权威枚举；
//   - RoundState.enterPhase 是 phaseReason / phaseStartUnixMs / 等待态标志位的唯一写入入口；
//   - RoundState.Deadline 是 deadlineUnixMs 的唯一读取入口（派生自 phaseStart + cfg.DurationFor）。
//
// engine 任何分支只允许通过 enterPhase 切换阶段；scheduler 仅按 Deadline() 对齐 OS 定时器，
// 不再写入 RoundState。具体约束由 .cursor/rules/room-phase-owner.mdc 与 enforcer 校验。
package room

import (
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// WaitingReason 表示房间当前等待的玩家动作类型。值域与 client.v1.WaitingReason 保持同语义；
// ReasonNone 表示无等待（如 settling 与 closed），此时 Deadline() 返回 0。
type WaitingReason int

const (
	ReasonNone WaitingReason = iota
	ReasonExchangeThree
	ReasonQueMen
	ReasonClaimWindow
	ReasonTsumo
	ReasonDiscard
	// ReasonSurrender 是预留枚举值；实际"托管者已掉线"路径走 ReasonXxx + surrender=true 缩短时长。
	ReasonSurrender
)

// String 用于结构化日志输出。
func (r WaitingReason) String() string {
	switch r {
	case ReasonExchangeThree:
		return "exchange_three"
	case ReasonQueMen:
		return "que_men"
	case ReasonClaimWindow:
		return "claim_window"
	case ReasonTsumo:
		return "tsumo"
	case ReasonDiscard:
		return "discard"
	case ReasonSurrender:
		return "surrender"
	default:
		return "none"
	}
}

// Proto 返回与 client.v1.WaitingReason 对齐的 proto 枚举值。
func (r WaitingReason) Proto() clientv1.WaitingReason {
	switch r {
	case ReasonExchangeThree:
		return clientv1.WaitingReason_WAITING_REASON_EXCHANGE_THREE
	case ReasonQueMen:
		return clientv1.WaitingReason_WAITING_REASON_QUE_MEN
	case ReasonClaimWindow:
		return clientv1.WaitingReason_WAITING_REASON_CLAIM_WINDOW
	case ReasonTsumo:
		return clientv1.WaitingReason_WAITING_REASON_TSUMO
	case ReasonDiscard:
		return clientv1.WaitingReason_WAITING_REASON_DISCARD
	case ReasonSurrender:
		return clientv1.WaitingReason_WAITING_REASON_SURRENDER
	default:
		return clientv1.WaitingReason_WAITING_REASON_NONE
	}
}

// WaitingReasonFromProto 将 proto 枚举映射回 Go 枚举。
func WaitingReasonFromProto(p clientv1.WaitingReason) WaitingReason {
	switch p {
	case clientv1.WaitingReason_WAITING_REASON_EXCHANGE_THREE:
		return ReasonExchangeThree
	case clientv1.WaitingReason_WAITING_REASON_QUE_MEN:
		return ReasonQueMen
	case clientv1.WaitingReason_WAITING_REASON_CLAIM_WINDOW:
		return ReasonClaimWindow
	case clientv1.WaitingReason_WAITING_REASON_TSUMO:
		return ReasonTsumo
	case clientv1.WaitingReason_WAITING_REASON_DISCARD:
		return ReasonDiscard
	case clientv1.WaitingReason_WAITING_REASON_SURRENDER:
		return ReasonSurrender
	default:
		return ReasonNone
	}
}

// DurationFor 返回某 reason 对应的服务端等待时长；surrender=true 缩短为 SurrenderAction 时长。
// 这是 TimeoutConfig 的纯函数视图，scheduler 与 enterPhase 共享同一来源。
func (cfg TimeoutConfig) DurationFor(reason WaitingReason, surrender bool) time.Duration {
	if surrender {
		return cfg.SurrenderAction
	}
	switch reason {
	case ReasonExchangeThree:
		return cfg.ExchangeThree
	case ReasonQueMen:
		return cfg.QueMen
	case ReasonClaimWindow:
		return cfg.ClaimWindow
	case ReasonTsumo:
		return cfg.TsumoWindow
	case ReasonDiscard:
		return cfg.Discard
	default:
		return 0
	}
}

// enterPhase 是 RoundState 阶段切换的唯一写入入口。
//
// 不变量：
//  1. 该方法是 phaseReason / phaseStartUnixMs / waitingExchange / waitingQueMen /
//     claimWindowOpen / waitingTsumo / waitingDiscard 七个字段的唯一写入位置。
//  2. 调用时刻立即计算 deadlineUnixMs；engine 任意构造 RoundProgress / PhaseUpdate
//     的代码均在调用 enterPhase 之后取值，从结构上消除 stale-deadline 错位。
//  3. ReasonNone 时清零 deadlineUnixMs 与 phaseStartUnixMs，scheduler 据此停表。
//
// 详见 ADR-0045。
func (rs *RoundState) enterPhase(reason WaitingReason) {
	if rs == nil {
		return
	}
	rs.waitingExchange = false
	rs.waitingQueMen = false
	rs.claimWindowOpen = false
	rs.waitingTsumo = false
	rs.waitingDiscard = false
	switch reason {
	case ReasonExchangeThree:
		rs.waitingExchange = true
	case ReasonQueMen:
		rs.waitingQueMen = true
	case ReasonClaimWindow:
		rs.claimWindowOpen = true
	case ReasonTsumo:
		rs.waitingTsumo = true
	case ReasonDiscard:
		rs.waitingDiscard = true
	}
	rs.phaseReason = reason
	if reason == ReasonNone || rs.clk == nil {
		rs.phaseStartUnixMs = 0
		rs.deadlineUnixMs = 0
		return
	}
	surrender := rs.surrenderedWaitingSeat(reason)
	rs.phaseStartUnixMs = rs.clk.Now().UnixMilli()
	if dur := rs.tmo.DurationFor(reason, surrender); dur > 0 {
		rs.deadlineUnixMs = rs.phaseStartUnixMs + dur.Milliseconds()
	} else {
		rs.deadlineUnixMs = 0
	}
}

// surrenderedWaitingSeat 判断 reason 当前等待的目标座位是否处于托管/掉线状态。
// 与 scheduler.surrenderedSeatWaiting 同源。
func (rs *RoundState) surrenderedWaitingSeat(reason WaitingReason) bool {
	if rs == nil {
		return false
	}
	switch reason {
	case ReasonExchangeThree:
		for seat, done := range rs.exchangeSubmitted {
			if !done && rs.isSurrendered(Seat(seat)) {
				return true
			}
		}
	case ReasonQueMen:
		for seat, done := range rs.queSubmitted {
			if !done && rs.isSurrendered(Seat(seat)) {
				return true
			}
		}
	case ReasonClaimWindow:
		if candidate, ok := rs.bestClaimCandidate(); ok {
			return rs.isSurrendered(candidate.seat)
		}
	case ReasonTsumo, ReasonDiscard:
		return rs.isSurrendered(rs.turn)
	}
	return false
}

// PhaseReason 返回 RoundState 当前权威等待原因；ReasonNone 表示无等待。
func (rs *RoundState) PhaseReason() WaitingReason {
	if rs == nil {
		return ReasonNone
	}
	return rs.phaseReason
}

// PhaseStartUnixMs 返回当前阶段进入时的服务端时间戳（毫秒）；ReasonNone 时返回 0。
func (rs *RoundState) PhaseStartUnixMs() int64 {
	if rs == nil {
		return 0
	}
	return rs.phaseStartUnixMs
}

// Deadline 返回当前阶段的派生截止时间；ReasonNone 或时钟未注入时返回 0。
func (rs *RoundState) Deadline() int64 {
	if rs == nil {
		return 0
	}
	return rs.deadlineUnixMs
}

// PhaseToken 是客户端动作请求中携带的轻量阶段令牌；详见 ADR-0045。
// 服务端 actor 在执行任何状态变更动作前比对 (Step, Reason) 与当前 RoundState；
// 若客户端令牌过时（典型场景：网络/UI 延迟、托管超时已推进），返回 PhaseDriftError
// 并由 handler 映射为 ERROR_CODE_PHASE_DRIFTED 携带最新 PhaseUpdate 回写客户端。
type PhaseToken struct {
	Step   int64
	Reason WaitingReason
}

// PhaseDriftError 表示客户端令牌与服务端当前阶段不一致；Current 字段总是非 nil，
// 调用方据此回写最新阶段视图给客户端，避免客户端基于陈旧 UI 再次提交。
type PhaseDriftError struct {
	Token    PhaseToken
	Current  PhaseToken
	Progress RoundProgress
}

// Error 描述 drift 的关键字段，便于日志与单测断言。
func (e *PhaseDriftError) Error() string {
	return "phase drifted"
}

func (e *PhaseDriftError) PhaseUpdate() *clientv1.PhaseUpdate {
	if e == nil {
		return nil
	}
	if e.Progress.Step != 0 || e.Progress.Phase != clientv1.Phase_PHASE_UNSPECIFIED || e.Progress.Reason != ReasonNone {
		return e.Progress.toPhaseUpdate()
	}
	return &clientv1.PhaseUpdate{
		Step:   e.Current.Step,
		Reason: e.Current.Reason.Proto(),
	}
}

// validatePhaseToken 在 actor 处理任意状态变更动作前调用。
//
//   - tok == nil 视作"未携带令牌"，保留向后兼容（老客户端 / 重连早期帧）。
//   - tok 与 (rs.step, rs.phaseReason) 完全一致时返回 nil。
//   - 否则返回 *PhaseDriftError，Current 字段反映服务端权威阶段。
func (rs *RoundState) validatePhaseToken(tok *PhaseToken) error {
	if rs == nil || tok == nil {
		return nil
	}
	if tok.Step == int64(rs.step) && tok.Reason == rs.phaseReason {
		return nil
	}
	return &PhaseDriftError{
		Token:    *tok,
		Current:  PhaseToken{Step: int64(rs.step), Reason: rs.phaseReason},
		Progress: rs.roundProgress(),
	}
}

// PhaseTokenFromProto 将 proto PhaseToken 映射为 Go 侧值；nil 输入返回 nil。
func PhaseTokenFromProto(p *clientv1.PhaseToken) *PhaseToken {
	if p == nil {
		return nil
	}
	return &PhaseToken{Step: p.GetStep(), Reason: WaitingReasonFromProto(p.GetReason())}
}

// PhaseTransition 描述 engine 单步推进产生的阶段切换；P3 actor 模板使用。
type PhaseTransition struct {
	From, To       WaitingReason
	StartUnixMs    int64
	DeadlineUnixMs int64
	Notifications  []Notification
}

func (rs *RoundState) phase() clientv1.Phase {
	if rs == nil {
		return clientv1.Phase_PHASE_UNSPECIFIED
	}
	switch {
	case rs.closed:
		return clientv1.Phase_PHASE_CLOSED
	case rs.waitingExchange:
		return clientv1.Phase_PHASE_EXCHANGE
	case rs.waitingQueMen:
		return clientv1.Phase_PHASE_QUE_MEN
	case rs.claimWindowOpen:
		return clientv1.Phase_PHASE_CLAIM
	case rs.waitingTsumo:
		return clientv1.Phase_PHASE_TSUMO
	case rs.waitingDiscard:
		return clientv1.Phase_PHASE_DISCARD
	default:
		return clientv1.Phase_PHASE_UNSPECIFIED
	}
}

func (rs *RoundState) actingSeats() []int32 {
	if rs == nil {
		return nil
	}
	switch {
	case rs.waitingExchange:
		return rs.pendingActiveSeats(rs.exchangeSubmitted)
	case rs.waitingQueMen:
		return rs.pendingActiveSeats(rs.queSubmitted)
	case rs.claimWindowOpen:
		if seat := rs.claimSeat(); seat >= 0 {
			return []int32{seat.Proto()}
		}
	case rs.waitingTsumo || rs.waitingDiscard:
		if !rs.isHued(rs.turn) && !rs.isSurrendered(rs.turn) {
			return []int32{rs.turn.Proto()}
		}
	}
	return nil
}

func (rs *RoundState) pendingActiveSeats(done []bool) []int32 {
	out := make([]int32, 0, 4)
	for seat := 0; seat < 4; seat++ {
		s := Seat(seat)
		if (seat >= len(done) || !done[seat]) && (rs == nil || (!rs.isHued(s) && !rs.isSurrendered(s))) {
			out = append(out, Seat(seat).Proto())
		}
	}
	return out
}
