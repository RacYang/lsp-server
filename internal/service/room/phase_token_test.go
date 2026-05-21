// Package room: ADR-0045 P7 单测 —— 校验 PhaseToken drift 行为的最小契约。
//
// 该文件不依赖完整开局流程，直接构造 RoundState（仅设置 step + phaseReason）
// 以覆盖三类典型场景：
//  1. tok == nil：保持向后兼容，validate 应返回 nil；
//  2. tok 与服务端权威 (step, reason) 完全一致：返回 nil；
//  3. tok 过时（step/reason 任一不一致）：返回 *PhaseDriftError，Current 字段反映服务端权威。
package room

import (
	"errors"
	"testing"

	"racoo.cn/lsp/internal/mahjong/rules"
	_ "racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi"
)

func TestRoundState_validatePhaseToken(t *testing.T) {
	rule := rules.MustGet("sichuan_xuezhandaodi_huansanzhang")
	rs := &RoundState{
		step:        7,
		phaseReason: ReasonDiscard,
		rule:        rule,
		ruleID:      rule.ID(),
		caps:        rules.CapabilitiesOf(rule),
	}

	if err := rs.validatePhaseToken(nil); err != nil {
		t.Fatalf("nil token must be accepted for backward compatibility, got %v", err)
	}

	matching := &PhaseToken{Step: 7, Reason: ReasonDiscard}
	if err := rs.validatePhaseToken(matching); err != nil {
		t.Fatalf("matching token should be accepted, got %v", err)
	}

	staleStep := &PhaseToken{Step: 6, Reason: ReasonDiscard}
	var drift *PhaseDriftError
	if err := rs.validatePhaseToken(staleStep); !errors.As(err, &drift) {
		t.Fatalf("stale step should produce PhaseDriftError, got %v", err)
	} else if drift.Current.Step != 7 || drift.Current.Reason != ReasonDiscard {
		t.Fatalf("drift error must carry authoritative current phase, got %+v", drift.Current)
	}

	staleReason := &PhaseToken{Step: 7, Reason: ReasonTsumo}
	drift = nil
	if err := rs.validatePhaseToken(staleReason); !errors.As(err, &drift) {
		t.Fatalf("stale reason should produce PhaseDriftError, got %v", err)
	}
}

func TestPhaseDriftError_ImplementsError(t *testing.T) {
	e := &PhaseDriftError{Token: PhaseToken{Step: 1, Reason: ReasonDiscard}, Current: PhaseToken{Step: 2, Reason: ReasonTsumo}}
	if e.Error() == "" {
		t.Fatal("PhaseDriftError.Error must return non-empty diagnostic")
	}
}
