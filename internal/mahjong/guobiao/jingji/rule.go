// Package jingji 实现国标竞技麻将规则包的注册入口与策略装配。
package jingji

import (
	"context"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

const ID = "guobiao_jingji_biaozhun"

func init() {
	rules.Register(rule{})
}

type rule struct{}

func (rule) ID() string { return ID }

func (rule) Name() string { return "国标麻将（竞技标准）" }

func (rule) Capabilities() rules.CapabilitySet {
	return rules.CapabilitySet{
		Metadata: rules.RuleMetadata{
			DisplayName: "国标麻将（竞技标准）",
			ShortDesc:   "完整牌组、吃碰杠、花牌补花、8 分起胡",
			EnabledFeatures: []string{
				"full_tiles",
				"honors",
				"flowers",
				"mcr_81_fans",
				"win_ends_round",
				"chi",
				"pong",
				"ming_gang",
				"an_gang",
				"bu_gang",
			},
			MaxHands: 4,
		},
		TileSet:     rule{},
		Claims:      rules.StandardClaimPolicy{},
		SelfActions: rules.StandardSelfActionPolicy{},
		Win:         rule{},
		State:       rules.EmptyRuleStatePolicy{},
		StateView:   rules.EmptyRuleStatePolicy{},
		Turn:        rules.FeatureSet{"draw", "tsumo_window", "gang_follow_up", "flower_supplement"},
		Scoring:     scoringPolicy{},
		Settlement:  settlementPolicy{},
		Termination: terminationPolicy{},
		Projection:  rules.FeatureSet{"per_seat_hand", "round_snapshot", "tui_authority"},
	}
}

func (rule) BuildWall(ctx context.Context, seed int64) *wall.Wall {
	_ = ctx
	w := wall.NewFull144()
	if seed <= 0 {
		w.ShuffleWithSeed(1)
		return w
	}
	w.ShuffleWithSeed(uint64(seed)) //nolint:gosec // seed>0 时由调用方保证为测试/房间用例值
	return w
}

func (rule) CheckHu(h *hand.Hand, target tile.Tile, hc rules.HuContext) (rules.HuResult, bool) {
	if h == nil || target == 0 || target.IsFlower() {
		return rules.HuResult{}, false
	}
	closed := h.Counts()
	idx := target.Index()
	if idx < 0 || idx >= tile.PlayableTileCount {
		return rules.HuResult{}, false
	}
	closed[idx]++
	openMelds := countOpenMelds(hc.Melds)
	if !isMCRLegalWin(closed, openMelds) {
		return rules.HuResult{}, false
	}
	win := closed
	for _, meld := range hc.Melds {
		for _, t := range meld.Tiles {
			if t != 0 && !t.IsFlower() {
				win[t.Index()]++
			}
		}
	}
	return rules.HuResult{
		Win:       win,
		Closed:    closed,
		OpenMelds: openMelds,
		Melds:     cloneMeldContexts(hc.Melds),
	}, true
}

func isMCRLegalWin(closed hu.Counts, openMelds int) bool {
	if hu.IsWinningWithOpenMelds(closed, openMelds) {
		return true
	}
	if openMelds != 0 {
		return false
	}
	return thirteenOrphans(closed) || greaterHonorsAndKnitted(closed) || lesserHonorsAndKnitted(closed)
}

type terminationPolicy struct{}

func (terminationPolicy) FeatureFlags() []string { return []string{"first_win_or_wall_empty"} }

func (terminationPolicy) GameOver(ctx rules.TerminationContext) bool {
	return len(ctx.WinEvents) >= 1 || ctx.WallRemaining <= 0
}

type settlementPolicy struct{}

func (settlementPolicy) FeatureFlags() []string {
	return []string{"seat_scores", "per_winner_breakdown"}
}

func (settlementPolicy) BuildSettlement(ctx rules.SettlementContext) rules.SettlementResult {
	winnerSeats := winnerSeatsFromEvents(ctx.WinEvents)
	seatScores := defaultSeatScores(ctx.PlayerIDs, ctx.ScoreEvents)
	return rules.SettlementResult{
		WinnerUserIDs:      winnerUserIDs(ctx.PlayerIDs, winnerSeats),
		SeatScores:         seatScores,
		PerWinnerBreakdown: winnerBreakdowns(ctx.PlayerIDs, ctx.ScoreEvents, winnerSeats),
		DetailText:         detailText(winnerSeats),
	}
}

func countOpenMelds(melds []rules.MeldContext) int {
	n := 0
	for _, meld := range melds {
		if len(meld.Tiles) >= 3 {
			n++
		}
	}
	return n
}

func cloneMeldContexts(in []rules.MeldContext) []rules.MeldContext {
	if len(in) == 0 {
		return nil
	}
	out := make([]rules.MeldContext, len(in))
	for i, meld := range in {
		out[i] = meld
		out[i].Tiles = append([]tile.Tile(nil), meld.Tiles...)
	}
	return out
}

func flowerCount(sc rules.ScoreContext, seat domainroom.Seat) int {
	if seat < 0 || int(seat) >= len(sc.SeatGenTiles) {
		return 0
	}
	n := 0
	for _, t := range sc.SeatGenTiles[seat] {
		if t.IsFlower() {
			n++
		}
	}
	return n
}

func defaultSeatScores(playerIDs [4]string, scoreEvents []rules.ScoreEvent) []*clientv1.SeatScore {
	balances := make([]int32, 4)
	for _, entry := range scoreEvents {
		if entry.Amount <= 0 {
			continue
		}
		if entry.FromSeat >= 0 && entry.FromSeat < 4 {
			balances[entry.FromSeat] -= entry.Amount
		}
		if entry.ToSeat >= 0 && entry.ToSeat < 4 {
			balances[entry.ToSeat] += entry.Amount
		}
	}
	out := make([]*clientv1.SeatScore, 0, 4)
	for seat, total := range balances {
		out = append(out, &clientv1.SeatScore{
			SeatIndex: int32(seat), //nolint:gosec // 固定四座
			UserId:    playerIDs[seat],
			TotalFan:  total,
		})
	}
	return out
}

func winnerUserIDs(playerIDs [4]string, winnerSeats []domainroom.Seat) []string {
	out := make([]string, 0, len(winnerSeats))
	for _, seat := range winnerSeats {
		if seat >= 0 && int(seat) < len(playerIDs) {
			out = append(out, playerIDs[seat])
		}
	}
	return out
}

func winnerSeatsFromEvents(events []rules.WinEvent) []domainroom.Seat {
	out := make([]domainroom.Seat, 0, len(events))
	seen := map[domainroom.Seat]struct{}{}
	for _, event := range events {
		if _, ok := seen[event.Seat]; ok {
			continue
		}
		seen[event.Seat] = struct{}{}
		out = append(out, event.Seat)
	}
	return out
}

func winnerBreakdowns(playerIDs [4]string, scoreEvents []rules.ScoreEvent, winnerSeats []domainroom.Seat) []*clientv1.WinnerBreakdown {
	winners := map[domainroom.Seat]struct{}{}
	for _, seat := range winnerSeats {
		winners[seat] = struct{}{}
	}
	bySeat := map[domainroom.Seat]*clientv1.WinnerBreakdown{}
	seen := map[domainroom.Seat]map[string]struct{}{}
	for _, entry := range scoreEvents {
		if _, ok := winners[entry.WinnerSeat]; !ok {
			continue
		}
		b := bySeat[entry.WinnerSeat]
		if b == nil {
			b = &clientv1.WinnerBreakdown{SeatIndex: entry.WinnerSeat.Proto(), UserId: playerIDs[entry.WinnerSeat]}
			bySeat[entry.WinnerSeat] = b
			seen[entry.WinnerSeat] = map[string]struct{}{}
		}
		if entry.WinnerFan > b.Fan {
			b.Fan = entry.WinnerFan
		}
		for _, name := range entry.FanNames {
			if name == "" {
				continue
			}
			if _, ok := seen[entry.WinnerSeat][name]; ok {
				continue
			}
			seen[entry.WinnerSeat][name] = struct{}{}
			b.FanNames = append(b.FanNames, name)
		}
	}
	out := make([]*clientv1.WinnerBreakdown, 0, len(winners))
	for seat := 0; seat < 4; seat++ {
		if b := bySeat[domainroom.SeatFromInt(seat)]; b != nil {
			out = append(out, b)
		}
	}
	return out
}

func detailText(winnerSeats []domainroom.Seat) string {
	if len(winnerSeats) == 0 {
		return "荒牌"
	}
	return "国标麻将胡牌"
}
