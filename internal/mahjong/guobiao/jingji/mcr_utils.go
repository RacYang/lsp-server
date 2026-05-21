package jingji

import (
	"sort"

	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func (m mcrMeld) suitIsSuited() bool {
	return m.suit == tile.SuitCharacters || m.suit == tile.SuitDots || m.suit == tile.SuitBamboo
}

func sameTile(ts []tile.Tile) bool {
	if len(ts) == 0 {
		return false
	}
	for _, t := range ts[1:] {
		if t != ts[0] {
			return false
		}
	}
	return true
}

func sortInts(v []int) { sort.Ints(v) }

func nextNonZeroCount(c hu.Counts) int {
	for i, n := range c {
		if n > 0 {
			return i
		}
	}
	return -1
}

func rankOfIndex(i int) int { return i%9 + 1 }
func isSuitedTerminal(i int) bool {
	return i < tile.SuitedTileCount && (rankOfIndex(i) == 1 || rankOfIndex(i) == 9)
}
func indexFor(s tile.Suit, rank int) int {
	if s == tile.SuitHonor {
		return tile.SuitedTileCount + rank - 1
	}
	return int(s)*9 + rank - 1
}
func containsInt(v []int, x int) bool {
	for _, n := range v {
		if n == x {
			return true
		}
	}
	return false
}
func allTilesIn(c hu.Counts, allowed map[int]bool) bool {
	for i, n := range c {
		if n > 0 && !allowed[i] {
			return false
		}
	}
	return c.Total() > 0
}
func allTilesMatch(c hu.Counts, pred func(int) bool) bool {
	seen := false
	for i, n := range c {
		if n == 0 {
			continue
		}
		seen = true
		if !pred(i) {
			return false
		}
	}
	return seen
}
func presentSuits(c hu.Counts) []tile.Suit {
	var out []tile.Suit
	for s := 0; s < 3; s++ {
		sum := 0
		for r := 0; r < 9; r++ {
			sum += c[s*9+r]
		}
		if sum > 0 {
			out = append(out, tile.Suit(s))
		}
	}
	return out
}
func hasHonors(c hu.Counts) bool {
	for i := tile.SuitedTileCount; i < tile.PlayableTileCount; i++ {
		if c[i] > 0 {
			return true
		}
	}
	return false
}
func hasWinds(c hu.Counts) bool {
	for i := 27; i <= 30; i++ {
		if c[i] > 0 {
			return true
		}
	}
	return false
}
func hasDragons(c hu.Counts) bool {
	for i := 31; i <= 33; i++ {
		if c[i] > 0 {
			return true
		}
	}
	return false
}
func countHonorKinds(c hu.Counts) int {
	n := 0
	for i := tile.SuitedTileCount; i < tile.PlayableTileCount; i++ {
		if c[i] > 0 {
			n++
		}
	}
	return n
}
func allSevenHonors(c hu.Counts) bool {
	for i := tile.SuitedTileCount; i < tile.PlayableTileCount; i++ {
		if c[i] == 0 {
			return false
		}
	}
	return true
}
func noPairsOrTriples(c hu.Counts) bool {
	for _, n := range c {
		if n > 1 {
			return false
		}
	}
	return true
}
func knittedPatterns() [][]int {
	return [][]int{
		{0, 3, 6, 10, 13, 16, 20, 23, 26},
		{0, 3, 6, 11, 14, 17, 19, 22, 25},
		{1, 4, 7, 9, 12, 15, 20, 23, 26},
		{1, 4, 7, 11, 14, 17, 18, 21, 24},
		{2, 5, 8, 9, 12, 15, 19, 22, 25},
		{2, 5, 8, 10, 13, 16, 18, 21, 24},
	}
}
func knittedPatternCount(c hu.Counts) int {
	max := 0
	for _, pattern := range knittedPatterns() {
		n := 0
		for _, idx := range pattern {
			if c[idx] > 0 {
				n++
			}
		}
		if n > max {
			max = n
		}
	}
	return max
}
func anyDecomp(ctx mcrContext, pred func(mcrDecomp) bool) bool {
	for _, d := range ctx.decomps {
		if pred(d) {
			return true
		}
	}
	return false
}
func hasChow(d mcrDecomp, s tile.Suit, rank int) bool {
	for _, m := range d.melds {
		if m.kind == "chow" && m.suit == s && m.rank == rank {
			return true
		}
	}
	return false
}
func hasPung(d mcrDecomp, s tile.Suit, rank int) bool {
	for _, m := range d.melds {
		if (m.kind == "pung" || m.kind == "kong") && m.suit == s && m.rank == rank {
			return true
		}
	}
	return false
}
func anyPung(ctx mcrContext, s tile.Suit, rank int) bool {
	for _, d := range ctx.decomps {
		if hasPung(d, s, rank) {
			return true
		}
	}
	return false
}
func groupContainsTerminalOrHonor(idx int) bool {
	return idx >= tile.SuitedTileCount || isSuitedTerminal(idx)
}
func meldContainsTerminalOrHonor(m mcrMeld) bool {
	if !m.suitIsSuited() {
		return true
	}
	if m.kind == "chow" {
		return m.rank == 1 || m.rank == 7
	}
	return m.rank == 1 || m.rank == 9
}
func remainingSuit(a, b tile.Suit) tile.Suit {
	for _, s := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
		if s != a && s != b {
			return s
		}
	}
	return tile.SuitCharacters
}
