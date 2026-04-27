// Package sichuanxzdd 实现四川麻将「血战到底」规则子集，覆盖交互房间主链路所需的和牌、番种与结算。
package sichuanxzdd

import (
	"context"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

const ruleID = "sichuan_xzdd"

func init() {
	rules.Register(&xzdd{})
}

type xzdd struct{}

func (x *xzdd) ID() string { return ruleID }

func (x *xzdd) Name() string { return "四川麻将血战到底（MVP）" }

func (x *xzdd) BuildWall(ctx context.Context, seed int64) *wall.Wall {
	_ = ctx
	w := wall.NewFull108()
	if seed <= 0 {
		w.ShuffleWithSeed(1)
		return w
	}
	// seed 仅用于可复现洗牌，不用于安全随机；正数范围下转换为 uint64 安全。
	w.ShuffleWithSeed(uint64(seed)) //nolint:gosec // G115：seed>0 时由调用方保证为测试/房间用例值
	return w
}

func (x *xzdd) CheckHu(h *hand.Hand, target tile.Tile, _ rules.HuContext) (rules.HuResult, bool) {
	if h == nil {
		return rules.HuResult{}, false
	}
	c := h.Counts()
	c[target.Index()]++
	if !hu.IsWinning(c) {
		return rules.HuResult{}, false
	}
	return rules.HuResult{Win: c}, true
}

func (x *xzdd) ScoreFans(result rules.HuResult, sc rules.ScoreContext) fan.Breakdown {
	var b fan.Breakdown
	c := result.Win
	if hu.SevenPairs(c) {
		b.Add(fan.KindQiDui, 4, "七对")
	} else {
		if isDuiDuiHu(c) {
			b.Add(fan.KindDuiDuiHu, 2, "对对胡")
		} else {
			b.Add(fan.KindPingHu, 1, "平胡")
		}
	}
	if isQingYiSe(c) {
		b.Add(fan.KindQingYiSe, 4, "清一色")
	}
	for i := 0; i < countGen(c); i++ {
		b.Add(fan.KindYiGen, 1, "一根")
	}
	if sc.IsGangShangHua {
		b.Add(fan.KindGangShangKai, 1, "杠上开花")
	}
	if sc.IsHaiDi {
		if sc.IsTsumo {
			b.Add(fan.KindHaiDiLao, 1, "海底捞月")
		} else {
			b.Add(fan.KindHaiDiPao, 1, "海底炮")
		}
	}
	for _, record := range sc.GangRecords {
		if record.Kind == rules.GangKindBu && record.ResponsibleSeat >= 0 {
			b.Add(fan.KindQiangGangHu, 1, "抢杠胡")
			break
		}
	}
	return b
}

func (x *xzdd) GameOver(state rules.GameState) bool {
	if state.HuedPlayers >= 3 {
		return true
	}
	if state.WallRemaining <= 0 {
		return true
	}
	return false
}

func isQingYiSe(c hu.Counts) bool {
	suits := 0
	for s := 0; s < 3; s++ {
		sum := 0
		for r := 0; r < 9; r++ {
			sum += c[s*9+r]
		}
		if sum > 0 {
			suits++
		}
	}
	return suits == 1
}

func isDuiDuiHu(c hu.Counts) bool {
	pairs := 0
	for _, n := range c {
		switch n {
		case 0:
			continue
		case 2:
			pairs++
		case 3:
		case 4:
			pairs++
		default:
			return false
		}
	}
	return pairs == 1
}

func countGen(c hu.Counts) int {
	n := 0
	for _, v := range c {
		if v == 4 {
			n++
		}
	}
	return n
}
