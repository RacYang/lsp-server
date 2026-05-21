package jingji

import (
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestMCRScorerDetectsSpecialHands(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		hand []string
		win  string
		want fan.Kind
	}{
		{
			name: "thirteen_orphans",
			hand: []string{"m1", "m9", "p1", "p9", "s1", "s9",
				"z1", "z2", "z3", "z4", "z5", "z6", "z7"},
			win:  "m1",
			want: "mcr_thirteen_orphans",
		},
		{
			name: "seven_shifted_pairs",
			hand: []string{"m1", "m1", "m2", "m2", "m3", "m3", "m4",
				"m4", "m5", "m5", "m6", "m6", "m7"},
			win:  "m7",
			want: "mcr_seven_shifted_pairs",
		},
		{
			name: "greater_honors_and_knitted_tiles",
			hand: []string{"m1", "m4", "m7", "p2", "p5", "p8", "s3",
				"z1", "z2", "z3", "z4", "z5", "z6"},
			win:  "z7",
			want: "mcr_greater_honors_and_knitted_tiles",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			breakdown := scoreHand(t, tc.hand, tc.win, rules.ScoreContext{WallRemaining: 50})
			require.Contains(t, fanKinds(breakdown), tc.want)
		})
	}
}

func TestMCRScorerAppliesHighFanSuppression(t *testing.T) {
	t.Parallel()

	breakdown := scoreHand(t, []string{
		"z1", "z1", "z1",
		"z2", "z2", "z2",
		"z3", "z3", "z3",
		"z4", "z4", "z4",
		"z5",
	}, "z5", rules.ScoreContext{WallRemaining: 50})
	kinds := fanKinds(breakdown)
	require.Contains(t, kinds, fan.Kind("mcr_big_four_winds"))
	require.NotContains(t, kinds, fan.Kind("mcr_big_three_winds"))
	require.NotContains(t, kinds, fan.Kind("mcr_all_pungs"))
}

func TestMCRScorerDetectsWaitShapesFromWinningTile(t *testing.T) {
	t.Parallel()

	winTile := tile.Must(tile.SuitCharacters, 3)
	ctx := newMCRContext(rules.HuResult{
		Win: countsFromStrings(t, []string{
			"m1", "m2", "m3",
			"p2", "p3", "p4",
			"s2", "s3", "s4",
			"m5", "m6", "m7",
			"z1", "z1",
		}),
		Closed: countsFromStrings(t, []string{
			"m1", "m2", "m3",
			"p2", "p3", "p4",
			"s2", "s3", "s4",
			"m5", "m6", "m7",
			"z1", "z1",
		}),
	}, rules.ScoreContext{WinningTile: winTile})
	require.True(t, edgeWait(ctx))
	require.False(t, closedWait(ctx))
}

func scoreHand(t *testing.T, tiles []string, win string, sc rules.ScoreContext) fan.Breakdown {
	t.Helper()
	h := hand.New()
	for _, raw := range tiles {
		parsed, err := tile.Parse(raw)
		require.NoError(t, err)
		h.Add(parsed)
	}
	winTile, err := tile.Parse(win)
	require.NoError(t, err)
	result, ok := testCheckHu(h, winTile, rules.HuContext{})
	require.True(t, ok)
	if sc.WinningTile == 0 {
		sc.WinningTile = winTile
	}
	return testScoreWin(result, sc)
}

func fanKinds(b fan.Breakdown) map[fan.Kind]struct{} {
	out := make(map[fan.Kind]struct{}, len(b.Items))
	for _, item := range b.Items {
		out[item.Kind] = struct{}{}
	}
	return out
}

func countsFromStrings(t *testing.T, tiles []string) hu.Counts {
	t.Helper()
	var out hu.Counts
	for _, raw := range tiles {
		parsed, err := tile.Parse(raw)
		require.NoError(t, err)
		idx := parsed.Index()
		require.GreaterOrEqual(t, idx, 0)
		require.Less(t, idx, tile.PlayableTileCount)
		out[idx]++
	}
	return out
}
