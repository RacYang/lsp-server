package room

import (
	"encoding/json"

	"racoo.cn/lsp/internal/mahjong/rules"
	_ "racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi"
	"racoo.cn/lsp/internal/mahjong/tile"
)

const (
	openingExchangeThree = "exchange_three"
	openingQueMen        = "que_men"
)

func testRuleState(missing []int32) rules.RuleState {
	return encodeTestSichuanRuleState(missing, nil)
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
