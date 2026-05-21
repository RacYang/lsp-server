package rules_test

import (
	"testing"

	_ "racoo.cn/lsp/internal/mahjong/guobiao/jingji"
	"racoo.cn/lsp/internal/mahjong/rules"
	_ "racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi"
)

type capabilityProvider interface {
	Capabilities() rules.CapabilitySet
}

func TestProductionRulesDeclareExplicitRuntimeStrategies(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"guobiao_jingji_biaozhun",
		"sichuan_xuezhandaodi_huansanzhang",
		"sichuan_xuezhandaodi_biaozhun",
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			rule := rules.MustGet(id)
			provider, ok := rule.(capabilityProvider)
			if !ok {
				t.Fatalf("%s must expose explicit CapabilitySet", id)
			}
			caps := provider.Capabilities()
			if caps.TileSet == nil {
				t.Fatalf("%s must declare TileSetPolicy", id)
			}
			if caps.Claims == nil {
				t.Fatalf("%s must declare ClaimPolicy", id)
			}
			if caps.SelfActions == nil {
				t.Fatalf("%s must declare SelfActionPolicy", id)
			}
			if caps.Win == nil {
				t.Fatalf("%s must declare WinPolicy", id)
			}
			if caps.State == nil {
				t.Fatalf("%s must declare RuleStateCodec", id)
			}
			if caps.StateView == nil {
				t.Fatalf("%s must declare RuleStateProjector", id)
			}
			if caps.Turn == nil {
				t.Fatalf("%s must declare TurnFlow", id)
			}
			if caps.Scoring == nil {
				t.Fatalf("%s must declare ScoringPolicy", id)
			}
			if caps.Settlement == nil {
				t.Fatalf("%s must declare SettlementPolicy", id)
			}
			if caps.Termination == nil {
				t.Fatalf("%s must declare TerminationPolicy", id)
			}
			if caps.Projection == nil {
				t.Fatalf("%s must declare RoundProjection", id)
			}
		})
	}
}
