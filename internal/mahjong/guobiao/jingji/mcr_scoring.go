package jingji

import (
	"sort"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

type mcrContext struct {
	win          hu.Counts
	closed       hu.Counts
	melds        []mcrMeld
	decomps      []mcrDecomp
	scoreContext rules.ScoreContext
}

type mcrMeld struct {
	kind      string
	suit      tile.Suit
	rank      int
	tiles     []tile.Tile
	concealed bool
}

type mcrDecomp struct {
	pair  int
	melds []mcrMeld
}

type mcrAward struct {
	id    string
	count int
}

func scoreMCR(result rules.HuResult, sc rules.ScoreContext) fan.Breakdown {
	ctx := newMCRContext(result, sc)
	awards := detectMCRFans(ctx)
	awards = suppressMCRFans(awards)
	var b fan.Breakdown
	defs := mcrFanDefinitionsByID()
	for _, award := range awards {
		def, ok := defs[award.id]
		if !ok {
			continue
		}
		count := award.count
		if count <= 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			b.Add(fan.Kind("mcr_"+def.ID), def.Points, def.Label)
		}
	}
	if b.Total == 0 && isMCRWinningShape(ctx) {
		if def, ok := defs["chicken_hand"]; ok {
			b.Add(fan.Kind("mcr_"+def.ID), def.Points, def.Label)
		}
	}
	if b.Total < 8 {
		return fan.Breakdown{}
	}
	return b
}

func newMCRContext(result rules.HuResult, sc rules.ScoreContext) mcrContext {
	ctx := mcrContext{
		win:          result.Win,
		closed:       result.Closed,
		scoreContext: sc,
	}
	for _, meld := range result.Melds {
		if converted, ok := mcrMeldFromContext(meld); ok {
			ctx.melds = append(ctx.melds, converted)
		}
	}
	ctx.decomps = enumerateMCRDecomps(result.Closed, ctx.melds)
	return ctx
}

func mcrFanDefinitionsByID() map[string]mcrFanDefinition {
	out := make(map[string]mcrFanDefinition, len(mcrFanRegistry()))
	for _, def := range mcrFanRegistry() {
		out[def.ID] = def
	}
	return out
}

func detectMCRFans(ctx mcrContext) []mcrAward {
	out := make([]mcrAward, 0, 32)
	add := func(id string) {
		out = append(out, mcrAward{id: id, count: 1})
	}
	addN := func(id string, n int) {
		if n > 0 {
			out = append(out, mcrAward{id: id, count: n})
		}
	}

	if countWindPungs(ctx) == 4 {
		add("big_four_winds")
	}
	if countDragonPungs(ctx) == 3 {
		add("big_three_dragons")
	}
	if allGreen(ctx.win) {
		add("all_green")
	}
	if nineGates(ctx.win) {
		add("nine_gates")
	}
	if countKongs(ctx) >= 4 {
		add("four_kongs")
	}
	if sevenShiftedPairs(ctx.win) {
		add("seven_shifted_pairs")
	}
	if thirteenOrphans(ctx.win) {
		add("thirteen_orphans")
	}
	if allTerminals(ctx.win) {
		add("all_terminals")
	}
	if countWindPungs(ctx) == 3 && hasWindPair(ctx) {
		add("little_four_winds")
	}
	if countDragonPungs(ctx) == 2 && hasDragonPair(ctx) {
		add("little_three_dragons")
	}
	if allHonors(ctx.win) {
		add("all_honors")
	}
	if countConcealedPungs(ctx) >= 4 {
		add("four_concealed_pungs")
	}
	if pureTerminalChows(ctx) {
		add("pure_terminal_chows")
	}
	if maxIdenticalChows(ctx, true) >= 4 {
		add("quadruple_chow")
	}
	if pureShiftedPungs(ctx, 4) {
		add("four_pure_shifted_pungs")
	}
	if pureShiftedChows(ctx, 4) {
		add("four_pure_shifted_chows")
	}
	if countKongs(ctx) >= 3 {
		add("three_kongs")
	}
	if allTerminalsAndHonors(ctx.win) {
		add("all_terminals_and_honors")
	}
	if hu.SevenPairs(ctx.win) {
		add("seven_pairs")
	}
	if greaterHonorsAndKnitted(ctx.win) {
		add("greater_honors_and_knitted_tiles")
	}
	if allEvenPungs(ctx) {
		add("all_even_pungs")
	}
	if fullFlushCounts(ctx.win) {
		add("full_flush")
	}
	if maxIdenticalChows(ctx, true) >= 3 {
		add("pure_triple_chow")
	}
	if pureShiftedPungs(ctx, 3) {
		add("pure_shifted_pungs")
	}
	if allRanksInRange(ctx.win, 7, 9) {
		add("upper_tiles")
	}
	if allRanksInRange(ctx.win, 4, 6) {
		add("middle_tiles")
	}
	if allRanksInRange(ctx.win, 1, 3) {
		add("lower_tiles")
	}
	if pureStraight(ctx) {
		add("pure_straight")
	}
	if threeSuitedTerminalChows(ctx) {
		add("three_suited_terminal_chows")
	}
	if pureShiftedChows(ctx, 3) {
		add("pure_shifted_chows")
	}
	if allFives(ctx) {
		add("all_fives")
	}
	if triplePung(ctx) {
		add("triple_pung")
	}
	if countConcealedPungs(ctx) >= 3 {
		add("three_concealed_pungs")
	}
	if lesserHonorsAndKnitted(ctx.win) {
		add("lesser_honors_and_knitted_tiles")
	}
	if knittedStraight(ctx.win) {
		add("knitted_straight")
	}
	if allRanksInRange(ctx.win, 6, 9) {
		add("upper_four")
	}
	if allRanksInRange(ctx.win, 1, 4) {
		add("lower_four")
	}
	if countWindPungs(ctx) >= 3 {
		add("big_three_winds")
	}
	if mixedStraight(ctx) {
		add("mixed_straight")
	}
	if reversibleTiles(ctx.win) {
		add("reversible_tiles")
	}
	if mixedTripleChow(ctx) {
		add("mixed_triple_chow")
	}
	if mixedShiftedPungs(ctx) {
		add("mixed_shifted_pungs")
	}
	if sc := ctx.scoreContext; sc.IsHaiDi && sc.IsTsumo {
		add("last_tile_draw")
	} else if sc.IsHaiDi {
		add("last_tile_claim")
	}
	if ctx.scoreContext.IsGangShangHua {
		add("out_with_replacement_tile")
	}
	if ctx.scoreContext.IsGangShangPao {
		add("robbing_the_kong")
	}
	if allPungsMCR(ctx) {
		add("all_pungs")
	}
	if halfFlushCounts(ctx.win) {
		add("half_flush")
	}
	if mixedShiftedChows(ctx) {
		add("mixed_shifted_chows")
	}
	if allTypes(ctx.win) {
		add("all_types")
	}
	if meldedHand(ctx) {
		add("melded_hand")
	}
	addN("two_concealed_kongs", countConcealedKongs(ctx)/2)
	if countDragonPungs(ctx) >= 2 {
		add("two_dragon_pungs")
	}
	if outsideHand(ctx) {
		add("outside_hand")
	}
	if fullyConcealedHand(ctx) {
		add("fully_concealed_hand")
	}
	addN("two_melded_kongs", countMeldedKongs(ctx)/2)
	if ctx.scoreContext.WallRemaining == 0 && ctx.scoreContext.Step > 0 {
		add("last_tile")
	}
	addN("dragon_pung", countDragonPungs(ctx))
	addN("prevalent_wind", countSpecificHonorPungs(ctx, 27))
	if ctx.scoreContext.HuSeat >= 0 {
		addN("seat_wind", countSpecificHonorPungs(ctx, 27+int(ctx.scoreContext.HuSeat)%4))
	}
	if concealedHand(ctx) {
		add("concealed_hand")
	}
	if allChows(ctx) {
		add("all_chows")
	}
	addN("tile_hog", tileHog(ctx.win))
	addN("double_pung", doublePung(ctx))
	addN("two_concealed_pungs", countConcealedPungs(ctx)/2)
	addN("concealed_kong", countConcealedKongs(ctx))
	if allSimplesCounts(ctx.win) {
		add("all_simples")
	}
	addN("pure_double_chow", pureDoubleChow(ctx))
	addN("mixed_double_chow", mixedDoubleChow(ctx))
	addN("short_straight", shortStraight(ctx))
	addN("two_terminal_chows", twoTerminalChows(ctx))
	addN("pung_of_terminals_or_honors", terminalOrHonorPungs(ctx))
	addN("melded_kong", countMeldedKongs(ctx))
	if oneVoidedSuit(ctx.win) {
		add("one_voided_suit")
	}
	if noHonors(ctx.win) {
		add("no_honors")
	}
	if edgeWait(ctx) {
		add("edge_wait")
	}
	if closedWait(ctx) {
		add("closed_wait")
	}
	if singleWait(ctx) {
		add("single_wait")
	}
	if ctx.scoreContext.IsTsumo {
		add("self_drawn")
	}
	addN("flower_tile", flowerCount(ctx.scoreContext, ctx.scoreContext.HuSeat))
	return out
}

func suppressMCRFans(in []mcrAward) []mcrAward {
	present := map[string]bool{}
	for _, award := range in {
		present[award.id] = true
	}
	suppressed := map[string]bool{}
	for high, lows := range mcrSuppressionRules() {
		if !present[high] {
			continue
		}
		for _, low := range lows {
			suppressed[low] = true
		}
	}
	out := make([]mcrAward, 0, len(in))
	for _, award := range in {
		if !suppressed[award.id] {
			out = append(out, award)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		defs := mcrFanDefinitionsByID()
		return defs[out[i].id].Points > defs[out[j].id].Points
	})
	return out
}

func mcrSuppressionRules() map[string][]string {
	return map[string][]string{
		"big_four_winds":                   {"big_three_winds", "seat_wind", "prevalent_wind", "pung_of_terminals_or_honors", "all_pungs"},
		"little_four_winds":                {"big_three_winds", "seat_wind", "prevalent_wind", "pung_of_terminals_or_honors"},
		"big_three_dragons":                {"two_dragon_pungs", "dragon_pung", "pung_of_terminals_or_honors", "three_concealed_pungs", "two_concealed_pungs"},
		"little_three_dragons":             {"two_dragon_pungs", "dragon_pung", "pung_of_terminals_or_honors"},
		"all_honors":                       {"all_terminals_and_honors", "pung_of_terminals_or_honors", "outside_hand"},
		"all_terminals":                    {"pung_of_terminals_or_honors", "outside_hand", "all_pungs", "no_honors"},
		"all_terminals_and_honors":         {"pung_of_terminals_or_honors", "outside_hand"},
		"nine_gates":                       {"full_flush", "one_voided_suit", "no_honors", "concealed_hand", "tile_hog", "two_concealed_pungs", "pung_of_terminals_or_honors"},
		"full_flush":                       {"half_flush", "one_voided_suit", "no_honors"},
		"half_flush":                       {"one_voided_suit"},
		"all_pungs":                        {"pung_of_terminals_or_honors"},
		"seven_shifted_pairs":              {"seven_pairs", "full_flush", "one_voided_suit", "no_honors", "all_chows", "pure_double_chow", "mixed_double_chow", "short_straight", "concealed_hand", "single_wait"},
		"thirteen_orphans":                 {"all_types", "outside_hand", "pung_of_terminals_or_honors"},
		"greater_honors_and_knitted_tiles": {"lesser_honors_and_knitted_tiles", "all_types"},
		"pure_terminal_chows":              {"pure_double_chow", "two_terminal_chows", "no_honors", "one_voided_suit"},
		"pure_straight":                    {"short_straight"},
		"mixed_straight":                   {"short_straight"},
		"four_concealed_pungs":             {"three_concealed_pungs", "two_concealed_pungs"},
		"quadruple_chow":                   {"seven_pairs", "pure_triple_chow", "pure_shifted_pungs", "three_concealed_pungs", "two_concealed_pungs", "pure_double_chow", "mixed_double_chow", "tile_hog", "pung_of_terminals_or_honors"},
		"pure_triple_chow":                 {"pure_double_chow"},
		"four_pure_shifted_chows":          {"pure_shifted_chows", "short_straight"},
		"four_pure_shifted_pungs":          {"pure_shifted_pungs"},
		"all_simples":                      {"no_honors"},
		"all_chows":                        {"no_honors"},
		"fully_concealed_hand":             {"concealed_hand", "self_drawn"},
	}
}

func isMCRWinningShape(ctx mcrContext) bool {
	return ctx.win.Total() == 14 || len(ctx.melds) > 0
}
