// Package analysis 提供麻将牌形分析、进张估算与机器人策略可复用的轻量启发式。
package analysis

import (
	"math"
	"sort"

	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// Counts 是与 tile.Tile.Index 对齐的 27 维牌张计数。
type Counts = hu.Counts

// PublicInfo 表示当前座位可见的公开牌桌信息。
type PublicInfo struct {
	DiscardsBySeat [][]string
	MeldsBySeat    [][]string
	DrawnBySeat    [][]string
	QueBySeat      []int32
	SelfSeat       int32
}

// DiscardOption 描述一次弃牌候选的评分结果。
type DiscardOption struct {
	Tile            tile.Tile
	Shanten         int
	AdvanceKinds    int
	AdvanceLeft     int
	Safety          int
	QueSuit         bool
	ExpectedFanHint int
}

// Shanten 返回手牌距离和牌所需的最少有效进张数，0 表示已经听牌。
func Shanten(c Counts) int {
	total := c.Total()
	if total == 0 {
		return 8
	}
	if total%3 == 2 && hu.IsWinning(c) {
		return -1
	}
	std := standardShanten(c)
	qd := sevenPairsShanten(c)
	if qd < std {
		return qd
	}
	return std
}

// Tenpai 返回 13 张手牌的听牌集合。
func Tenpai(c Counts) []tile.Tile {
	return hu.TingTiles(c)
}

// AdvanceTiles 返回当前 13 张手牌的进张与公开信息推导出的剩余张数。
func AdvanceTiles(c Counts, pub PublicInfo) map[tile.Tile]int {
	out := make(map[tile.Tile]int)
	for _, t := range Tenpai(c) {
		left := RemainingByPublic(t, c, pub)
		if left > 0 {
			out[t] = left
		}
	}
	return out
}

// RemainingByPublic 用自家手牌与公开牌桌事实估算某张牌仍未出现的数量。
func RemainingByPublic(t tile.Tile, self Counts, pub PublicInfo) int {
	left := 4 - self[t.Index()]
	for _, group := range pub.DiscardsBySeat {
		left -= countTileText(group, t)
	}
	for _, group := range pub.MeldsBySeat {
		left -= countTileInMelds(group, t)
	}
	for _, group := range pub.DrawnBySeat {
		left -= countTileText(group, t)
	}
	if left < 0 {
		return 0
	}
	return left
}

// SafetyScore 返回越大越安全的朴素安全度评分。
func SafetyScore(t tile.Tile, pub PublicInfo) int {
	score := 0
	for seat, discards := range pub.DiscardsBySeat {
		seatIndex := int32(seat) //nolint:gosec // 座位下标来自长度固定的公开座位切片。
		if seatIndex == pub.SelfSeat {
			continue
		}
		if containsTileText(discards, t) {
			score += 6
		}
		if seat < len(pub.QueBySeat) && pub.QueBySeat[seat] == int32(t.Suit()) {
			score += 4
		}
	}
	for seat, melds := range pub.MeldsBySeat {
		seatIndex := int32(seat) //nolint:gosec // 座位下标来自长度固定的公开座位切片。
		if seatIndex == pub.SelfSeat {
			continue
		}
		for _, raw := range melds {
			if countTileInMeld(raw, t) > 0 {
				score -= 2
			}
		}
	}
	return score
}

// BestQueSuit 选择定缺花色：优先张数少，并用向听变化打破平局。
func BestQueSuit(ts []tile.Tile) tile.Suit {
	countsBySuit := suitCounts(ts)
	best := tile.SuitCharacters
	bestCount := math.MaxInt
	bestShanten := math.MaxInt
	for _, suit := range allSuits() {
		filtered := make([]tile.Tile, 0, len(ts))
		for _, t := range ts {
			if t.Suit() != suit {
				filtered = append(filtered, t)
			}
		}
		sh := Shanten(hand.FromTiles(filtered).Counts())
		if countsBySuit[suit] < bestCount || countsBySuit[suit] == bestCount && sh < bestShanten {
			best = suit
			bestCount = countsBySuit[suit]
			bestShanten = sh
		}
	}
	return best
}

// BestExchangeThree 选择同花色的三张换三张候选。
func BestExchangeThree(ts []tile.Tile, que tile.Suit) []tile.Tile {
	if len(ts) <= 3 {
		return append([]tile.Tile(nil), ts...)
	}
	bySuit := map[tile.Suit][]tile.Tile{}
	for _, t := range ts {
		bySuit[t.Suit()] = append(bySuit[t.Suit()], t)
	}
	target := que
	if len(bySuit[target]) < 3 {
		target = tile.SuitCharacters
		bestLen := -1
		for _, suit := range allSuits() {
			if n := len(bySuit[suit]); n > bestLen {
				target = suit
				bestLen = n
			}
		}
	}
	candidates := append([]tile.Tile(nil), bySuit[target]...)
	sort.Slice(candidates, func(i, j int) bool {
		return tileWeakness(candidates[i]) > tileWeakness(candidates[j])
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	return candidates
}

// BestDiscard 返回当前手牌在公开信息下的弃牌候选。
func BestDiscard(ts []tile.Tile, que tile.Suit, pub PublicInfo) DiscardOption {
	options := DiscardOptions(ts, que, pub)
	if len(options) == 0 {
		return DiscardOption{}
	}
	return options[0]
}

// DiscardOptions 计算并按策略偏好排序全部弃牌候选。
func DiscardOptions(ts []tile.Tile, que tile.Suit, pub PublicInfo) []DiscardOption {
	counted := uniqueTiles(ts)
	out := make([]DiscardOption, 0, len(counted))
	for _, discard := range counted {
		rest := removeOne(ts, discard)
		c := hand.FromTiles(rest).Counts()
		adv := AdvanceTiles(c, pub)
		kinds := len(adv)
		left := 0
		for _, n := range adv {
			left += n
		}
		out = append(out, DiscardOption{
			Tile:         discard,
			Shanten:      Shanten(c),
			AdvanceKinds: kinds,
			AdvanceLeft:  left,
			Safety:       SafetyScore(discard, pub),
			QueSuit:      discard.Suit() == que,
		})
	}
	sort.Slice(out, func(i, j int) bool { return betterDiscard(out[i], out[j]) })
	return out
}

func betterDiscard(a, b DiscardOption) bool {
	if a.QueSuit != b.QueSuit {
		return a.QueSuit
	}
	if a.Shanten != b.Shanten {
		return a.Shanten < b.Shanten
	}
	if a.AdvanceKinds != b.AdvanceKinds {
		return a.AdvanceKinds > b.AdvanceKinds
	}
	if a.AdvanceLeft != b.AdvanceLeft {
		return a.AdvanceLeft > b.AdvanceLeft
	}
	if a.Safety != b.Safety {
		return a.Safety > b.Safety
	}
	if a.Tile.Suit() != b.Tile.Suit() {
		return a.Tile.Suit() < b.Tile.Suit()
	}
	return a.Tile.Rank() < b.Tile.Rank()
}

func standardShanten(c Counts) int {
	best := 8
	var walk func(Counts, int, int, int, bool)
	walk = func(cur Counts, idx int, melds int, taatsu int, pair bool) {
		for idx < 27 && cur[idx] == 0 {
			idx++
		}
		if idx >= 27 {
			if taatsu > 4-melds {
				taatsu = 4 - melds
			}
			pairUsed := 0
			if pair {
				pairUsed = 1
			}
			sh := 8 - melds*2 - taatsu - pairUsed
			if sh < best {
				best = sh
			}
			return
		}
		if cur[idx] >= 3 {
			next := cur
			next[idx] -= 3
			walk(next, idx, melds+1, taatsu, pair)
		}
		suitStart := (idx / 9) * 9
		pos := idx - suitStart
		if pos <= 6 && cur[idx+1] > 0 && cur[idx+2] > 0 {
			next := cur
			next[idx]--
			next[idx+1]--
			next[idx+2]--
			walk(next, idx, melds+1, taatsu, pair)
		}
		if !pair && cur[idx] >= 2 {
			next := cur
			next[idx] -= 2
			walk(next, idx, melds, taatsu, true)
		}
		if cur[idx] >= 2 {
			next := cur
			next[idx] -= 2
			walk(next, idx, melds, taatsu+1, pair)
		}
		if pos <= 7 && cur[idx+1] > 0 {
			next := cur
			next[idx]--
			next[idx+1]--
			walk(next, idx, melds, taatsu+1, pair)
		}
		if pos <= 6 && cur[idx+2] > 0 {
			next := cur
			next[idx]--
			next[idx+2]--
			walk(next, idx, melds, taatsu+1, pair)
		}
		next := cur
		next[idx]--
		walk(next, idx, melds, taatsu, pair)
	}
	walk(c, 0, 0, 0, false)
	if best < -1 {
		return -1
	}
	return best
}

func sevenPairsShanten(c Counts) int {
	pairs := 0
	kinds := 0
	for _, n := range c {
		if n > 0 {
			kinds++
		}
		if n >= 2 {
			pairs++
		}
	}
	needPairs := 7 - pairs
	needKinds := 7 - kinds
	if needKinds < 0 {
		needKinds = 0
	}
	return needPairs + needKinds - 1
}

func allSuits() []tile.Suit {
	return []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo}
}

func suitCounts(ts []tile.Tile) map[tile.Suit]int {
	out := map[tile.Suit]int{
		tile.SuitCharacters: 0,
		tile.SuitDots:       0,
		tile.SuitBamboo:     0,
	}
	for _, t := range ts {
		out[t.Suit()]++
	}
	return out
}

func uniqueTiles(ts []tile.Tile) []tile.Tile {
	seen := make(map[tile.Tile]bool, len(ts))
	out := make([]tile.Tile, 0, len(ts))
	for _, t := range ts {
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index() < out[j].Index() })
	return out
}

func removeOne(ts []tile.Tile, target tile.Tile) []tile.Tile {
	out := append([]tile.Tile(nil), ts...)
	for i, t := range out {
		if t == target {
			return append(out[:i], out[i+1:]...)
		}
	}
	return out
}

func tileWeakness(t tile.Tile) int {
	r := t.Rank()
	switch r {
	case 1, 9:
		return 4
	case 2, 8:
		return 3
	case 3, 7:
		return 2
	default:
		return 1
	}
}

func countTileText(raws []string, target tile.Tile) int {
	n := 0
	for _, raw := range raws {
		t, err := tile.Parse(raw)
		if err == nil && t == target {
			n++
		}
	}
	return n
}

func containsTileText(raws []string, target tile.Tile) bool {
	return countTileText(raws, target) > 0
}

func countTileInMelds(melds []string, target tile.Tile) int {
	n := 0
	for _, meld := range melds {
		n += countTileInMeld(meld, target)
	}
	return n
}

func countTileInMeld(raw string, target tile.Tile) int {
	n := 0
	for i := 0; i+1 < len(raw); i++ {
		if raw[i] != 'm' && raw[i] != 'p' && raw[i] != 's' {
			continue
		}
		t, err := tile.Parse(raw[i : i+2])
		if err == nil && t == target {
			n++
		}
	}
	return n
}
