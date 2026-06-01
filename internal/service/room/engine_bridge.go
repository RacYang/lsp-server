// engine_bridge.go — Phase B 过渡期桥接文件。
//
// 把 internal/service/room/engine 的导出类型重新暴露为 service/room 包成员，
// 使现有外部调用方（adapter、handler、app）在 Phase B 期间无需修改 import 路径。
//
// TODO(debt): Phase C 完成后，外部调用方改为直接 import service/room/engine，
// 届时删除本文件。回收条件：adapter/room、adapter/local、handler 的 import 全部
// 迁至 engine 包，且 service/room 包内部文件已改用 eng. 前缀。
package room

import (
	act "racoo.cn/lsp/internal/service/room/actor"
	eng "racoo.cn/lsp/internal/service/room/engine"
)

// ── 基础类型 ──────────────────────────────────────────────────────────────────

type (
	Seat = eng.Seat

	Kind         = eng.Kind
	Notification = eng.Notification

	Engine        = eng.Engine
	RoundState    = eng.RoundState
	TimeoutConfig = eng.TimeoutConfig

	WaitingReason   = eng.WaitingReason
	PhaseToken      = eng.PhaseToken
	PhaseDriftError = eng.PhaseDriftError
	PhaseTransition = eng.PhaseTransition

	Phase = eng.Phase

	RuleMeta       = eng.RuleMeta
	LastActionInfo = eng.LastActionInfo
	MeldInfo       = eng.MeldInfo
	SeatMelds      = eng.SeatMelds

	RoundView           = eng.RoundView
	RoundProgress       = eng.RoundProgress
	RoundFacts          = eng.RoundFacts
	RoundProjection     = eng.RoundProjection
	RoundClaimCandidate = eng.RoundClaimCandidate
)

// ── Kind 常量 ─────────────────────────────────────────────────────────────────

const (
	KindInitialDeal = eng.KindInitialDeal
	KindOpeningDone = eng.KindOpeningDone
	KindStartGame   = eng.KindStartGame
	KindDrawTile    = eng.KindDrawTile
	KindAction      = eng.KindAction
	KindSettlement  = eng.KindSettlement
)

// ── WaitingReason 常量 ────────────────────────────────────────────────────────

const (
	ReasonNone        = eng.ReasonNone
	ReasonOpening     = eng.ReasonOpening
	ReasonClaimWindow = eng.ReasonClaimWindow
	ReasonTsumo       = eng.ReasonTsumo
	ReasonDiscard     = eng.ReasonDiscard
	ReasonSurrender   = eng.ReasonSurrender
)

// ── Phase 常量 ────────────────────────────────────────────────────────────────

const (
	PhaseUnspecified = eng.PhaseUnspecified
	PhaseOpening     = eng.PhaseOpening
	PhaseClaim       = eng.PhaseClaim
	PhaseTsumo       = eng.PhaseTsumo
	PhaseDiscard     = eng.PhaseDiscard
	PhaseDraw        = eng.PhaseDraw
	PhaseClosed      = eng.PhaseClosed
)

// ── Seat 常量（来自 engine/seat.go）──────────────────────────────────────────

const (
	SeatInvalid   = eng.SeatInvalid
	BroadcastSeat = eng.BroadcastSeat
	SeatCount     = eng.SeatCount
)

// ── 错误变量 ──────────────────────────────────────────────────────────────────

var ErrRoundPersistUnsupportedSchema = eng.ErrRoundPersistUnsupportedSchema

var (
	ErrRoomFull    = act.ErrRoomFull
	ErrRateLimited = act.ErrRateLimited
)

// ── 构造函数与工具函数 ─────────────────────────────────────────────────────────

// ── 测试专用工厂（禁止生产代码引用）─────────────────────────────────────────────

type RoundStateConfig = eng.RoundStateConfig

var NewRoundStateFromConfig = eng.NewRoundStateFromConfig

// ── 构造函数与工具函数 ─────────────────────────────────────────────────────────

var (
	NewEngine                   = eng.NewEngine
	DefaultTimeoutConfig        = eng.DefaultTimeoutConfig
	PhaseTokenFromProto         = eng.PhaseTokenFromProto
	WaitingReasonFromProto      = eng.WaitingReasonFromProto
	RoundViewFromPersistJSON    = eng.RoundViewFromPersistJSON
	RestoreRoundFromPersistJSON = eng.RestoreRoundFromPersistJSON
	SeatFromInt                 = eng.SeatFromInt
)
