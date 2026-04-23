package sichuan_xzdd

import (
	"context"
	"testing"

	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestName(t *testing.T) {
	var x xzdd
	if x.Name() == "" {
		t.Fatal("empty name")
	}
}

func TestBuildWallSeedZero(t *testing.T) {
	var x xzdd
	w1 := x.BuildWall(context.Background(), 0)
	w2 := x.BuildWall(context.Background(), 0)
	if len(w1.Tiles()) != len(w2.Tiles()) {
		t.Fatal("wall size mismatch")
	}
}

func TestCheckHuNilHand(t *testing.T) {
	var x xzdd
	ti := tile.Must(tile.SuitDots, 1)
	if _, ok := x.CheckHu(nil, ti, rules.HuContext{}); ok {
		t.Fatal("expected false")
	}
}

func TestCheckHuFalse(t *testing.T) {
	var x xzdd
	h := hand.New()
	for _, s := range []string{"m1", "m9", "p1", "p9", "s1", "s9"} {
		ti, _ := tile.Parse(s)
		h.Add(ti)
	}
	ti := tile.Must(tile.SuitDots, 5)
	if _, ok := x.CheckHu(h, ti, rules.HuContext{}); ok {
		t.Fatal("expected false")
	}
}

func TestScoreFansPingHuOnly(t *testing.T) {
	var x xzdd
	h := hand.New()
	for _, s := range []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "p1", "p1", "p1", "p2"} {
		ti, _ := tile.Parse(s)
		h.Add(ti)
	}
	win, _ := tile.Parse("p2")
	res, ok := x.CheckHu(h, win, rules.HuContext{})
	if !ok {
		t.Fatal("expected win")
	}
	b := x.ScoreFans(res, rules.ScoreContext{})
	if b.Total < 1 {
		t.Fatalf("expected ping hu fan, got %+v", b)
	}
}
