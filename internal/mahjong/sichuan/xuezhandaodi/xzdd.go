// Package xuezhandaodi 实现四川麻将「血战到底」规则子集，覆盖交互房间主链路所需的和牌、番种与结算。
package xuezhandaodi

import (
	"context"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

const (
	IDHuansanzhang = "sichuan_xuezhandaodi_huansanzhang"
	IDBiaozhun     = "sichuan_xuezhandaodi_biaozhun"
)

func init() {
	rules.Register(newRule(IDHuansanzhang, true))
	rules.Register(newRule(IDBiaozhun, false))
}

type rule struct {
	id           string
	withExchange bool
}

func newRule(id string, withExchange bool) *rule {
	return &rule{id: id, withExchange: withExchange}
}

func (x *rule) ID() string { return x.id }

func (x *rule) Name() string {
	if x.withExchange {
		return "四川麻将血战到底（换三张）"
	}
	return "四川麻将血战到底（标准）"
}

func (x *rule) Capabilities() rules.CapabilitySet {
	features := []string{
		"que_men",
		"no_chi",
		"pong",
		"ming_gang",
		"an_gang",
		"bu_gang",
		"qiang_gang_hu",
		"xuezhan_continue",
		"hai_di",
		"gang_shang",
		"settlement_penalty",
	}
	opening := rules.StaticOpeningFlow{"que_men"}
	displayName := "四川血战到底"
	shortDesc := "定缺、无吃、胡牌后血战续行"
	if x.withExchange {
		features = append([]string{"exchange_three"}, features...)
		opening = rules.StaticOpeningFlow{"exchange_three", "que_men"}
		displayName = "四川血战到底（换三张）"
		shortDesc = "换三张、定缺、无吃、胡牌后血战续行"
	}
	return rules.CapabilitySet{
		Metadata: rules.RuleMetadata{
			DisplayName:     displayName,
			ShortDesc:       shortDesc,
			EnabledFeatures: append([]string(nil), features...),
			MaxHands:        4,
		},
		Opening:     opening,
		Claims:      rules.NoEatingClaimPolicy{},
		Turn:        rules.FeatureSet{"draw", "tsumo_window", "gang_follow_up", "hai_di"},
		Scoring:     rules.FeatureSet{"fan_breakdown", "dealer", "advanced_fans", "gang_context"},
		Settlement:  rules.FeatureSet{"ledger", "cha_hua_zhu", "cha_da_jiao", "tax_refund"},
		Termination: rules.FeatureSet{"three_hued_or_wall_empty"},
		Projection:  rules.FeatureSet{"per_seat_hand", "round_snapshot", "tui_authority"},
	}
}

func (x *rule) BuildWall(ctx context.Context, seed int64) *wall.Wall {
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

func (x *rule) CheckHu(h *hand.Hand, target tile.Tile, _ rules.HuContext) (rules.HuResult, bool) {
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

func (x *rule) ScoreFans(result rules.HuResult, sc rules.ScoreContext) fan.Breakdown {
	var b fan.Breakdown
	c := result.Win
	specialOpening := false
	switch {
	case sc.HuSeat == sc.DealerSeat && sc.IsTsumo && sc.IsOpeningDraw:
		b.Add(fan.KindHeavenlyHand, 8, "天胡")
		specialOpening = true
	case sc.HuSeat != sc.DealerSeat && !sc.IsTsumo && sc.IsDealerFirstDiscard:
		b.Add(fan.KindEarthlyHand, 8, "地胡")
		specialOpening = true
	}
	genCount := countGen(c)
	longQiDui := hu.SevenPairs(c) && genCount > 0
	switch {
	case specialOpening:
		// 天胡/地胡替代基础牌型番，但仍保留清一色、杠上炮等场况番。
	case longQiDui:
		b.Add(fan.KindLongQiDui, 8, "龙七对")
	case hu.SevenPairs(c):
		b.Add(fan.KindQiDui, 4, "七对")
	default:
		switch {
		case isJiangDui(c):
			b.Add(fan.KindJiangDui, 4, "将对")
		case isDuiDuiHu(c):
			b.Add(fan.KindDuiDuiHu, 2, "对对胡")
		default:
			b.Add(fan.KindPingHu, 1, "平胡")
		}
	}
	if isQingYiSe(c) {
		b.Add(fan.KindQingYiSe, 4, "清一色")
	}
	if !longQiDui {
		for i := 0; i < genCount; i++ {
			b.Add(fan.KindYiGen, 1, "一根")
		}
	}
	shiBaLuoHan := countSeatGang(sc.GangRecords, sc.HuSeat) >= 4
	if shiBaLuoHan {
		b.Add(fan.KindShiBaLuoHan, 16, "十八罗汉")
	}
	if sc.IsGangShangHua {
		b.Add(fan.KindGangShangKai, 1, "杠上开花")
	}
	if sc.IsGangShangPao {
		b.Add(fan.KindGangShangPao, 1, "杠上炮")
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
	for i := 0; i < countAnKe(c); i++ {
		b.Add(fan.KindAnKe, 1, "暗刻")
	}
	if !shiBaLuoHan {
		anGang := countAnGang(sc.GangRecords, sc.HuSeat)
		for i := 0; i < anGang; i++ {
			b.Add(fan.KindAnGang, 1, "暗杠")
		}
		if anGang >= 2 {
			b.Add(fan.KindShuangAnGang, 1, "双暗杠")
		}
	}
	return b
}

func (x *rule) GameOver(state rules.GameState) bool {
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

func isJiangDui(c hu.Counts) bool {
	if !isDuiDuiHu(c) {
		return false
	}
	for idx, n := range c {
		if n == 0 {
			continue
		}
		rank := idx%9 + 1
		if rank != 2 && rank != 5 && rank != 8 {
			return false
		}
	}
	return true
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

func countAnKe(c hu.Counts) int {
	n := 0
	for _, v := range c {
		if v == 3 {
			n++
		}
	}
	return n
}

func countSeatGang(records []rules.GangRecord, seat domainroom.Seat) int {
	n := 0
	for _, record := range records {
		if record.Seat == seat && record.Kind != rules.GangKindUnspecified {
			n++
		}
	}
	return n
}

func countAnGang(records []rules.GangRecord, seat domainroom.Seat) int {
	n := 0
	for _, record := range records {
		if record.Seat == seat && record.Kind == rules.GangKindAn {
			n++
		}
	}
	return n
}
