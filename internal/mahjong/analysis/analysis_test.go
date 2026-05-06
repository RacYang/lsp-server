package analysis

import (
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestShantenRecognizesWinningAndTenpai(t *testing.T) {
	win := hand.FromTiles(mustTiles("m1", "m1", "m1", "m2", "m3", "m4", "p2", "p3", "p4", "s7", "s8", "s9", "m5", "m5"))
	require.Equal(t, -1, Shanten(win.Counts()))

	tenpai := hand.FromTiles(mustTiles("m1", "m1", "m1", "m2", "m3", "m4", "p2", "p3", "p4", "s7", "s8", "s9", "m5"))
	require.Equal(t, 0, Shanten(tenpai.Counts()))
	require.NotEmpty(t, Tenpai(tenpai.Counts()))
}

func TestShantenHandlesSevenPairs(t *testing.T) {
	tenpai := hand.FromTiles(mustTiles("m1", "m1", "m2", "m2", "m3", "m3", "p4", "p4", "p5", "p5", "s6", "s6", "s7"))
	require.Equal(t, 0, Shanten(tenpai.Counts()))
}

func TestBestQueSuitAndExchange(t *testing.T) {
	ts := mustTiles("m1", "m8", "m9", "p2", "p3", "p4", "p7", "s1", "s2", "s3", "s4", "s5", "s6")
	require.Equal(t, tile.SuitCharacters, BestQueSuit(ts))
	ex := BestExchangeThree(ts, tile.SuitCharacters)
	require.Len(t, ex, 3)
	for _, got := range ex {
		require.Equal(t, tile.SuitCharacters, got.Suit())
	}
}

func TestRemainingByPublicAndSafety(t *testing.T) {
	target := tile.Must(tile.SuitCharacters, 5)
	self := hand.FromTiles(mustTiles("m5", "m5", "p1")).Counts()
	pub := PublicInfo{
		DiscardsBySeat: [][]string{{}, {"m5"}, {}, {}},
		MeldsBySeat:    [][]string{{}, {}, {"pong:m5"}, {}},
		DrawnBySeat:    [][]string{{}, {}, {}, {}},
		QueBySeat:      []int32{-1, int32(tile.SuitCharacters), -1, -1},
		SelfSeat:       0,
	}
	require.Zero(t, RemainingByPublic(target, self, pub))
	require.Positive(t, SafetyScore(target, pub))
}

func mustTiles(raws ...string) []tile.Tile {
	out := make([]tile.Tile, 0, len(raws))
	for _, raw := range raws {
		t, err := tile.Parse(raw)
		if err != nil {
			panic(err)
		}
		out = append(out, t)
	}
	return out
}
