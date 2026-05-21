package jingji

import (
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/rules"
)

func TestMCRSuppressionGraphDocumentsGreenBookPrinciples(t *testing.T) {
	t.Parallel()

	rules := mcrSuppressionRules()
	assertSuppressionLinks(t, rules, "big_four_winds", "big_three_winds", "seat_wind", "prevalent_wind", "pung_of_terminals_or_honors", "all_pungs")
	assertSuppressionLinks(t, rules, "little_four_winds", "big_three_winds", "seat_wind", "prevalent_wind", "pung_of_terminals_or_honors")
	assertSuppressionLinks(t, rules, "big_three_dragons", "two_dragon_pungs", "dragon_pung", "pung_of_terminals_or_honors")
	assertSuppressionLinks(t, rules, "little_three_dragons", "two_dragon_pungs", "dragon_pung", "pung_of_terminals_or_honors")
	assertSuppressionLinks(t, rules, "all_honors", "all_terminals_and_honors", "pung_of_terminals_or_honors")
	assertSuppressionLinks(t, rules, "four_concealed_pungs", "three_concealed_pungs", "two_concealed_pungs")
	assertSuppressionLinks(t, rules, "nine_gates", "full_flush", "one_voided_suit", "no_honors", "concealed_hand")
	assertSuppressionLinks(t, rules, "seven_shifted_pairs", "seven_pairs", "full_flush", "one_voided_suit", "no_honors")
	assertSuppressionLinks(t, rules, "full_flush", "half_flush", "one_voided_suit", "no_honors")
	assertSuppressionLinks(t, rules, "quadruple_chow", "seven_pairs", "pure_triple_chow", "pure_shifted_pungs", "pure_double_chow", "mixed_double_chow", "tile_hog")
	assertSuppressionLinks(t, rules, "four_pure_shifted_chows", "pure_shifted_chows", "short_straight")
}

func TestMCRSemanticFixturesScoreGreenBookPrinciples(t *testing.T) {
	t.Parallel()

	cases := []mcrFanFixture{
		{
			name:       "semantic_big_four_winds",
			closed:     []string{"z1", "z1", "z1", "z2", "z2", "z2", "z3", "z3", "z3", "z4", "z4", "z4", "z5", "z5"},
			score:      rules.ScoreContext{HuSeat: 0, WallRemaining: 50},
			rawWant:    []string{"big_four_winds", "all_honors", "big_three_winds", "all_pungs", "prevalent_wind", "seat_wind", "pung_of_terminals_or_honors"},
			scoredWant: []string{"big_four_winds", "all_honors"},
			notScored:  []string{"all_terminals_and_honors", "big_three_winds", "all_pungs", "three_concealed_pungs", "two_concealed_pungs", "prevalent_wind", "seat_wind", "pung_of_terminals_or_honors"},
			minTotal:   152,
		},
		{
			name:       "semantic_big_three_dragons",
			closed:     []string{"z5", "z5", "z5", "z6", "z6", "z6", "z7", "z7", "z7", "m1", "m2", "m3", "p1", "p1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			rawWant:    []string{"big_three_dragons", "two_dragon_pungs", "dragon_pung"},
			scoredWant: []string{"big_three_dragons", "outside_hand", "concealed_hand", "one_voided_suit", "single_wait"},
			notScored:  []string{"two_dragon_pungs", "dragon_pung", "pung_of_terminals_or_honors"},
			exactTotal: 96,
		},
		{
			name:       "semantic_little_four_winds",
			closed:     []string{"z1", "z1", "z1", "z2", "z2", "z2", "z3", "z3", "z3", "z4", "z4", "m1", "m2", "m3"},
			score:      rules.ScoreContext{WallRemaining: 50},
			rawWant:    []string{"little_four_winds", "big_three_winds"},
			scoredWant: []string{"little_four_winds"},
			notScored:  []string{"big_three_winds", "pung_of_terminals_or_honors"},
		},
		{
			name:       "semantic_little_three_dragons",
			closed:     []string{"z5", "z5", "z5", "z6", "z6", "z6", "z7", "z7", "m1", "m2", "m3", "p1", "p2", "p3"},
			score:      rules.ScoreContext{WallRemaining: 50},
			rawWant:    []string{"little_three_dragons", "two_dragon_pungs", "dragon_pung"},
			scoredWant: []string{"little_three_dragons"},
			notScored:  []string{"two_dragon_pungs", "dragon_pung", "pung_of_terminals_or_honors"},
		},
		{
			name:       "semantic_nine_gates",
			closed:     []string{"m1", "m1", "m1", "m2", "m3", "m4", "m5", "m5", "m6", "m7", "m8", "m9", "m9", "m9"},
			score:      rules.ScoreContext{WallRemaining: 50},
			rawWant:    []string{"nine_gates", "full_flush"},
			scoredWant: []string{"nine_gates"},
			notScored:  []string{"full_flush", "one_voided_suit", "no_honors", "concealed_hand"},
			exactTotal: 88,
		},
		{
			name:       "semantic_seven_shifted_pairs",
			closed:     []string{"m1", "m1", "m2", "m2", "m3", "m3", "m4", "m4", "m5", "m5", "m6", "m6", "m7", "m7"},
			score:      rules.ScoreContext{WallRemaining: 50},
			rawWant:    []string{"seven_shifted_pairs", "seven_pairs"},
			scoredWant: []string{"seven_shifted_pairs"},
			notScored:  []string{"seven_pairs", "full_flush", "one_voided_suit", "no_honors", "concealed_hand"},
			exactTotal: 88,
		},
		{
			name:       "semantic_full_flush_takes_highest_flush_family",
			closed:     []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "m2", "m2", "m2", "m5", "m5"},
			score:      rules.ScoreContext{WallRemaining: 50},
			rawWant:    []string{"full_flush", "pure_straight", "no_honors"},
			scoredWant: []string{"full_flush", "pure_straight"},
			notScored:  []string{"half_flush", "one_voided_suit", "no_honors"},
			minTotal:   40,
		},
		{
			name:       "semantic_quadruple_chow_counts_once",
			closed:     []string{"m1", "m1", "m1", "m1", "m2", "m2", "m2", "m2", "m3", "m3", "m3", "m3", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			rawWant:    []string{"quadruple_chow", "pure_triple_chow", "pure_double_chow"},
			scoredWant: []string{"quadruple_chow"},
			notScored:  []string{"seven_pairs", "pure_triple_chow", "pure_shifted_pungs", "three_concealed_pungs", "two_concealed_pungs", "pure_double_chow", "mixed_double_chow", "tile_hog", "pung_of_terminals_or_honors"},
			minTotal:   48,
		},
		{
			name:       "semantic_four_pure_shifted_chows_counts_once",
			closed:     []string{"m1", "m2", "m3", "m2", "m3", "m4", "m3", "m4", "m5", "m4", "m5", "m6", "z1", "z1"},
			score:      rules.ScoreContext{WallRemaining: 50},
			rawWant:    []string{"four_pure_shifted_chows", "pure_shifted_chows", "short_straight"},
			scoredWant: []string{"four_pure_shifted_chows"},
			notScored:  []string{"pure_shifted_chows", "short_straight"},
			minTotal:   32,
		},
		{
			name:   "semantic_four_kongs_keeps_kong_components",
			closed: []string{"z1", "z1"},
			melds: []mcrFixtureMeld{
				{kind: "ming_gang", tiles: []string{"m1", "m1", "m1", "m1"}},
				{kind: "ming_gang", tiles: []string{"p2", "p2", "p2", "p2"}},
				{kind: "an_gang", tiles: []string{"s3", "s3", "s3", "s3"}, concealed: true},
				{kind: "an_gang", tiles: []string{"z5", "z5", "z5", "z5"}, concealed: true},
			},
			score:      rules.ScoreContext{HuSeat: 0, WallRemaining: 50},
			rawWant:    []string{"four_kongs", "three_kongs", "two_concealed_kongs", "two_melded_kongs", "concealed_kong", "melded_kong"},
			scoredWant: []string{"four_kongs", "three_kongs", "two_concealed_kongs", "two_melded_kongs", "concealed_kong", "melded_kong"},
			minTotal:   128,
		},
		{
			name:   "semantic_last_tile_draw_keeps_self_drawn",
			closed: []string{"p3", "p4", "p5", "s5", "s6", "s7", "p7", "p8", "p9", "z1", "z1"},
			melds: []mcrFixtureMeld{
				{kind: "chi", tiles: []string{"m1", "m2", "m3"}},
			},
			score:      rules.ScoreContext{IsTsumo: true, IsHaiDi: true, WallRemaining: 50, WinningTile: mustParseTileForTest("p9")},
			rawWant:    []string{"last_tile_draw", "self_drawn"},
			scoredWant: []string{"last_tile_draw", "self_drawn"},
			exactTotal: 9,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertMCRSemanticFixture(t, tc)
		})
	}
}

func TestMCRSemanticFixtureKeepsChickenHandStrict(t *testing.T) {
	t.Parallel()

	chicken := chickenFixture()
	assertMCRSemanticFixture(t, mcrFanFixture{
		name:       "semantic_chicken_hand",
		closed:     chicken.closed,
		melds:      chicken.melds,
		score:      chicken.score,
		scoredWant: []string{"chicken_hand"},
		exactTotal: 8,
	})

	lowOtherFan := mcrFanFixture{
		name:   "semantic_low_other_fan_is_not_chicken",
		closed: []string{"m1", "m2", "m3", "m4", "m5", "m6", "p2", "p3", "p4", "s2", "s3", "s4", "z1", "z1"},
		score:  rules.ScoreContext{WallRemaining: 50, WinningTile: mustParseTileForTest("z1")},
	}
	breakdown := scoreMCR(lowOtherFan.result(t), lowOtherFan.normalizedScore())
	require.Zero(t, breakdown.Total)
	require.Empty(t, breakdown.Items)
}

func assertMCRSemanticFixture(t *testing.T, fx mcrFanFixture) {
	t.Helper()

	raw := rawAwardIDs(detectMCRFans(fx.context(t)))
	for _, id := range fx.rawWant {
		require.Contains(t, raw, id, "%s should detect raw fan %s", fx.name, id)
	}
	for _, id := range fx.notRaw {
		require.NotContains(t, raw, id, "%s should not detect raw fan %s", fx.name, id)
	}

	breakdown := scoreMCR(fx.result(t), fx.normalizedScore())
	kinds := fanKinds(breakdown)
	for _, id := range fx.scoredWant {
		require.Contains(t, kinds, fan.Kind("mcr_"+id), "%s should score fan %s", fx.name, id)
	}
	for _, id := range fx.notScored {
		require.NotContains(t, kinds, fan.Kind("mcr_"+id), "%s should suppress fan %s", fx.name, id)
	}
	if fx.exactTotal > 0 {
		require.Equal(t, fx.exactTotal, breakdown.Total, "%s total: %+v", fx.name, breakdown.Items)
	}
	if fx.minTotal > 0 {
		require.GreaterOrEqual(t, breakdown.Total, fx.minTotal, "%s total: %+v", fx.name, breakdown.Items)
	}
}

func assertSuppressionLinks(t *testing.T, rules map[string][]string, high string, lows ...string) {
	t.Helper()
	for _, low := range lows {
		require.Contains(t, rules[high], low, "%s should suppress %s", high, low)
	}
}
