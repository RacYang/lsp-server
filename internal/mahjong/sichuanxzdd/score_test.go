package sichuanxzdd

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
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
	if !hasFanKind(b, fan.KindLongQiDui) {
		t.Fatalf("expected long qi dui, got %+v total=%d", b.Items, b.Total)
	}
}

func TestScoreFansHeavenlyAndEarthlyHand(t *testing.T) {
	var x xzdd
	res := rules.HuResult{Win: countsFromTileTexts(t, []string{
		"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "p1", "p1", "p1", "p2", "p2",
	})}
	heavenly := x.ScoreFans(res, rules.ScoreContext{HuSeat: 0, DealerSeat: 0, IsTsumo: true, IsOpeningDraw: true})
	if !hasFanKind(heavenly, fan.KindHeavenlyHand) || hasFanKind(heavenly, fan.KindPingHu) {
		t.Fatalf("unexpected heavenly hand breakdown: %+v", heavenly.Items)
	}
	earthly := x.ScoreFans(res, rules.ScoreContext{HuSeat: 1, DealerSeat: 0, IsDealerFirstDiscard: true})
	if !hasFanKind(earthly, fan.KindEarthlyHand) || hasFanKind(earthly, fan.KindPingHu) {
		t.Fatalf("unexpected earthly hand breakdown: %+v", earthly.Items)
	}
	notEarthly := x.ScoreFans(res, rules.ScoreContext{HuSeat: 1, DealerSeat: 0, IsTsumo: true, IsOpeningDraw: true})
	if hasFanKind(notEarthly, fan.KindEarthlyHand) {
		t.Fatalf("tsumo should not be earthly hand: %+v", notEarthly.Items)
	}
}

func TestScoreFansShiBaLuoHanFiltersWinnerSeat(t *testing.T) {
	var x xzdd
	res := rules.HuResult{Win: countsFromTileTexts(t, []string{
		"m1", "m1", "m1", "m2", "m2", "m2", "m3", "m3", "m3", "p4", "p4", "p4", "s5", "s5",
	})}
	records := []rules.GangRecord{
		{Seat: 2, Kind: rules.GangKindMing},
		{Seat: 2, Kind: rules.GangKindAn},
		{Seat: 2, Kind: rules.GangKindBu},
		{Seat: 2, Kind: rules.GangKindMing},
	}
	b := x.ScoreFans(res, rules.ScoreContext{HuSeat: 2, GangRecords: records})
	if !hasFanKind(b, fan.KindShiBaLuoHan) || hasFanKind(b, fan.KindAnGang) {
		t.Fatalf("expected shi ba luo han without an gang stacking, got %+v", b.Items)
	}
	other := x.ScoreFans(res, rules.ScoreContext{HuSeat: 1, GangRecords: records})
	if hasFanKind(other, fan.KindShiBaLuoHan) {
		t.Fatalf("other player's gangs should not count: %+v", other.Items)
	}
}

func hasFanKind(b fan.Breakdown, kind fan.Kind) bool {
	for _, item := range b.Items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func countsFromTileTexts(t *testing.T, texts []string) hu.Counts {
	t.Helper()
	var c hu.Counts
	for _, text := range texts {
		ti, err := tile.Parse(text)
		if err != nil {
			t.Fatalf("parse tile %s: %v", text, err)
		}
		c[ti.Index()]++
	}
	return c
}
