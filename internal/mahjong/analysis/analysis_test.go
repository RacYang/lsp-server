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

func TestAdvanceTiles(t *testing.T) {
	tests := []struct {
		name      string
		tiles     []string
		pub       PublicInfo
		wantTile  string
		wantCount int
		wantIn    bool
	}{
		{
			// 自手 1 张 m9，场上已见 2 张 m9 → 剩余 4-1-2=1
			name:  "听牌手牌进张牌按公开信息减少",
			tiles: []string{"m1", "m2", "m3", "m4", "m5", "m6", "p1", "p2", "p3", "s7", "s8", "s9", "m9"},
			pub: PublicInfo{
				DiscardsBySeat: [][]string{{"m9"}, {"m9"}, {}, {}},
				MeldsBySeat:    [][]string{{}, {}, {}, {}},
				DrawnBySeat:    [][]string{{}, {}, {}, {}},
				QueBySeat:      []int32{-1, -1, -1, -1},
				SelfSeat:       0,
			},
			wantTile:  "m9",
			wantCount: 1,
			wantIn:    true,
		},
		{
			name:  "公开信息中已全出的进张牌不出现",
			tiles: []string{"m1", "m2", "m3", "m4", "m5", "m6", "p1", "p2", "p3", "s7", "s8", "s9", "m9"},
			pub: PublicInfo{
				DiscardsBySeat: [][]string{{"m7"}, {"m7"}, {"m7"}, {"m7"}},
				MeldsBySeat:    [][]string{{}, {}, {}, {}},
				DrawnBySeat:    [][]string{{}, {}, {}, {}},
				QueBySeat:      []int32{-1, -1, -1, -1},
				SelfSeat:       0,
			},
			wantTile:  "m7",
			wantCount: 0,
			wantIn:    false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := hand.FromTiles(mustTiles(tc.tiles...)).Counts()
			got := AdvanceTiles(c, tc.pub)
			target, err := tile.Parse(tc.wantTile)
			require.NoError(t, err)
			if tc.wantIn {
				require.Equal(t, tc.wantCount, got[target])
			} else {
				_, exists := got[target]
				require.False(t, exists)
			}
		})
	}
}

func TestDiscardOptions(t *testing.T) {
	emptyPub := PublicInfo{
		DiscardsBySeat: [][]string{{}, {}, {}, {}},
		MeldsBySeat:    [][]string{{}, {}, {}, {}},
		DrawnBySeat:    [][]string{{}, {}, {}, {}},
		QueBySeat:      []int32{-1, -1, -1, -1},
		SelfSeat:       0,
	}
	tests := []struct {
		name        string
		tiles       []string
		que         tile.Suit
		pub         PublicInfo
		wantNonZero bool
		wantFirst   string
	}{
		{
			name:        "非听牌手牌返回候选列表非空",
			tiles:       []string{"m1", "m9", "p1", "p9", "s1", "s9", "m2", "p2", "s2", "m3", "p3", "s3", "m4"},
			que:         tile.SuitCharacters,
			pub:         emptyPub,
			wantNonZero: true,
		},
		{
			name:        "que 花色牌优先排在首位",
			tiles:       []string{"m1", "m2", "m3", "p2", "p3", "p4", "s1", "s2", "s3", "m9", "p9", "s9", "m5"},
			que:         tile.SuitCharacters,
			pub:         emptyPub,
			wantNonZero: true,
			wantFirst:   "m",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := mustTiles(tc.tiles...)
			opts := DiscardOptions(ts, tc.que, tc.pub)
			if tc.wantNonZero {
				require.NotEmpty(t, opts)
			}
			if tc.wantFirst != "" {
				first := opts[0]
				require.Equal(t, tc.que, first.Tile.Suit())
			}
		})
	}
}

func TestBestDiscard(t *testing.T) {
	emptyPub := PublicInfo{
		DiscardsBySeat: [][]string{{}, {}, {}, {}},
		MeldsBySeat:    [][]string{{}, {}, {}, {}},
		DrawnBySeat:    [][]string{{}, {}, {}, {}},
		QueBySeat:      []int32{-1, -1, -1, -1},
		SelfSeat:       0,
	}
	t.Run("与 DiscardOptions 首元素一致", func(t *testing.T) {
		ts := mustTiles("m1", "m9", "p1", "p9", "s1", "s9", "m2", "p2", "s2", "m3", "p3", "s3", "m4")
		best := BestDiscard(ts, tile.SuitCharacters, emptyPub)
		opts := DiscardOptions(ts, tile.SuitCharacters, emptyPub)
		require.Equal(t, opts[0], best)
	})
	t.Run("空手牌返回零值", func(t *testing.T) {
		best := BestDiscard(nil, tile.SuitCharacters, emptyPub)
		require.Equal(t, DiscardOption{}, best)
	})
}

func TestBestExchangeThreeEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		tiles    []string
		que      tile.Suit
		wantLen  int
		wantSuit tile.Suit
	}{
		{
			name:    "手牌不足三张原样返回",
			tiles:   []string{"m1", "m2"},
			que:     tile.SuitCharacters,
			wantLen: 2,
		},
		{
			name:    "que 花色牌不足三张时降级到牌数最多的花色",
			tiles:   []string{"m1", "p2", "p3", "p4", "p5", "s6", "s7", "s8", "s9", "m2", "m3", "m4", "m5"},
			que:     tile.SuitBamboo,
			wantLen: 3,
		},
		{
			name:    "候选超过三张截断",
			tiles:   []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "p1", "p2", "s1", "s2"},
			que:     tile.SuitCharacters,
			wantLen: 3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BestExchangeThree(mustTiles(tc.tiles...), tc.que)
			require.Len(t, got, tc.wantLen)
		})
	}
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
