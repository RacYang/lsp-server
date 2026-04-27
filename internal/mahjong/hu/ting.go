package hu

import "racoo.cn/lsp/internal/mahjong/tile"

// TingTiles 枚举当前 13 张手牌的一张进张听牌集合。
func TingTiles(c Counts) []tile.Tile {
	if c.Total() != 13 {
		return nil
	}
	out := make([]tile.Tile, 0, 9)
	for idx := 0; idx < 27; idx++ {
		if c[idx] >= 4 {
			continue
		}
		next := c
		next[idx]++
		if !IsWinning(next) {
			continue
		}
		t, err := tile.FromIndex(idx)
		if err != nil {
			continue
		}
		out = append(out, t)
	}
	return out
}

// IsTing 判断 13 张手牌是否至少有一张可和进张。
func IsTing(c Counts) bool {
	return len(TingTiles(c)) > 0
}
