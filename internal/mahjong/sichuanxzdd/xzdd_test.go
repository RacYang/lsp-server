package sichuanxzdd

import (
	"context"
	"testing"

	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestBuildWallDeterministic(t *testing.T) {
	var x xzdd
	w1 := x.BuildWall(context.Background(), 7)
	w2 := x.BuildWall(context.Background(), 7)
	if w1.Tiles()[0] != w2.Tiles()[0] {
		t.Fatalf("wall mismatch")
	}
}

func TestGameOverConditions(t *testing.T) {
	var x xzdd
	if !x.GameOver(rules.GameState{HuedPlayers: 3, WallRemaining: 10}) {
		t.Fatal("expected game over on 3 hu")
	}
	if !x.GameOver(rules.GameState{HuedPlayers: 0, WallRemaining: 0}) {
		t.Fatal("expected game over on empty wall")
	}
	if x.GameOver(rules.GameState{HuedPlayers: 2, WallRemaining: 5}) {
		t.Fatal("expected continue")
	}
}

func TestCapabilitiesExposeComposableFeatures(t *testing.T) {
	var x xzdd
	caps := x.Capabilities()
	if caps.Claims == nil {
		t.Fatal("expected claim policy")
	}
	if got := caps.Opening.Steps(); len(got) != 2 || got[0] != "exchange_three" || got[1] != "que_men" {
		t.Fatalf("unexpected opening flow: %v", got)
	}
	if caps.Metadata.DisplayName != "四川血战到底" {
		t.Fatalf("unexpected display name: %s", caps.Metadata.DisplayName)
	}
}

func TestCheckHuSevenPairsScoring(t *testing.T) {
	var x xzdd
	h := hand.New()
	// 七对：7 个不同对子
	pairs := []string{"m1", "m1", "m3", "m3", "m5", "m5", "m7", "m7", "p2", "p2", "p4", "p4", "s6"}
	for _, s := range pairs {
		ti, _ := tile.Parse(s)
		h.Add(ti)
	}
	win, _ := tile.Parse("s6")
	res, ok := x.CheckHu(h, win, rules.HuContext{})
	if !ok {
		t.Fatal("expected win")
	}
	b := x.ScoreFans(res, rules.ScoreContext{})
	if b.Total < 4 {
		t.Fatalf("expected qi dui fan, got %+v", b)
	}
}
