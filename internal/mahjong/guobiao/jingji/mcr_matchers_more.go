package jingji

import (
	"sort"
	"strconv"

	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func countWindPungs(ctx mcrContext) int   { return countHonorPungsInRange(ctx, 27, 30) }
func countDragonPungs(ctx mcrContext) int { return countHonorPungsInRange(ctx, 31, 33) }
func countHonorPungsInRange(ctx mcrContext, lo, hi int) int {
	n := 0
	for idx := lo; idx <= hi; idx++ {
		if countAsPung(ctx, idx) {
			n++
		}
	}
	return n
}
func countSpecificHonorPungs(ctx mcrContext, idx int) int {
	if countAsPung(ctx, idx) {
		return 1
	}
	return 0
}
func countAsPung(ctx mcrContext, idx int) bool {
	if ctx.win[idx] >= 3 {
		return true
	}
	t, _ := tile.FromIndex(idx)
	for _, m := range ctx.melds {
		if (m.kind == "pung" || m.kind == "kong") && len(m.tiles) > 0 && m.tiles[0] == t {
			return true
		}
	}
	return false
}
func hasWindPair(ctx mcrContext) bool   { return hasPairInRange(ctx, 27, 30) }
func hasDragonPair(ctx mcrContext) bool { return hasPairInRange(ctx, 31, 33) }
func hasPairInRange(ctx mcrContext, lo, hi int) bool {
	for i := lo; i <= hi; i++ {
		if ctx.win[i] >= 2 {
			return true
		}
	}
	return false
}

func countKongs(ctx mcrContext) int {
	n := 0
	for _, m := range ctx.melds {
		if m.kind == "kong" {
			n++
		}
	}
	for _, record := range ctx.scoreContext.GangRecords {
		if record.Seat == ctx.scoreContext.HuSeat {
			n++
		}
	}
	return n
}
func countConcealedKongs(ctx mcrContext) int {
	n := 0
	for _, m := range ctx.melds {
		if m.kind == "kong" && m.concealed {
			n++
		}
	}
	for _, record := range ctx.scoreContext.GangRecords {
		if record.Seat == ctx.scoreContext.HuSeat && record.Kind == "an" {
			n++
		}
	}
	return n
}
func countMeldedKongs(ctx mcrContext) int {
	n := countKongs(ctx) - countConcealedKongs(ctx)
	if n < 0 {
		return 0
	}
	return n
}

func countConcealedPungs(ctx mcrContext) int {
	max := 0
	for _, d := range ctx.decomps {
		n := 0
		for _, m := range d.melds {
			if (m.kind == "pung" || m.kind == "kong") && m.concealed {
				n++
			}
		}
		if n > max {
			max = n
		}
	}
	return max
}

func pureShiftedPungs(ctx mcrContext, need int) bool {
	for _, d := range ctx.decomps {
		bySuit := map[tile.Suit][]int{}
		for _, m := range d.melds {
			if (m.kind == "pung" || m.kind == "kong") && m.suitIsSuited() {
				bySuit[m.suit] = append(bySuit[m.suit], m.rank)
			}
		}
		for _, ranks := range bySuit {
			if hasShiftedRanks(ranks, need) {
				return true
			}
		}
	}
	return false
}
func pureShiftedChows(ctx mcrContext, need int) bool {
	for _, d := range ctx.decomps {
		bySuit := map[tile.Suit][]int{}
		for _, m := range d.melds {
			if m.kind == "chow" {
				bySuit[m.suit] = append(bySuit[m.suit], m.rank)
			}
		}
		for _, ranks := range bySuit {
			if hasShiftedRanks(ranks, need) {
				return true
			}
		}
	}
	return false
}
func hasShiftedRanks(ranks []int, need int) bool {
	sort.Ints(ranks)
	counts := map[int]int{}
	for _, r := range ranks {
		counts[r]++
	}
	for start := 1; start <= 9; start++ {
		ok := true
		for i := 0; i < need; i++ {
			if counts[start+i] == 0 {
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

func maxIdenticalChows(ctx mcrContext, sameSuit bool) int {
	max := 0
	for _, d := range ctx.decomps {
		counts := map[string]int{}
		for _, m := range d.melds {
			if m.kind != "chow" {
				continue
			}
			key := strconv.Itoa(m.rank)
			if sameSuit {
				key = strconv.Itoa(int(m.suit)) + ":" + key
			}
			counts[key]++
			if counts[key] > max {
				max = counts[key]
			}
		}
	}
	return max
}
func pureStraight(ctx mcrContext) bool  { return straightWithSuits(ctx, true) }
func mixedStraight(ctx mcrContext) bool { return straightWithSuits(ctx, false) }
func straightWithSuits(ctx mcrContext, sameSuit bool) bool {
	for _, d := range ctx.decomps {
		if sameSuit {
			for _, s := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
				if hasChow(d, s, 1) && hasChow(d, s, 4) && hasChow(d, s, 7) {
					return true
				}
			}
			continue
		}
		for _, a := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
			for _, b := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
				for _, c := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
					if a != b && a != c && b != c && hasChow(d, a, 1) && hasChow(d, b, 4) && hasChow(d, c, 7) {
						return true
					}
				}
			}
		}
	}
	return false
}
func threeSuitedTerminalChows(ctx mcrContext) bool {
	for _, d := range ctx.decomps {
		for _, s1 := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
			for _, s2 := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
				if s1 == s2 {
					continue
				}
				s3 := remainingSuit(s1, s2)
				if hasChow(d, s1, 1) && hasChow(d, s2, 7) && hasChow(d, s3, 1) && hasChow(d, s3, 7) {
					return true
				}
			}
		}
	}
	return false
}
func triplePung(ctx mcrContext) bool        { return nSuitSameRankPung(ctx, 3) }
func mixedTripleChow(ctx mcrContext) bool   { return nSuitSameRankChow(ctx, 3) }
func mixedShiftedPungs(ctx mcrContext) bool { return mixedShifted(ctx, false) }
func mixedShiftedChows(ctx mcrContext) bool { return mixedShifted(ctx, true) }
func nSuitSameRankPung(ctx mcrContext, n int) bool {
	for rank := 1; rank <= 9; rank++ {
		suits := map[tile.Suit]bool{}
		for _, d := range ctx.decomps {
			for _, m := range d.melds {
				if (m.kind == "pung" || m.kind == "kong") && m.rank == rank && m.suitIsSuited() {
					suits[m.suit] = true
				}
			}
		}
		if len(suits) >= n {
			return true
		}
	}
	return false
}
func nSuitSameRankChow(ctx mcrContext, n int) bool {
	for _, d := range ctx.decomps {
		for rank := 1; rank <= 7; rank++ {
			suits := map[tile.Suit]bool{}
			for _, m := range d.melds {
				if m.kind == "chow" && m.rank == rank {
					suits[m.suit] = true
				}
			}
			if len(suits) >= n {
				return true
			}
		}
	}
	return false
}
func mixedShifted(ctx mcrContext, chow bool) bool {
	for _, d := range ctx.decomps {
		for start := 1; start <= 7; start++ {
			ok := true
			for off, s := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
				if chow && !hasChow(d, s, start+off) {
					ok = false
				}
				if !chow && !hasPung(d, s, start+off) {
					ok = false
				}
			}
			if ok {
				return true
			}
		}
	}
	return false
}

func pureDoubleChow(ctx mcrContext) int  { return countDoubleChow(ctx, true) }
func mixedDoubleChow(ctx mcrContext) int { return countDoubleChow(ctx, false) }
func countDoubleChow(ctx mcrContext, sameSuit bool) int {
	max := 0
	for _, d := range ctx.decomps {
		counts := map[string]int{}
		for _, m := range d.melds {
			if m.kind != "chow" {
				continue
			}
			key := strconv.Itoa(m.rank)
			if sameSuit {
				key = strconv.Itoa(int(m.suit)) + ":" + key
			}
			counts[key]++
		}
		n := 0
		for _, c := range counts {
			n += c / 2
		}
		if n > max {
			max = n
		}
	}
	return max
}
func shortStraight(ctx mcrContext) int {
	return countChowCombos(ctx, [][2]int{{1, 4}, {4, 7}})
}
func twoTerminalChows(ctx mcrContext) int {
	return countChowCombos(ctx, [][2]int{{1, 7}})
}
func countChowCombos(ctx mcrContext, combos [][2]int) int {
	max := 0
	for _, d := range ctx.decomps {
		n := 0
		for _, s := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
			for _, combo := range combos {
				if hasChow(d, s, combo[0]) && hasChow(d, s, combo[1]) {
					n++
				}
			}
		}
		if n > max {
			max = n
		}
	}
	return max
}

func terminalOrHonorPungs(ctx mcrContext) int {
	n := 0
	seen := map[int]bool{}
	for _, d := range ctx.decomps {
		for _, m := range d.melds {
			if m.kind != "pung" && m.kind != "kong" {
				continue
			}
			idx := indexFor(m.suit, m.rank)
			if seen[idx] {
				continue
			}
			if idx >= tile.SuitedTileCount || isSuitedTerminal(idx) {
				seen[idx] = true
				n++
			}
		}
	}
	return n
}
func doublePung(ctx mcrContext) int {
	n := 0
	for rank := 1; rank <= 9; rank++ {
		count := 0
		for _, s := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
			if anyPung(ctx, s, rank) {
				count++
			}
		}
		n += count / 2
	}
	return n
}

func tileHog(c hu.Counts) int {
	n := 0
	for _, count := range c {
		if count == 4 {
			n++
		}
	}
	return n
}
