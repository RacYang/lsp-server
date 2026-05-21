package jingji

import (
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func mcrMeldFromContext(m rules.MeldContext) (mcrMeld, bool) {
	tiles := make([]tile.Tile, 0, len(m.Tiles))
	for _, t := range m.Tiles {
		if t != 0 && !t.IsFlower() {
			tiles = append(tiles, t)
		}
	}
	if len(tiles) < 3 {
		return mcrMeld{}, false
	}
	out := mcrMeld{tiles: append([]tile.Tile(nil), tiles...), concealed: m.Concealed}
	if sameTile(tiles) {
		out.kind = "pung"
		if len(tiles) >= 4 || m.Kind == "an_gang" || m.Kind == "bu_gang" || m.Kind == "ming_gang" || m.Kind == "gang" {
			out.kind = "kong"
		}
		out.suit = tiles[0].Suit()
		out.rank = tiles[0].Rank()
		return out, true
	}
	if tiles[0].IsSuited() {
		ranks := []int{tiles[0].Rank(), tiles[1].Rank(), tiles[2].Rank()}
		sortInts(ranks)
		if tiles[1].Suit() == tiles[0].Suit() && tiles[2].Suit() == tiles[0].Suit() &&
			ranks[1] == ranks[0]+1 && ranks[2] == ranks[0]+2 {
			out.kind = "chow"
			out.suit = tiles[0].Suit()
			out.rank = ranks[0]
			return out, true
		}
	}
	return out, false
}

func enumerateMCRDecomps(closed hu.Counts, open []mcrMeld) []mcrDecomp {
	openMelds := len(open)
	need := 4 - openMelds
	if need < 0 {
		return nil
	}
	if closed.Total() != 2+need*3 {
		return nil
	}
	out := []mcrDecomp{}
	for pair := 0; pair < tile.PlayableTileCount; pair++ {
		if closed[pair] < 2 {
			continue
		}
		rest := closed
		rest[pair] -= 2
		for _, melds := range enumerateMelds(rest, need) {
			all := append([]mcrMeld(nil), open...)
			all = append(all, melds...)
			out = append(out, mcrDecomp{pair: pair, melds: all})
		}
	}
	return out
}

func enumerateMelds(c hu.Counts, need int) [][]mcrMeld {
	if need == 0 {
		if c.Total() == 0 {
			return [][]mcrMeld{{}}
		}
		return nil
	}
	i := nextNonZeroCount(c)
	if i < 0 {
		return nil
	}
	out := [][]mcrMeld{}
	if c[i] >= 3 {
		rest := c
		rest[i] -= 3
		t, _ := tile.FromIndex(i)
		m := mcrMeld{kind: "pung", suit: t.Suit(), rank: t.Rank(), tiles: []tile.Tile{t, t, t}, concealed: true}
		for _, tail := range enumerateMelds(rest, need-1) {
			out = append(out, append([]mcrMeld{m}, tail...))
		}
	}
	if i < tile.SuitedTileCount {
		suit := i / 9
		r := i % 9
		base := suit * 9
		for start := r - 2; start <= r; start++ {
			if start < 0 || start > 6 {
				continue
			}
			a, b, cc := base+start, base+start+1, base+start+2
			if c[a] > 0 && c[b] > 0 && c[cc] > 0 {
				rest := c
				rest[a]--
				rest[b]--
				rest[cc]--
				t, _ := tile.FromIndex(a)
				m := mcrMeld{kind: "chow", suit: t.Suit(), rank: start + 1, tiles: []tile.Tile{t, tile.Must(t.Suit(), start+2), tile.Must(t.Suit(), start+3)}, concealed: true}
				for _, tail := range enumerateMelds(rest, need-1) {
					out = append(out, append([]mcrMeld{m}, tail...))
				}
			}
		}
	}
	return out
}

func allGreen(c hu.Counts) bool {
	allowed := map[int]bool{19: true, 20: true, 21: true, 23: true, 25: true, 32: true}
	return allTilesIn(c, allowed)
}

func nineGates(c hu.Counts) bool {
	suits := presentSuits(c)
	if len(suits) != 1 || hasHonors(c) {
		return false
	}
	base := int(suits[0]) * 9
	need := []int{3, 1, 1, 1, 1, 1, 1, 1, 3}
	extra := false
	for i, n := range need {
		got := c[base+i]
		if got < n {
			return false
		}
		if got > n {
			extra = true
		}
	}
	return extra
}

func sevenShiftedPairs(c hu.Counts) bool {
	if !hu.SevenPairs(c) || hasHonors(c) {
		return false
	}
	for s := 0; s < 3; s++ {
		base := s * 9
		for start := 0; start <= 2; start++ {
			ok := true
			for r := 0; r < 7; r++ {
				if c[base+start+r] != 2 {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
	}
	return false
}

func thirteenOrphans(c hu.Counts) bool {
	req := []int{0, 8, 9, 17, 18, 26, 27, 28, 29, 30, 31, 32, 33}
	pair := false
	for _, idx := range req {
		if c[idx] == 0 {
			return false
		}
		if c[idx] >= 2 {
			pair = true
		}
	}
	for i, n := range c {
		if n > 0 && !containsInt(req, i) {
			return false
		}
	}
	return pair
}

func allTerminals(c hu.Counts) bool {
	return allTilesMatch(c, isSuitedTerminal)
}
func allHonors(c hu.Counts) bool {
	return allTilesMatch(c, func(i int) bool { return i >= tile.SuitedTileCount })
}
func allTerminalsAndHonors(c hu.Counts) bool {
	return allTilesMatch(c, func(i int) bool { return i >= tile.SuitedTileCount || isSuitedTerminal(i) })
}

func pureTerminalChows(ctx mcrContext) bool {
	for _, d := range ctx.decomps {
		suit := tile.Suit(255)
		chow16, chow79 := 0, 0
		ok := d.pair >= 0 && d.pair < tile.SuitedTileCount && (d.pair%9 == 4)
		for _, m := range d.melds {
			if m.kind != "chow" {
				ok = false
				break
			}
			if suit == tile.Suit(255) {
				suit = m.suit
			}
			if m.suit != suit {
				ok = false
				break
			}
			switch m.rank {
			case 1:
				chow16++
			case 7:
				chow79++
			}
		}
		if ok && chow16 == 2 && chow79 == 2 {
			return true
		}
	}
	return false
}

func greaterHonorsAndKnitted(c hu.Counts) bool {
	return c.Total() == 14 && noPairsOrTriples(c) && allSevenHonors(c) && knittedPatternCount(c) >= 7
}

func lesserHonorsAndKnitted(c hu.Counts) bool {
	return c.Total() == 14 && noPairsOrTriples(c) && knittedPatternCount(c)+countHonorKinds(c) == 14 && !greaterHonorsAndKnitted(c)
}

func knittedStraight(c hu.Counts) bool {
	for _, p := range knittedPatterns() {
		ok := true
		for _, idx := range p {
			if c[idx] == 0 {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func allEvenPungs(ctx mcrContext) bool {
	return anyDecomp(ctx, func(d mcrDecomp) bool {
		if d.pair < 0 || d.pair >= tile.SuitedTileCount || (d.pair%9+1)%2 != 0 {
			return false
		}
		for _, m := range d.melds {
			if (m.kind != "pung" && m.kind != "kong") || !m.suitIsSuited() || m.rank%2 != 0 {
				return false
			}
		}
		return true
	})
}

func fullFlushCounts(c hu.Counts) bool { return len(presentSuits(c)) == 1 && !hasHonors(c) }
func halfFlushCounts(c hu.Counts) bool { return len(presentSuits(c)) == 1 && hasHonors(c) }
func allRanksInRange(c hu.Counts, lo, hi int) bool {
	return allTilesMatch(c, func(i int) bool { return i < tile.SuitedTileCount && rankOfIndex(i) >= lo && rankOfIndex(i) <= hi })
}
func allFives(ctx mcrContext) bool {
	return anyDecomp(ctx, func(d mcrDecomp) bool {
		if d.pair < 0 || d.pair >= tile.SuitedTileCount || rankOfIndex(d.pair) != 5 {
			return false
		}
		for _, m := range d.melds {
			if !m.suitIsSuited() {
				return false
			}
			switch m.kind {
			case "chow":
				if m.rank > 5 || m.rank+2 < 5 {
					return false
				}
			case "pung", "kong":
				if m.rank != 5 {
					return false
				}
			default:
				return false
			}
		}
		return true
	})
}
func allSimplesCounts(c hu.Counts) bool {
	return allTilesMatch(c, func(i int) bool { return i < tile.SuitedTileCount && rankOfIndex(i) >= 2 && rankOfIndex(i) <= 8 })
}

func allPungsMCR(ctx mcrContext) bool {
	return anyDecomp(ctx, func(d mcrDecomp) bool {
		for _, m := range d.melds {
			if m.kind != "pung" && m.kind != "kong" {
				return false
			}
		}
		return true
	})
}

func allTypes(c hu.Counts) bool              { return len(presentSuits(c)) == 3 && hasWinds(c) && hasDragons(c) }
func meldedHand(ctx mcrContext) bool         { return len(ctx.melds) >= 4 && !ctx.scoreContext.IsTsumo }
func fullyConcealedHand(ctx mcrContext) bool { return len(ctx.melds) == 0 && ctx.scoreContext.IsTsumo }
func concealedHand(ctx mcrContext) bool      { return len(ctx.melds) == 0 && !ctx.scoreContext.IsTsumo }
func oneVoidedSuit(c hu.Counts) bool         { return len(presentSuits(c)) == 2 }
func noHonors(c hu.Counts) bool              { return !hasHonors(c) }

func allChows(ctx mcrContext) bool {
	return anyDecomp(ctx, func(d mcrDecomp) bool {
		if d.pair >= tile.SuitedTileCount {
			return false
		}
		for _, m := range d.melds {
			if m.kind != "chow" {
				return false
			}
		}
		return true
	})
}

func outsideHand(ctx mcrContext) bool {
	return anyDecomp(ctx, func(d mcrDecomp) bool {
		if !groupContainsTerminalOrHonor(d.pair) {
			return false
		}
		for _, m := range d.melds {
			if !meldContainsTerminalOrHonor(m) {
				return false
			}
		}
		return true
	})
}

func reversibleTiles(c hu.Counts) bool {
	allowed := map[int]bool{10: true, 11: true, 12: true, 14: true, 16: true, 17: true, 18: true, 20: true, 22: true, 23: true, 24: true, 26: true, 33: true}
	return allTilesIn(c, allowed)
}

func edgeWait(ctx mcrContext) bool {
	idx := ctx.scoreContext.WinningTile.Index()
	if idx < 0 || idx >= tile.SuitedTileCount {
		return false
	}
	rank := rankOfIndex(idx)
	for _, d := range ctx.decomps {
		for _, m := range d.melds {
			if m.kind != "chow" || m.suit != ctx.scoreContext.WinningTile.Suit() {
				continue
			}
			if m.rank == 1 && rank == 3 {
				return true
			}
			if m.rank == 7 && rank == 7 {
				return true
			}
		}
	}
	return false
}

func closedWait(ctx mcrContext) bool {
	idx := ctx.scoreContext.WinningTile.Index()
	if idx < 0 || idx >= tile.SuitedTileCount {
		return false
	}
	rank := rankOfIndex(idx)
	for _, d := range ctx.decomps {
		for _, m := range d.melds {
			if m.kind == "chow" && m.suit == ctx.scoreContext.WinningTile.Suit() && m.rank+1 == rank {
				return true
			}
		}
	}
	return false
}

func singleWait(ctx mcrContext) bool {
	idx := ctx.scoreContext.WinningTile.Index()
	if idx < 0 || idx >= tile.PlayableTileCount {
		return false
	}
	for _, d := range ctx.decomps {
		if d.pair == idx {
			return true
		}
	}
	return false
}
