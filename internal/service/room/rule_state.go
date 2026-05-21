package room

import (
	"racoo.cn/lsp/internal/mahjong/rules"
)

const ruleProjectionKeyQueSuit = "que_suit"

func newInitialRuleState(caps rules.CapabilitySet) rules.RuleState {
	return caps.State.InitialRuleState()
}

func (rs *RoundState) normalizeRuleState() {
	if rs == nil {
		return
	}
	rs.ruleState = rs.caps.State.NormalizeRuleState(rs.ruleState)
}

func (rs *RoundState) ruleStateProjection() rules.RuleStateProjection {
	if rs == nil {
		return rules.RuleStateProjection{}
	}
	rs.normalizeRuleState()
	return rs.caps.StateView.ProjectRuleState(rs.ruleState)
}

func (rs *RoundState) missingSuitBySeat() []int32 {
	if rs == nil {
		return nil
	}
	return append([]int32(nil), rs.ruleStateProjection().SeatInts[ruleProjectionKeyQueSuit]...)
}

func (rs *RoundState) openingSubmitted(action string) []bool {
	if rs == nil {
		return nil
	}
	return append([]bool(nil), rs.ruleStateProjection().OpeningSubmittedByAction[action]...)
}

func (rs *RoundState) openingSubmittedByAction() map[string][]bool {
	if rs == nil {
		return nil
	}
	src := rs.ruleStateProjection().OpeningSubmittedByAction
	out := make(map[string][]bool, len(src))
	for action, submitted := range src {
		out[action] = append([]bool(nil), submitted...)
	}
	return out
}

func (rs *RoundState) winnerSeats() []Seat {
	if rs == nil {
		return nil
	}
	out := make([]Seat, 0, len(rs.winEvents))
	seen := map[Seat]struct{}{}
	for _, event := range rs.winEvents {
		seat := Seat(event.Seat)
		if _, ok := seen[seat]; ok {
			continue
		}
		seen[seat] = struct{}{}
		out = append(out, seat)
	}
	return out
}

func (rs *RoundState) huedSeats() []bool {
	out := make([]bool, 4)
	if rs == nil {
		return out
	}
	for _, seat := range rs.winnerSeats() {
		if seat >= 0 && seat < 4 {
			out[seat] = true
		}
	}
	return out
}
