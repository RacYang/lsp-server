package sichuanxzdd

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/hand"
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
	totalFan := []int32{0, 4, 2, 0}

	scores, penalties, detail := BuildSettlement(playerIDs, hands, queBySeat, []int{1, 2}, totalFan)
	if len(scores) != 4 {
		t.Fatalf("scores len = %d", len(scores))
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
