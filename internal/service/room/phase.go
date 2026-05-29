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
	"racoo.cn/lsp/internal/clock"
)

// WaitingReason 表示房间当前等待的玩家动作类型。值域与 client.v1.WaitingReason 保持同语义；
// ReasonNone 表示无等待（如 settling 与 closed），此时 Deadline() 返回 0。
type WaitingReason int

const (
	ReasonNone WaitingReason = iota
	ReasonOpening
	ReasonClaimWindow
	ReasonTsumo
	ReasonDiscard
	// ReasonSurrender 是预留枚举值；实际"托管者已掉线"路径走 ReasonXxx + surrender=true 缩短时长。
	ReasonSurrender
)

// String 用于结构化日志输出。
func (r WaitingReason) String() string {
	switch r {
	case ReasonOpening:
		return "opening"
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
	case ReasonOpening:
		return clientv1.WaitingReason_WAITING_REASON_OPENING
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
	case clientv1.WaitingReason_WAITING_REASON_OPENING:
		return ReasonOpening
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
	case ReasonOpening:
		return cfg.OpeningDefault
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

func (cfg TimeoutConfig) DurationForOpeningAction(action string, surrender bool) time.Duration {
	if surrender {
		return cfg.SurrenderAction
	}
	if dur := cfg.OpeningByAction[action]; dur > 0 {
		return dur
	}
	return cfg.OpeningDefault
}

// enterPhase 是 RoundState 阶段切换的唯一写入入口。
//
// 不变量：
//  1. 该方法是 phaseReason / phaseStartUnixMs / opening wait flag /
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
	rs.waitingOpening = false
	rs.claimWindowOpen = false
	rs.waitingTsumo = false
	rs.waitingDiscard = false
	switch reason {
	case ReasonOpening:
		rs.waitingOpening = true
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
	if dur := rs.phaseDuration(reason, surrender); dur > 0 {
		rs.deadlineUnixMs = rs.phaseStartUnixMs + dur.Milliseconds()
	} else {
		rs.deadlineUnixMs = 0
	}
}

func (rs *RoundState) phaseDuration(reason WaitingReason, surrender bool) time.Duration {
	if rs == nil {
		return 0
	}
	if reason == ReasonOpening {
		action := ""
		if step, ok := rs.currentOpeningStep(); ok {
			action = step.Action
		}
		return rs.tmo.DurationForOpeningAction(action, surrender)
	}
	return rs.tmo.DurationFor(reason, surrender)
}

// surrenderedWaitingSeat 判断 reason 当前等待的目标座位是否处于托管/掉线状态。
// 与 scheduler.surrenderedSeatWaiting 同源。
func (rs *RoundState) surrenderedWaitingSeat(reason WaitingReason) bool {
	if rs == nil {
		return false
	}
	switch reason {
	case ReasonOpening:
		step, ok := rs.currentOpeningStep()
		if !ok {
			return false
		}
		for seat, done := range rs.openingSubmitted(step.Action) {
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

// InjectRecoveryRuntime 是节点重启恢复路径的唯一 clk/tmo 注入入口。
//
// 注入完成后按如下规则重锚定 deadline（与 enterPhase 语义对齐）：
//   - phaseReason == ReasonNone：不设 deadline，scheduler 停表。
//   - phaseStartUnixMs == 0（老快照无锚点）：以当前时刻为起点重新计算。
//   - phaseStartUnixMs > 0（新快照有锚点）：以快照时刻加 duration 计算；
//     若 deadline 已过期，scheduler.armUntil 会立即触发 cmdAutoTimeout，
//     与 ADR-0045 "重启后超时立即托管"约束一致。
//
// 调用方仅为 service.startActorLocked 的恢复分支；禁止在其他路径直接写
// rs.clk / rs.tmo / rs.phaseStartUnixMs / rs.deadlineUnixMs。
func (rs *RoundState) InjectRecoveryRuntime(clk clock.Clock, tmo TimeoutConfig) {
	if rs == nil {
		return
	}
	rs.clk = clk
	rs.tmo = tmo
	if rs.phaseReason == ReasonNone {
		return
	}
	if rs.phaseStartUnixMs == 0 {
		rs.phaseStartUnixMs = clk.Now().UnixMilli()
	}
	surrender := rs.surrenderedWaitingSeat(rs.phaseReason)
	if dur := rs.phaseDuration(rs.phaseReason, surrender); dur > 0 {
		rs.deadlineUnixMs = rs.phaseStartUnixMs + dur.Milliseconds()
	} else {
		rs.deadlineUnixMs = 0
	}
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
	if e.Progress.Step != 0 || e.Progress.Phase != PhaseUnspecified || e.Progress.Reason != ReasonNone {
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

func (rs *RoundState) phase() Phase {
	if rs == nil {
		return PhaseUnspecified
	}
	switch {
	case rs.closed:
		return PhaseClosed
	case rs.waitingOpening:
		return rs.openingPhase()
	case rs.claimWindowOpen:
		return PhaseClaim
	case rs.waitingTsumo:
		return PhaseTsumo
	case rs.waitingDiscard:
		return PhaseDiscard
	default:
		return PhaseUnspecified
	}
}

func (rs *RoundState) actingSeats() []int32 {
	if rs == nil {
		return nil
	}
	switch {
	case rs.waitingOpening:
		if step, ok := rs.currentOpeningStep(); ok {
			return rs.pendingActiveSeats(rs.openingSubmitted(step.Action))
		}
	case rs.claimWindowOpen:
		if seat := rs.claimSeat(); seat >= 0 {
			return []int32{seat.Proto()}
		}
	case rs.waitingTsumo || rs.waitingDiscard:
		if !rs.isSeatOutAfterHu(rs.turn) && !rs.isSurrendered(rs.turn) {
			return []int32{rs.turn.Proto()}
		}
	}
	return nil
}

func (rs *RoundState) openingPhase() Phase {
	return PhaseOpening
}

func (rs *RoundState) pendingActiveSeats(done []bool) []int32 {
	out := make([]int32, 0, 4)
	for seat := 0; seat < 4; seat++ {
		s := Seat(seat)
		if (seat >= len(done) || !done[seat]) && (rs == nil || (!rs.isSeatOutAfterHu(s) && !rs.isSurrendered(s))) {
			out = append(out, Seat(seat).Proto())
		}
	}
	return out
}
