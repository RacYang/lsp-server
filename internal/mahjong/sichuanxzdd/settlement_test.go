package sichuanxzdd

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestBuildSettlementMultiWinnerAndPenalties(t *testing.T) {
	playerIDs := [4]string{"u0", "u1", "u2", "u3"}
	hands := []*hand.Hand{
		hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1)}),
		hand.New(),
		hand.New(),
		hand.New(),
	}
	queBySeat := []int32{int32(tile.SuitCharacters), int32(tile.SuitDots), int32(tile.SuitBamboo), int32(tile.SuitDots)}
	ledger := []ScoreEntry{
		{Reason: ReasonHuTsumo, FromSeat: 0, ToSeat: 1, Amount: 4, WinnerSeat: 1, WinnerFan: 4, FanNames: []string{"平胡"}},
		{Reason: ReasonHuDiscard, FromSeat: 3, ToSeat: 2, Amount: 2, WinnerSeat: 2, WinnerFan: 2, FanNames: []string{"对对胡"}},
	}

	scores, penalties, breakdowns, detail := BuildSettlement(playerIDs, hands, queBySeat, ledger, []int{1, 2})
	if len(scores) != 4 {
		t.Fatalf("scores len = %d", len(scores))
	}
	if len(breakdowns) != 2 {
		t.Fatalf("breakdowns len = %d", len(breakdowns))
	}
	if detail == "荒牌" {
		t.Fatal("winner detail should be populated")
	}
	var sawHuaZhu bool
	var sawChaDaJiao bool
	for _, p := range penalties {
		if p.GetReason() == "查花猪" {
			sawHuaZhu = true
		}
		if p.GetReason() == "查大叫" {
			sawChaDaJiao = true
		}
	}
	if !sawHuaZhu || !sawChaDaJiao {
		t.Fatalf("missing penalties: %+v", penalties)
	}
}

func TestBuildSettlementRefundsGangIncome(t *testing.T) {
	playerIDs := [4]string{"u0", "u1", "u2", "u3"}
	hands := []*hand.Hand{
		hand.New(),
		hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1)}),
		hand.New(),
		hand.New(),
	}
	queBySeat := []int32{int32(tile.SuitDots), int32(tile.SuitCharacters), int32(tile.SuitBamboo), int32(tile.SuitDots)}
	ledger := []ScoreEntry{{Reason: ReasonGangMing, FromSeat: 0, ToSeat: 1, Amount: 1, WinnerSeat: -1}}

	scores, penalties, _, _ := BuildSettlement(playerIDs, hands, queBySeat, ledger, nil)
	if scores[1].GetTotalFan() >= 1 {
		t.Fatalf("expected refund to reduce seat score, got %+v", scores[1])
	}
	var sawRefund bool
	for _, penalty := range penalties {
		if penalty.GetReason() == ReasonRefundMing {
			sawRefund = true
		}
	}
	if !sawRefund {
		t.Fatalf("missing refund penalty: %+v", penalties)
	}
}

func TestBuildSettlementWinnerBreakdownBaoPai(t *testing.T) {
	playerIDs := [4]string{"u0", "u1", "u2", "u3"}
	hands := []*hand.Hand{hand.New(), hand.New(), hand.New(), hand.New()}
	queBySeat := []int32{0, 1, 2, 0}
	ledger := []ScoreEntry{{
		Reason:     ReasonHuQiangGang,
		FromSeat:   0,
		ToSeat:     2,
		Amount:     3,
		WinnerSeat: 2,
		WinnerFan:  3,
		FanNames:   []string{"抢杠胡", ReasonBaoPai},
	}}

	_, _, breakdowns, _ := BuildSettlement(playerIDs, hands, queBySeat, ledger, []int{2})
	if len(breakdowns) != 1 {
		t.Fatalf("breakdowns len = %d", len(breakdowns))
	}
	if breakdowns[0].GetFan() != 3 {
		t.Fatalf("fan = %d", breakdowns[0].GetFan())
	}
	found := false
	for _, name := range breakdowns[0].GetFanNames() {
		if name == ReasonBaoPai {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing bao pai fan name: %+v", breakdowns[0].GetFanNames())
	}
}

func TestScoreFansContextualKinds(t *testing.T) {
	var x xzdd
	h := hand.FromTiles([]tile.Tile{
		tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1),
		tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2),
		tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3),
		tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4),
		tile.Must(tile.SuitCharacters, 5), tile.Must(tile.SuitCharacters, 5),
	})
	res, ok := x.CheckHu(h, tile.Must(tile.SuitCharacters, 5), rules.HuContext{})
	if !ok {
		t.Fatal("expected win")
	}
	b := x.ScoreFans(res, rules.ScoreContext{
		IsTsumo:        false,
		IsHaiDi:        true,
		IsGangShangHua: true,
		GangRecords: []rules.GangRecord{{
			Kind:            rules.GangKindBu,
			ResponsibleSeat: 1,
		}},
	})
	kinds := map[string]bool{}
	for _, item := range b.Items {
		kinds[string(item.Kind)] = true
	}
	for _, want := range []string{"qing_yi_se", "gang_shang_kai", "hai_di_pao", "qiang_gang_hu"} {
		if !kinds[want] {
			t.Fatalf("missing fan %s in %+v", want, b.Items)
		}
	}
}

func TestScoreFansDeepeningKinds(t *testing.T) {
	var x xzdd
	win := countsFromTiles([]tile.Tile{
		tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2),
		tile.Must(tile.SuitCharacters, 5), tile.Must(tile.SuitCharacters, 5), tile.Must(tile.SuitCharacters, 5),
		tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 2),
		tile.Must(tile.SuitDots, 5), tile.Must(tile.SuitDots, 5), tile.Must(tile.SuitDots, 5),
		tile.Must(tile.SuitBamboo, 8), tile.Must(tile.SuitBamboo, 8),
	})
	b := x.ScoreFans(rules.HuResult{Win: win}, rules.ScoreContext{
		IsGangShangPao: true,
		GangRecords: []rules.GangRecord{
			{Kind: rules.GangKindAn},
			{Kind: rules.GangKindAn},
		},
	})
	kinds := map[fan.Kind]bool{}
	for _, item := range b.Items {
		kinds[item.Kind] = true
	}
	for _, want := range []fan.Kind{fan.KindJiangDui, fan.KindAnKe, fan.KindAnGang, fan.KindShuangAnGang, fan.KindGangShangPao} {
		if !kinds[want] {
			t.Fatalf("missing fan %s in %+v", want, b.Items)
		}
	}
}

func countsFromTiles(ts []tile.Tile) hu.Counts {
	var c hu.Counts
	for _, t := range ts {
		c[t.Index()]++
	}
	return c
}
