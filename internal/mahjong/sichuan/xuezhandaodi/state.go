package xuezhandaodi

import (
	"encoding/json"

	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

type ruleState struct {
	MissingSuits []int32                  `json:"missing_suits,omitempty"`
	Submitted    map[string][]bool        `json:"submitted,omitempty"`
	Selections   map[string][][]tile.Tile `json:"selections,omitempty"`
	Direction    map[string]int32         `json:"direction,omitempty"`
}

func (x *rule) InitialRuleState() rules.RuleState {
	return encodeRuleState(initialRuleState(x.withExchange))
}

func (x *rule) NormalizeRuleState(state rules.RuleState) rules.RuleState {
	current := decodeRuleState(state)
	normalizeRuleState(&current, x.withExchange)
	return encodeRuleState(current)
}

func (x *rule) ProjectRuleState(state rules.RuleState) rules.RuleStateProjection {
	current := decodeRuleState(state)
	normalizeRuleState(&current, x.withExchange)
	done := map[string][]bool{
		openingProtocolMissing: append([]bool(nil), current.Submitted[openingStepMissing]...),
	}
	if x.withExchange {
		done[openingProtocolExchange] = append([]bool(nil), current.Submitted[openingStepExchange]...)
	}
	return rules.RuleStateProjection{
		SeatInts:                 map[string][]int32{"que_suit": append([]int32(nil), current.MissingSuits...)},
		OpeningSubmittedByAction: done,
	}
}

func initialRuleState(withExchange bool) ruleState {
	state := ruleState{
		MissingSuits: make([]int32, 4),
		Submitted:    map[string][]bool{},
		Selections:   map[string][][]tile.Tile{},
		Direction:    map[string]int32{},
	}
	state.Direction[openingStepExchange] = -1
	if withExchange {
		state.Submitted[openingStepExchange] = make([]bool, 4)
		state.Selections[openingStepExchange] = make([][]tile.Tile, 4)
	}
	state.Submitted[openingStepMissing] = make([]bool, 4)
	return state
}

func decodeRuleState(state rules.RuleState) ruleState {
	var out ruleState
	_ = json.Unmarshal(state.Data, &out)
	ensureRuleStateMaps(&out)
	return out
}

func encodeRuleState(state ruleState) rules.RuleState {
	ensureRuleStateMaps(&state)
	data, err := json.Marshal(state)
	if err != nil {
		panic(err)
	}
	return rules.RuleState{Data: data}
}

func normalizeRuleState(state *ruleState, withExchange bool) {
	if state == nil {
		return
	}
	ensureRuleStateMaps(state)
	for len(state.MissingSuits) < 4 {
		state.MissingSuits = append(state.MissingSuits, 0)
	}
	if withExchange {
		ensureBoolStep(state.Submitted, openingStepExchange)
		ensureTileStep(state.Selections, openingStepExchange)
	}
	ensureBoolStep(state.Submitted, openingStepMissing)
	if _, ok := state.Direction[openingStepExchange]; !ok {
		state.Direction[openingStepExchange] = -1
	}
}

func ensureRuleStateMaps(state *ruleState) {
	if state == nil {
		return
	}
	for len(state.MissingSuits) < 4 {
		state.MissingSuits = append(state.MissingSuits, 0)
	}
	if state.Submitted == nil {
		state.Submitted = map[string][]bool{}
	}
	if state.Selections == nil {
		state.Selections = map[string][][]tile.Tile{}
	}
	if state.Direction == nil {
		state.Direction = map[string]int32{}
	}
}
