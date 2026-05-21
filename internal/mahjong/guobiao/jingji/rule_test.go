package jingji

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

func testCapabilities() rules.CapabilitySet {
	return rules.CapabilitiesOf(rules.MustGet(ID))
}

func testBuildWall(seed int64) *wall.Wall {
	return testCapabilities().TileSet.BuildWall(context.Background(), seed)
}

func testCheckHu(h *hand.Hand, target tile.Tile, hc rules.HuContext) (rules.HuResult, bool) {
	return testCapabilities().Win.CheckHu(h, target, hc)
}

func testScoreWin(result rules.HuResult, sc rules.ScoreContext) fan.Breakdown {
	breakdown, _, _ := testCapabilities().Scoring.ScoreWin(result, sc)
	return breakdown
}

func TestBuildWallUsesFull144TileSet(t *testing.T) {
	t.Parallel()

	w := testBuildWall(1)
	require.Equal(t, 144, w.Remaining())

	seenHonors := false
	seenFlowers := false
	for _, t := range w.Tiles() {
		seenHonors = seenHonors || t.IsHonor()
		seenFlowers = seenFlowers || t.IsFlower()
	}
	require.True(t, seenHonors)
	require.True(t, seenFlowers)
}

func TestCheckHuSupportsHonorTriplets(t *testing.T) {
	t.Parallel()

	h := hand.FromTiles([]tile.Tile{
		tile.Must(tile.SuitHonor, 1),
		tile.Must(tile.SuitHonor, 2), tile.Must(tile.SuitHonor, 2), tile.Must(tile.SuitHonor, 2),
		tile.Must(tile.SuitHonor, 3), tile.Must(tile.SuitHonor, 3), tile.Must(tile.SuitHonor, 3),
		tile.Must(tile.SuitHonor, 4), tile.Must(tile.SuitHonor, 4), tile.Must(tile.SuitHonor, 4),
		tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3),
	})
	result, ok := testCheckHu(h, tile.Must(tile.SuitHonor, 1), rules.HuContext{})
	require.True(t, ok)

	breakdown := testScoreWin(result, rules.ScoreContext{IsTsumo: true, IsHaiDi: true})
	require.GreaterOrEqual(t, breakdown.Total, 8)
}

func TestScoreWinRejectsBelowEightWithoutChickenFallback(t *testing.T) {
	t.Parallel()

	h := hand.FromTiles([]tile.Tile{
		tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3),
		tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 5), tile.Must(tile.SuitCharacters, 6),
		tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 3), tile.Must(tile.SuitDots, 4),
		tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitBamboo, 3), tile.Must(tile.SuitBamboo, 4),
		tile.Must(tile.SuitHonor, 1),
	})
	result, ok := testCheckHu(h, tile.Must(tile.SuitHonor, 1), rules.HuContext{})
	require.True(t, ok)

	breakdown := testScoreWin(result, rules.ScoreContext{WallRemaining: 50})
	require.Zero(t, breakdown.Total)
	require.Empty(t, breakdown.Items)
}
