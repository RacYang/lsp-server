package xuezhandaodi

import (
	"context"
	"encoding/json"
	"testing"

	"racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

func testBuildWall(x *rule, seed int64) *wall.Wall {
	return x.Capabilities().TileSet.BuildWall(context.Background(), seed)
}

func testCheckHu(x *rule, h *hand.Hand, target tile.Tile, hc rules.HuContext) (rules.HuResult, bool) {
	return x.Capabilities().Win.CheckHu(h, target, hc)
}

func testScoreWin(x *rule, result rules.HuResult, sc rules.ScoreContext) fan.Breakdown {
	breakdown, _, _ := x.Capabilities().Scoring.ScoreWin(result, sc)
	return breakdown
}

func testGameOver(x *rule, huedPlayers int, wallRemaining int) bool {
	events := make([]rules.WinEvent, huedPlayers)
	for i := range events {
		events[i] = rules.WinEvent{Seat: room.SeatFromInt(i)}
	}
	return x.Capabilities().Termination.GameOver(rules.TerminationContext{
		WinEvents:     events,
		WallRemaining: wallRemaining,
	})
}

func TestBuildWallDeterministic(t *testing.T) {
	x := newRule(IDHuansanzhang, true)
	w1 := testBuildWall(x, 7)
	w2 := testBuildWall(x, 7)
	if w1.Tiles()[0] != w2.Tiles()[0] {
		t.Fatalf("wall mismatch")
	}
}

func TestGameOverConditions(t *testing.T) {
	x := newRule(IDHuansanzhang, true)
	if !testGameOver(x, 3, 10) {
		t.Fatal("expected game over on 3 hu")
	}
	if !testGameOver(x, 0, 0) {
		t.Fatal("expected game over on empty wall")
	}
	if testGameOver(x, 2, 5) {
		t.Fatal("expected continue")
	}
}

func TestCapabilitiesExposeComposableFeatures(t *testing.T) {
	x := newRule(IDHuansanzhang, true)
	caps := x.Capabilities()
	if caps.Claims == nil {
		t.Fatal("expected claim policy")
	}
	if got := caps.Opening.Steps(); len(got) != 2 || got[0] != "exchange_three" || got[1] != "que_men" {
		t.Fatalf("unexpected opening flow: %v", got)
	}
	if caps.Metadata.DisplayName != "四川血战到底（换三张）" {
		t.Fatalf("unexpected display name: %s", caps.Metadata.DisplayName)
	}
}

func TestRuleStateUsesInternalStepKeysAndProjectsProtocolActions(t *testing.T) {
	x := newRule(IDHuansanzhang, true)
	state := x.InitialRuleState()
	var raw ruleState
	if err := json.Unmarshal(state.Data, &raw); err != nil {
		t.Fatalf("unmarshal rule state: %v", err)
	}
	if _, ok := raw.Submitted[openingProtocolExchange]; ok {
		t.Fatalf("rule state must not store protocol action key %q", openingProtocolExchange)
	}
	if _, ok := raw.Submitted[openingProtocolMissing]; ok {
		t.Fatalf("rule state must not store protocol action key %q", openingProtocolMissing)
	}
	if _, ok := raw.Direction[openingProtocolExchange]; ok {
		t.Fatalf("rule state must not store protocol direction key %q", openingProtocolExchange)
	}
	if _, ok := raw.Submitted[openingStepExchange]; !ok {
		t.Fatalf("rule state must store internal step key %q", openingStepExchange)
	}
	if _, ok := raw.Submitted[openingStepMissing]; !ok {
		t.Fatalf("rule state must store internal step key %q", openingStepMissing)
	}
	projection := x.ProjectRuleState(state)
	if _, ok := projection.OpeningSubmittedByAction[openingProtocolExchange]; !ok {
		t.Fatalf("projection must expose protocol action key %q", openingProtocolExchange)
	}
	if _, ok := projection.OpeningSubmittedByAction[openingProtocolMissing]; !ok {
		t.Fatalf("projection must expose protocol action key %q", openingProtocolMissing)
	}
	if _, ok := projection.SeatInts["que_suit"]; !ok {
		t.Fatalf("projection must expose missing suit seat-int key")
	}
}

func TestBiaozhunCapabilitiesSkipExchangeThree(t *testing.T) {
	x := newRule(IDBiaozhun, false)
	caps := x.Capabilities()
	if got := caps.Opening.Steps(); len(got) != 1 || got[0] != "que_men" {
		t.Fatalf("unexpected opening flow: %v", got)
	}
	for _, feature := range caps.Metadata.EnabledFeatures {
		if feature == "exchange_three" {
			t.Fatalf("biaozhun should not expose exchange_three: %v", caps.Metadata.EnabledFeatures)
		}
	}
}

func TestCheckHuSevenPairsScoring(t *testing.T) {
	x := newRule(IDHuansanzhang, true)
	h := hand.New()
	// 七对：7 个不同对子
	pairs := []string{"m1", "m1", "m3", "m3", "m5", "m5", "m7", "m7", "p2", "p2", "p4", "p4", "s6"}
	for _, s := range pairs {
		ti, _ := tile.Parse(s)
		h.Add(ti)
	}
	win, _ := tile.Parse("s6")
	res, ok := testCheckHu(x, h, win, rules.HuContext{})
	if !ok {
		t.Fatal("expected win")
	}
	b := testScoreWin(x, res, rules.ScoreContext{})
	if b.Total < 4 {
		t.Fatalf("expected qi dui fan, got %+v", b)
	}
}
