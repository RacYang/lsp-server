package sichuanxzdd

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestScoreFansQingYiSeWithSevenPairs(t *testing.T) {
	var x xzdd
	h := hand.New()
	// 清一色万子七对：m1m1 m3m3 m5m5 m7m7 m9m9 m2m2 m4m4（13 张）+ m4 进张成对
	seq := []string{
		"m1", "m1", "m2", "m2", "m3", "m3", "m4", "m4", "m5", "m5", "m6", "m6", "m7",
	}
	for _, s := range seq {
		ti, _ := tile.Parse(s)
		h.Add(ti)
	}
	win, _ := tile.Parse("m7")
	res, ok := x.CheckHu(h, win, rules.HuContext{})
	if !ok {
		t.Fatal("expected win")
	}
	b := x.ScoreFans(res, rules.ScoreContext{})
	if b.Total < 8 {
		t.Fatalf("expected qi dui + qing yise, got %+v total=%d", b.Items, b.Total)
	}
}

func TestScoreFansDuiDuiHu(t *testing.T) {
	var x xzdd
	h := hand.New()
	// 13 张：111 222 333 444 5，胡 5 成对对胡。
	for _, s := range []string{
		"m1", "m1", "m1",
		"m2", "m2", "m2",
		"m3", "m3", "m3",
		"p4", "p4", "p4",
		"s5",
	} {
		ti, _ := tile.Parse(s)
		h.Add(ti)
	}
	win, _ := tile.Parse("s5")
	res, ok := x.CheckHu(h, win, rules.HuContext{})
	if !ok {
		t.Fatal("expected win")
	}
	b := x.ScoreFans(res, rules.ScoreContext{})
	if b.Total < 2 {
		t.Fatalf("expected dui dui hu fan, got %+v total=%d", b.Items, b.Total)
	}
}

func TestScoreFansQiDuiWithGen(t *testing.T) {
	var x xzdd
	h := hand.New()
	for _, s := range []string{
		"m1", "m1", "m1", "m1",
		"m2", "m2",
		"m3", "m3",
		"p4", "p4",
		"p5", "p5",
		"s6",
	} {
		ti, _ := tile.Parse(s)
		h.Add(ti)
	}
	win, _ := tile.Parse("s6")
	res, ok := x.CheckHu(h, win, rules.HuContext{})
	if !ok {
		t.Fatal("expected win")
	}
	b := x.ScoreFans(res, rules.ScoreContext{})
	if b.Total < 5 {
		t.Fatalf("expected qi dui plus gen, got %+v total=%d", b.Items, b.Total)
	}
}
