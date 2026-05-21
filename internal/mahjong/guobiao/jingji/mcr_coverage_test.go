package jingji

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

type mcrFanFixture struct {
	name       string
	closed     []string
	melds      []mcrFixtureMeld
	score      rules.ScoreContext
	primary    []string
	rawWant    []string
	scoredWant []string
	notRaw     []string
	notScored  []string
	exactTotal int
	minTotal   int
}

type mcrFixtureMeld struct {
	kind      string
	tiles     []string
	concealed bool
}

func TestMCRFanFixturesCoverEveryRegistryFan(t *testing.T) {
	t.Parallel()

	covered := map[string]string{}
	for _, fx := range mcrFanFixtures() {
		ctx := fx.context(t)
		raw := rawAwardIDs(detectMCRFans(ctx))
		for _, id := range fx.rawIDs() {
			require.Contains(t, raw, id, "%s should detect %s", fx.name, id)
		}
		for _, id := range fx.primaryIDs() {
			require.Contains(t, raw, id, "%s should primarily cover %s", fx.name, id)
			if _, ok := covered[id]; !ok {
				covered[id] = fx.name
			}
		}
		for _, id := range fx.notRaw {
			require.NotContains(t, raw, id, "%s should not detect %s", fx.name, id)
		}
	}
	chicken := chickenFixture()
	chickenBreakdown := scoreMCR(chicken.result(t), chicken.normalizedScore())
	require.Contains(t, fanKinds(chickenBreakdown), fan.Kind("mcr_chicken_hand"))
	covered["chicken_hand"] = chicken.name

	var missing []string
	for _, def := range mcrFanRegistry() {
		if _, ok := covered[def.ID]; !ok {
			missing = append(missing, def.ID)
		}
	}
	require.Empty(t, missing, "MCR registry fans without positive fixture coverage")
}

func TestMCRFanFixturesScoreAndSuppress(t *testing.T) {
	t.Parallel()

	for _, fx := range mcrFanFixtures() {
		fx := fx
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()

			breakdown := scoreMCR(fx.result(t), fx.normalizedScore())
			kinds := fanKinds(breakdown)
			for _, id := range fx.scoredIDs() {
				if _, suppressed := stringSet(fx.notScored)[id]; suppressed {
					continue
				}
				require.Contains(t, kinds, fan.Kind("mcr_"+id), "%s should score %s", fx.name, id)
			}
			for _, id := range fx.notScored {
				require.NotContains(t, kinds, fan.Kind("mcr_"+id), "%s should suppress %s", fx.name, id)
			}
			if fx.exactTotal > 0 {
				require.Equal(t, fx.exactTotal, breakdown.Total, "%s total", fx.name)
			}
			if fx.minTotal > 0 {
				require.GreaterOrEqual(t, breakdown.Total, fx.minTotal, "%s total", fx.name)
			}
		})
	}
}

func TestMCRRepresentativeFixtureExactItems(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		fixt  mcrFanFixture
		items []fan.Item
		total int
	}{
		{
			name:  "big_three_dragons",
			fixt:  fixtureByName(t, "big_three_dragons"),
			total: 96,
			items: []fan.Item{
				{Kind: "mcr_big_three_dragons", Fan: 88, Label: "大三元"},
				{Kind: "mcr_outside_hand", Fan: 4, Label: "全带幺"},
				{Kind: "mcr_concealed_hand", Fan: 2, Label: "门前清"},
				{Kind: "mcr_one_voided_suit", Fan: 1, Label: "缺一门"},
				{Kind: "mcr_single_wait", Fan: 1, Label: "单钓将"},
			},
		},
		{
			name:  "nine_gates",
			fixt:  fixtureByName(t, "nine_gates"),
			total: 88,
			items: []fan.Item{{Kind: "mcr_nine_gates", Fan: 88, Label: "九莲宝灯"}},
		},
		{
			name:  "seven_shifted_pairs",
			fixt:  fixtureByName(t, "seven_shifted_pairs"),
			total: 88,
			items: []fan.Item{{Kind: "mcr_seven_shifted_pairs", Fan: 88, Label: "连七对"}},
		},
		{
			name:  "last_tile_draw",
			fixt:  fixtureByName(t, "last_tile_draw"),
			total: 9,
			items: []fan.Item{
				{Kind: "mcr_last_tile_draw", Fan: 8, Label: "妙手回春"},
				{Kind: "mcr_self_drawn", Fan: 1, Label: "自摸"},
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			breakdown := scoreMCR(tc.fixt.result(t), tc.fixt.normalizedScore())
			require.Equal(t, tc.total, breakdown.Total, "%+v", breakdown.Items)
			require.Equal(t, tc.items, breakdown.Items)
		})
	}
}

func TestMCRChickenHandRequiresNoOtherFan(t *testing.T) {
	t.Parallel()

	chicken := chickenFixture()
	breakdown := scoreMCR(chicken.result(t), chicken.normalizedScore())
	require.Equal(t, 8, breakdown.Total)
	require.Equal(t, []fan.Item{{Kind: "mcr_chicken_hand", Fan: 8, Label: "无番和"}}, breakdown.Items)

	lowOtherFan := mcrFanFixture{
		name:   "low_other_fan",
		closed: []string{"m1", "m2", "m3", "m4", "m5", "m6", "p2", "p3", "p4", "s2", "s3", "s4", "z1", "z1"},
		score:  rules.ScoreContext{WallRemaining: 50, WinningTile: mustParseTileForTest("z1")},
	}
	breakdown = scoreMCR(lowOtherFan.result(t), lowOtherFan.normalizedScore())
	require.Zero(t, breakdown.Total)
	require.Empty(t, breakdown.Items)
}

func TestMCRSettlementPayments(t *testing.T) {
	t.Parallel()

	result := mcrFanFixture{
		name:   "settlement_full_flush",
		closed: []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "m2", "m2", "m2", "m5", "m5"},
	}.result(t)
	policy := scoringPolicy{}

	breakdown, events, ok := policy.ScoreWin(result, rules.ScoreContext{
		HuSeat:          0,
		ResponsibleSeat: 2,
		ActiveSeats:     []room.Seat{0, 1, 2, 3},
		WallRemaining:   50,
		WinningTile:     mustParseTileForTest("m5"),
	})
	require.True(t, ok)
	require.Contains(t, fanKinds(breakdown), fan.Kind("mcr_full_flush"))
	require.Len(t, events, 1)
	require.Equal(t, room.Seat(2), events[0].FromSeat)
	require.Equal(t, room.Seat(0), events[0].ToSeat)

	_, events, ok = policy.ScoreWin(result, rules.ScoreContext{
		HuSeat:        0,
		IsTsumo:       true,
		ActiveSeats:   []room.Seat{0, 1, 2, 3},
		WallRemaining: 50,
		WinningTile:   mustParseTileForTest("m5"),
	})
	require.True(t, ok)
	require.Len(t, events, 3)
	for _, event := range events {
		require.Equal(t, room.Seat(0), event.ToSeat)
		require.NotEqual(t, room.Seat(0), event.FromSeat)
	}
}

func mcrFanFixtures() []mcrFanFixture {
	return []mcrFanFixture{
		{
			name:       "big_four_winds",
			closed:     []string{"z1", "z1", "z1", "z2", "z2", "z2", "z3", "z3", "z3", "z4", "z4", "z4", "z5", "z5"},
			score:      rules.ScoreContext{HuSeat: 0, WallRemaining: 50},
			primary:    []string{"big_four_winds", "all_honors", "big_three_winds", "all_pungs", "prevalent_wind", "seat_wind", "pung_of_terminals_or_honors"},
			rawWant:    []string{"big_four_winds", "all_honors", "big_three_winds", "all_pungs", "prevalent_wind", "seat_wind", "pung_of_terminals_or_honors"},
			scoredWant: []string{"big_four_winds", "all_honors"},
			notScored:  []string{"big_three_winds", "all_pungs", "prevalent_wind", "seat_wind", "pung_of_terminals_or_honors"},
			minTotal:   88,
		},
		{
			name:       "big_three_dragons",
			closed:     []string{"z5", "z5", "z5", "z6", "z6", "z6", "z7", "z7", "z7", "m1", "m2", "m3", "p1", "p1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"big_three_dragons", "two_dragon_pungs", "dragon_pung"},
			rawWant:    []string{"big_three_dragons", "two_dragon_pungs", "dragon_pung"},
			scoredWant: []string{"big_three_dragons", "outside_hand", "concealed_hand", "one_voided_suit", "single_wait"},
		},
		{
			name:       "all_green",
			closed:     []string{"s2", "s3", "s4", "s2", "s3", "s4", "s6", "s6", "s6", "s8", "s8", "s8", "z6", "z6"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"all_green"},
			rawWant:    []string{"all_green"},
			scoredWant: []string{"all_green"},
		},
		{
			name:       "nine_gates",
			closed:     []string{"m1", "m1", "m1", "m2", "m3", "m4", "m5", "m5", "m6", "m7", "m8", "m9", "m9", "m9"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"nine_gates", "full_flush"},
			rawWant:    []string{"nine_gates", "full_flush"},
			scoredWant: []string{"nine_gates"},
			notScored:  []string{"full_flush"},
		},
		{
			name:   "four_kongs",
			closed: []string{"z1", "z1"},
			melds: []mcrFixtureMeld{
				{kind: "ming_gang", tiles: []string{"m1", "m1", "m1", "m1"}},
				{kind: "ming_gang", tiles: []string{"p2", "p2", "p2", "p2"}},
				{kind: "an_gang", tiles: []string{"s3", "s3", "s3", "s3"}, concealed: true},
				{kind: "an_gang", tiles: []string{"z5", "z5", "z5", "z5"}, concealed: true},
			},
			score:      rules.ScoreContext{HuSeat: 0, WallRemaining: 50},
			primary:    []string{"four_kongs", "three_kongs", "two_concealed_kongs", "two_melded_kongs", "concealed_kong", "melded_kong"},
			rawWant:    []string{"four_kongs", "three_kongs", "two_concealed_kongs", "two_melded_kongs", "concealed_kong", "melded_kong"},
			scoredWant: []string{"four_kongs", "three_kongs", "two_concealed_kongs", "two_melded_kongs", "concealed_kong", "melded_kong"},
		},
		{
			name:       "seven_shifted_pairs",
			closed:     []string{"m1", "m1", "m2", "m2", "m3", "m3", "m4", "m4", "m5", "m5", "m6", "m6", "m7", "m7"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"seven_shifted_pairs", "seven_pairs"},
			rawWant:    []string{"seven_shifted_pairs", "seven_pairs"},
			scoredWant: []string{"seven_shifted_pairs"},
			notScored:  []string{"seven_pairs"},
		},
		{
			name:       "thirteen_orphans",
			closed:     []string{"m1", "m1", "m9", "p1", "p9", "s1", "s9", "z1", "z2", "z3", "z4", "z5", "z6", "z7"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"thirteen_orphans", "all_types"},
			rawWant:    []string{"thirteen_orphans", "all_types"},
			scoredWant: []string{"thirteen_orphans"},
			notScored:  []string{"all_types"},
		},
		{
			name:       "greater_honors_and_knitted_tiles",
			closed:     []string{"m1", "m4", "m7", "p2", "p5", "p8", "s3", "z1", "z2", "z3", "z4", "z5", "z6", "z7"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"greater_honors_and_knitted_tiles"},
			rawWant:    []string{"greater_honors_and_knitted_tiles"},
			scoredWant: []string{"greater_honors_and_knitted_tiles"},
		},
		{
			name:       "all_terminals",
			closed:     []string{"m1", "m1", "m1", "m9", "m9", "m9", "p1", "p1", "p1", "p9", "p9", "p9", "s1", "s1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"all_terminals", "all_pungs", "no_honors"},
			rawWant:    []string{"all_terminals", "all_pungs", "no_honors"},
			scoredWant: []string{"all_terminals"},
			notScored:  []string{"all_pungs", "no_honors"},
		},
		{
			name:       "little_four_winds",
			closed:     []string{"z1", "z1", "z1", "z2", "z2", "z2", "z3", "z3", "z3", "z4", "z4", "m1", "m2", "m3"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"little_four_winds", "big_three_winds"},
			rawWant:    []string{"little_four_winds", "big_three_winds"},
			scoredWant: []string{"little_four_winds"},
			notScored:  []string{"big_three_winds"},
		},
		{
			name:       "little_three_dragons",
			closed:     []string{"z5", "z5", "z5", "z6", "z6", "z6", "z7", "z7", "m1", "m2", "m3", "p1", "p2", "p3"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"little_three_dragons", "two_dragon_pungs", "dragon_pung"},
			rawWant:    []string{"little_three_dragons", "two_dragon_pungs", "dragon_pung"},
			scoredWant: []string{"little_three_dragons"},
			notScored:  []string{"two_dragon_pungs", "dragon_pung"},
		},
		{
			name:       "four_concealed_pungs",
			closed:     []string{"m2", "m2", "m2", "p3", "p3", "p3", "s4", "s4", "s4", "m5", "m5", "m5", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"four_concealed_pungs", "three_concealed_pungs", "two_concealed_pungs", "all_pungs"},
			rawWant:    []string{"four_concealed_pungs", "three_concealed_pungs", "two_concealed_pungs", "all_pungs"},
			scoredWant: []string{"four_concealed_pungs", "all_pungs"},
		},
		{
			name:       "pure_terminal_chows",
			closed:     []string{"m1", "m2", "m3", "m1", "m2", "m3", "m7", "m8", "m9", "m7", "m8", "m9", "m5", "m5"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"pure_terminal_chows", "pure_double_chow", "two_terminal_chows"},
			rawWant:    []string{"pure_terminal_chows", "pure_double_chow", "two_terminal_chows"},
			scoredWant: []string{"pure_terminal_chows"},
			notScored:  []string{"pure_double_chow", "two_terminal_chows"},
		},
		{
			name:       "quadruple_chow",
			closed:     []string{"m1", "m1", "m1", "m1", "m2", "m2", "m2", "m2", "m3", "m3", "m3", "m3", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"quadruple_chow", "pure_triple_chow", "pure_double_chow"},
			rawWant:    []string{"quadruple_chow", "pure_triple_chow", "pure_double_chow"},
			scoredWant: []string{"quadruple_chow"},
			notScored:  []string{"pure_triple_chow", "pure_double_chow"},
		},
		{
			name:       "four_pure_shifted_pungs",
			closed:     []string{"m1", "m1", "m1", "m2", "m2", "m2", "m3", "m3", "m3", "m4", "m4", "m4", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"four_pure_shifted_pungs", "pure_shifted_pungs"},
			rawWant:    []string{"four_pure_shifted_pungs", "pure_shifted_pungs"},
			scoredWant: []string{"four_pure_shifted_pungs"},
			notScored:  []string{"pure_shifted_pungs"},
		},
		{
			name:       "four_pure_shifted_chows",
			closed:     []string{"m1", "m2", "m3", "m2", "m3", "m4", "m3", "m4", "m5", "m4", "m5", "m6", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"four_pure_shifted_chows", "pure_shifted_chows", "short_straight"},
			rawWant:    []string{"four_pure_shifted_chows", "pure_shifted_chows", "short_straight"},
			scoredWant: []string{"four_pure_shifted_chows"},
			notScored:  []string{"pure_shifted_chows", "short_straight"},
		},
		{
			name:       "all_terminals_and_honors",
			closed:     []string{"m1", "m1", "m1", "p9", "p9", "p9", "z1", "z1", "z1", "z5", "z5", "z5", "s1", "s1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"all_terminals_and_honors", "all_pungs", "pung_of_terminals_or_honors"},
			rawWant:    []string{"all_terminals_and_honors", "all_pungs", "pung_of_terminals_or_honors"},
			scoredWant: []string{"all_terminals_and_honors", "all_pungs"},
			notScored:  []string{"pung_of_terminals_or_honors"},
		},
		{
			name:       "all_even_pungs",
			closed:     []string{"m2", "m2", "m2", "p4", "p4", "p4", "s6", "s6", "s6", "m8", "m8", "m8", "p2", "p2"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"all_even_pungs", "all_pungs", "all_simples"},
			rawWant:    []string{"all_even_pungs", "all_pungs", "all_simples"},
			scoredWant: []string{"all_even_pungs", "all_pungs", "all_simples"},
		},
		{
			name:       "full_flush",
			closed:     []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "m2", "m2", "m2", "m5", "m5"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"full_flush", "pure_straight", "no_honors"},
			rawWant:    []string{"full_flush", "pure_straight", "no_honors"},
			scoredWant: []string{"full_flush", "pure_straight"},
			notScored:  []string{"no_honors"},
		},
		{
			name:       "pure_triple_chow",
			closed:     []string{"m1", "m1", "m1", "m2", "m2", "m2", "m3", "m3", "m3", "p1", "p1", "p1", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"pure_triple_chow", "pure_double_chow"},
			rawWant:    []string{"pure_triple_chow", "pure_double_chow"},
			scoredWant: []string{"pure_triple_chow"},
			notScored:  []string{"pure_double_chow"},
		},
		{
			name:       "pure_shifted_pungs",
			closed:     []string{"m1", "m1", "m1", "m2", "m2", "m2", "m3", "m3", "m3", "p1", "p2", "p3", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"pure_shifted_pungs"},
			rawWant:    []string{"pure_shifted_pungs"},
			scoredWant: []string{"pure_shifted_pungs"},
		},
		{
			name:       "upper_tiles",
			closed:     []string{"m7", "m8", "m9", "p7", "p8", "p9", "s7", "s8", "s9", "m7", "m7", "m7", "p8", "p8"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"upper_tiles"},
			rawWant:    []string{"upper_tiles"},
			scoredWant: []string{"upper_tiles"},
		},
		{
			name:       "middle_tiles",
			closed:     []string{"m4", "m5", "m6", "p4", "p5", "p6", "s4", "s5", "s6", "m4", "m4", "m4", "p5", "p5"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"middle_tiles"},
			rawWant:    []string{"middle_tiles"},
			scoredWant: []string{"middle_tiles"},
		},
		{
			name:       "lower_tiles",
			closed:     []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "m1", "m1", "m1", "p2", "p2"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"lower_tiles"},
			rawWant:    []string{"lower_tiles"},
			scoredWant: []string{"lower_tiles"},
		},
		{
			name:       "three_suited_terminal_chows",
			closed:     []string{"m1", "m2", "m3", "p7", "p8", "p9", "s1", "s2", "s3", "s7", "s8", "s9", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"three_suited_terminal_chows"},
			rawWant:    []string{"three_suited_terminal_chows"},
			scoredWant: []string{"three_suited_terminal_chows"},
		},
		{
			name:       "pure_shifted_chows",
			closed:     []string{"m1", "m2", "m3", "m2", "m3", "m4", "m3", "m4", "m5", "p1", "p1", "p1", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"pure_shifted_chows"},
			rawWant:    []string{"pure_shifted_chows"},
			scoredWant: []string{"pure_shifted_chows"},
		},
		{
			name:       "all_fives",
			closed:     []string{"m3", "m4", "m5", "p4", "p5", "p6", "s5", "s6", "s7", "m5", "m5", "m5", "p5", "p5"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"all_fives"},
			rawWant:    []string{"all_fives"},
			scoredWant: []string{"all_fives"},
		},
		{
			name:       "triple_pung",
			closed:     []string{"m2", "m2", "m2", "p2", "p2", "p2", "s2", "s2", "s2", "m4", "m5", "m6", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"triple_pung"},
			rawWant:    []string{"triple_pung"},
			scoredWant: []string{"triple_pung"},
		},
		{
			name:       "lesser_honors_and_knitted_tiles",
			closed:     []string{"m1", "m4", "m7", "p2", "p5", "p8", "s3", "s6", "s9", "z1", "z2", "z3", "z4", "z5"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"lesser_honors_and_knitted_tiles", "knitted_straight"},
			rawWant:    []string{"lesser_honors_and_knitted_tiles", "knitted_straight"},
			scoredWant: []string{"lesser_honors_and_knitted_tiles", "knitted_straight"},
		},
		{
			name:       "upper_four",
			closed:     []string{"m6", "m7", "m8", "p7", "p8", "p9", "s6", "s7", "s8", "m9", "m9", "m9", "p6", "p6"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"upper_four"},
			rawWant:    []string{"upper_four"},
			scoredWant: []string{"upper_four"},
		},
		{
			name:       "lower_four",
			closed:     []string{"m1", "m2", "m3", "p2", "p3", "p4", "s1", "s2", "s3", "m1", "m1", "m1", "p2", "p2"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"lower_four"},
			rawWant:    []string{"lower_four"},
			scoredWant: []string{"lower_four"},
		},
		{
			name:       "mixed_straight",
			closed:     []string{"m1", "m2", "m3", "p4", "p5", "p6", "s7", "s8", "s9", "z1", "z1", "z1", "z2", "z2"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"mixed_straight"},
			rawWant:    []string{"mixed_straight"},
			scoredWant: []string{"mixed_straight"},
		},
		{
			name:       "reversible_tiles",
			closed:     []string{"p2", "p3", "p4", "p2", "p3", "p4", "s5", "s6", "s7", "s5", "s6", "s7", "z7", "z7"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"reversible_tiles"},
			rawWant:    []string{"reversible_tiles"},
			scoredWant: []string{"reversible_tiles"},
		},
		{
			name:       "mixed_triple_chow",
			closed:     []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "z1", "z1", "z1", "z2", "z2"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"mixed_triple_chow", "mixed_double_chow"},
			rawWant:    []string{"mixed_triple_chow", "mixed_double_chow"},
			scoredWant: []string{"mixed_triple_chow", "mixed_double_chow"},
		},
		{
			name:       "mixed_shifted_pungs",
			closed:     []string{"m1", "m1", "m1", "p2", "p2", "p2", "s3", "s3", "s3", "m4", "m5", "m6", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"mixed_shifted_pungs"},
			rawWant:    []string{"mixed_shifted_pungs"},
			scoredWant: []string{"mixed_shifted_pungs"},
		},
		{
			name:   "last_tile_draw",
			closed: []string{"p3", "p4", "p5", "s5", "s6", "s7", "p7", "p8", "p9", "z1", "z1"},
			melds: []mcrFixtureMeld{
				{kind: "chi", tiles: []string{"m1", "m2", "m3"}},
			},
			score:      rules.ScoreContext{IsTsumo: true, IsHaiDi: true, WallRemaining: 50, WinningTile: mustParseTileForTest("p9")},
			primary:    []string{"last_tile_draw", "self_drawn"},
			rawWant:    []string{"last_tile_draw", "self_drawn"},
			scoredWant: []string{"last_tile_draw", "self_drawn"},
		},
		{
			name:       "last_tile_claim",
			closed:     []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "m7", "m8", "m9", "z1", "z1"},
			score:      rules.ScoreContext{IsHaiDi: true, WallRemaining: 50},
			primary:    []string{"last_tile_claim"},
			rawWant:    []string{"last_tile_claim"},
			scoredWant: []string{"last_tile_claim"},
		},
		{
			name:       "out_with_replacement_tile",
			closed:     []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "m7", "m8", "m9", "z1", "z1"},
			score:      rules.ScoreContext{IsGangShangHua: true, WallRemaining: 50},
			primary:    []string{"out_with_replacement_tile"},
			rawWant:    []string{"out_with_replacement_tile"},
			scoredWant: []string{"out_with_replacement_tile"},
		},
		{
			name:       "robbing_the_kong",
			closed:     []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "m7", "m8", "m9", "z1", "z1"},
			score:      rules.ScoreContext{IsGangShangPao: true, WallRemaining: 50},
			primary:    []string{"robbing_the_kong"},
			rawWant:    []string{"robbing_the_kong"},
			scoredWant: []string{"robbing_the_kong"},
		},
		{
			name:       "half_flush",
			closed:     []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "z1", "z1", "z1", "z2", "z2"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"half_flush"},
			rawWant:    []string{"half_flush"},
			scoredWant: []string{"half_flush"},
		},
		{
			name:       "mixed_shifted_chows",
			closed:     []string{"m1", "m2", "m3", "p2", "p3", "p4", "s3", "s4", "s5", "z1", "z1", "z1", "z2", "z2"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"mixed_shifted_chows"},
			rawWant:    []string{"mixed_shifted_chows"},
			scoredWant: []string{"mixed_shifted_chows"},
		},
		{
			name:       "all_types",
			closed:     []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "z1", "z1", "z1", "z5", "z5"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"all_types"},
			rawWant:    []string{"all_types"},
			scoredWant: []string{"all_types"},
		},
		{
			name:   "melded_hand",
			closed: []string{"z1", "z1"},
			melds: []mcrFixtureMeld{
				{kind: "chi", tiles: []string{"m1", "m2", "m3"}},
				{kind: "chi", tiles: []string{"p2", "p3", "p4"}},
				{kind: "chi", tiles: []string{"s3", "s4", "s5"}},
				{kind: "peng", tiles: []string{"z5", "z5", "z5"}},
			},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"melded_hand"},
			rawWant:    []string{"melded_hand"},
			scoredWant: []string{"melded_hand"},
		},
		{
			name:       "outside_hand",
			closed:     []string{"m1", "m2", "m3", "p7", "p8", "p9", "s1", "s1", "s1", "z5", "z5", "z5", "m9", "m9"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"outside_hand"},
			rawWant:    []string{"outside_hand"},
			scoredWant: []string{"outside_hand"},
		},
		{
			name:       "fully_concealed_hand",
			closed:     []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "m7", "m8", "m9", "z1", "z1"},
			score:      rules.ScoreContext{IsTsumo: true, WallRemaining: 50},
			primary:    []string{"fully_concealed_hand", "self_drawn"},
			rawWant:    []string{"fully_concealed_hand", "self_drawn"},
			scoredWant: []string{"fully_concealed_hand"},
			notScored:  []string{"self_drawn"},
		},
		{
			name:       "last_tile",
			closed:     []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "m7", "m8", "m9", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 0, Step: 1},
			primary:    []string{"last_tile"},
			rawWant:    []string{"last_tile"},
			scoredWant: []string{"last_tile"},
		},
		{
			name:       "concealed_hand_all_chows",
			closed:     []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "m7", "m8", "m9", "p5", "p5"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"concealed_hand", "all_chows", "no_honors"},
			rawWant:    []string{"concealed_hand", "all_chows", "no_honors"},
			scoredWant: []string{"concealed_hand", "all_chows"},
			notScored:  []string{"no_honors"},
		},
		{
			name:       "tile_hog",
			closed:     []string{"m1", "m1", "m1", "m1", "m2", "m3", "p4", "p5", "p6", "s7", "s8", "s9", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"tile_hog"},
			rawWant:    []string{"tile_hog"},
			scoredWant: []string{"tile_hog"},
		},
		{
			name:       "double_pung",
			closed:     []string{"m2", "m2", "m2", "p2", "p2", "p2", "s4", "s5", "s6", "m7", "m8", "m9", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"double_pung"},
			rawWant:    []string{"double_pung"},
			scoredWant: nil,
		},
		{
			name:       "one_voided_suit",
			closed:     []string{"m1", "m2", "m3", "m4", "m5", "m6", "p2", "p3", "p4", "p7", "p8", "p9", "m5", "m5"},
			score:      rules.ScoreContext{WallRemaining: 50},
			primary:    []string{"one_voided_suit", "no_honors"},
			rawWant:    []string{"one_voided_suit", "no_honors"},
			scoredWant: []string{"one_voided_suit"},
			notScored:  []string{"no_honors"},
		},
		{
			name:   "waits_and_flower",
			closed: []string{"m1", "m2", "m3", "p2", "p3", "p4", "s2", "s3", "s4", "m5", "m6", "m7", "z1", "z1"},
			score: rules.ScoreContext{
				HuSeat:        0,
				WallRemaining: 50,
				WinningTile:   mustParseTileForTest("m3"),
				SeatGenTiles:  [][]tile.Tile{{mustParseTileForTest("f1")}},
			},
			primary:    []string{"edge_wait", "flower_tile"},
			rawWant:    []string{"edge_wait", "flower_tile"},
			scoredWant: nil,
		},
		{
			name:       "closed_wait",
			closed:     []string{"m1", "m2", "m3", "p2", "p3", "p4", "s2", "s3", "s4", "m5", "m6", "m7", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50, WinningTile: mustParseTileForTest("p3")},
			primary:    []string{"closed_wait"},
			rawWant:    []string{"closed_wait"},
			scoredWant: nil,
		},
		{
			name:       "single_wait",
			closed:     []string{"m1", "m2", "m3", "p2", "p3", "p4", "s2", "s3", "s4", "m5", "m6", "m7", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50, WinningTile: mustParseTileForTest("z1")},
			primary:    []string{"single_wait"},
			rawWant:    []string{"single_wait"},
			scoredWant: nil,
		},
	}
}

func chickenFixture() mcrFanFixture {
	return mcrFanFixture{
		name:   "chicken_hand",
		closed: []string{"p3", "p4", "p5", "s5", "s6", "s7", "p7", "p8", "p9", "z1", "z1"},
		melds: []mcrFixtureMeld{
			{kind: "chi", tiles: []string{"m1", "m2", "m3"}},
		},
		score:      rules.ScoreContext{WallRemaining: 50, WinningTile: mustParseTileForTest("m9")},
		exactTotal: 8,
	}
}

func (fx mcrFanFixture) primaryIDs() []string {
	return fx.primary
}

func (fx mcrFanFixture) rawIDs() []string {
	return fx.rawWant
}

func (fx mcrFanFixture) scoredIDs() []string {
	return fx.scoredWant
}

func fixtureByName(t *testing.T, name string) mcrFanFixture {
	t.Helper()
	for _, fx := range mcrFanFixtures() {
		if fx.name == name {
			return fx
		}
	}
	t.Fatalf("fixture %q not found", name)
	return mcrFanFixture{}
}

func (fx mcrFanFixture) context(t *testing.T) mcrContext {
	t.Helper()
	return newMCRContext(fx.result(t), fx.normalizedScore())
}

func (fx mcrFanFixture) result(t *testing.T) rules.HuResult {
	t.Helper()
	closed := countsFromStrings(t, fx.closed)
	win := closed
	melds := make([]rules.MeldContext, 0, len(fx.melds))
	for _, meld := range fx.melds {
		ctx := rules.MeldContext{Kind: meld.kind, Concealed: meld.concealed}
		for _, raw := range meld.tiles {
			parsed, err := tile.Parse(raw)
			require.NoError(t, err)
			ctx.Tiles = append(ctx.Tiles, parsed)
			if !parsed.IsFlower() {
				win[parsed.Index()]++
			}
		}
		melds = append(melds, ctx)
	}
	return rules.HuResult{Win: win, Closed: closed, OpenMelds: len(melds), Melds: melds}
}

func (fx mcrFanFixture) normalizedScore() rules.ScoreContext {
	sc := fx.score
	if sc.WallRemaining == 0 && sc.Step == 0 {
		sc.WallRemaining = 50
	}
	if sc.WinningTile == 0 {
		if len(fx.closed) > 0 {
			sc.WinningTile = mustParseTileForTest(fx.closed[len(fx.closed)-1])
		} else {
			sc.WinningTile = mustParseTileForTest("z1")
		}
	}
	return sc
}

func rawAwardIDs(awards []mcrAward) map[string]int {
	out := map[string]int{}
	for _, award := range awards {
		out[award.id] += max(1, award.count)
	}
	return out
}

func stringSet(in []string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for _, s := range in {
		out[s] = struct{}{}
	}
	return out
}

func mustParseTileForTest(raw string) tile.Tile {
	t, err := tile.Parse(raw)
	if err != nil {
		panic(fmt.Sprintf("parse test tile %q: %v", raw, err))
	}
	return t
}
