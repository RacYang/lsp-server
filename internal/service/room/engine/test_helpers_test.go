package engine

import (
	"encoding/json"

	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/rules"
	_ "racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// containsSettlement 是引擎测试用辅助函数，供 conformance_test.go 使用。
func containsSettlement(notifications []Notification) bool {
	for _, notification := range notifications {
		if notification.Kind == KindSettlement {
			return true
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(notification.Payload, &env); err == nil && env.GetSettlement() != nil {
			return true
		}
	}
	return false
}

// ── 四川规则状态辅助函数（供 engine_score_test.go 使用）─────────────────────────

const (
	openingExchangeThree = "exchange_three"
	openingQueMen        = "que_men"
)

func testRuleState(missing []int32) rules.RuleState {
	return encodeTestSichuanRuleState(missing, nil)
}

func testRuleStateWithSubmitted(missing []int32, step string, submitted []bool) rules.RuleState {
	stateStep := testSichuanStateStep(step)
	done := map[string][]bool{stateStep: append([]bool(nil), submitted...)}
	if step == openingQueMen {
		done[testSichuanStateStep(openingExchangeThree)] = []bool{true, true, true, true}
	}
	return encodeTestSichuanRuleState(missing, done)
}

func testSichuanStateStep(protocolAction string) string {
	switch protocolAction {
	case openingExchangeThree:
		return "sichuan.exchange"
	case openingQueMen:
		return "sichuan.missing_suit"
	default:
		return protocolAction
	}
}

func testSichuanExchangeDirection(state rules.RuleState) int32 {
	var decoded struct {
		Direction map[string]int32 `json:"direction,omitempty"`
	}
	_ = json.Unmarshal(state.Data, &decoded)
	if decoded.Direction == nil {
		return -1
	}
	if direction, ok := decoded.Direction[testSichuanStateStep(openingExchangeThree)]; ok {
		return direction
	}
	return -1
}

func encodeTestSichuanRuleState(missing []int32, submitted map[string][]bool) rules.RuleState {
	state := struct {
		MissingSuits []int32                  `json:"missing_suits,omitempty"`
		Submitted    map[string][]bool        `json:"submitted,omitempty"`
		Selections   map[string][][]tile.Tile `json:"selections,omitempty"`
		Direction    map[string]int32         `json:"direction,omitempty"`
	}{
		MissingSuits: append([]int32(nil), missing...),
		Submitted:    submitted,
		Selections:   map[string][][]tile.Tile{},
		Direction:    map[string]int32{testSichuanStateStep(openingExchangeThree): -1},
	}
	data, err := json.Marshal(state)
	if err != nil {
		panic(err)
	}
	return rules.RuleState{Data: data}
}
