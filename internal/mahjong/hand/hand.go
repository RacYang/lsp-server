// Package hand 提供手牌计数、增删与和牌用计数表转换，不包含网络语义。
package hand

import (
	"fmt"

	"racoo.cn/lsp/internal/mahjong/tile"
)

// Hand 表示一名玩家的手牌 multiset。
type Hand struct {
	tiles []tile.Tile
}

// New 创建空手牌。
func New() *Hand {
	return &Hand{tiles: make([]tile.Tile, 0, 14)}
}

// FromTiles 由切片构造手牌（会复制）。
func FromTiles(ts []tile.Tile) *Hand {
	cp := append([]tile.Tile(nil), ts...)
	return &Hand{tiles: cp}
}

// Counts 返回 27 维计数，下标与 tile.Tile.Index 一致。
func (h *Hand) Counts() [27]int {
	var c [27]int
	if h == nil {
		return c
	}
	for _, t := range h.tiles {
		c[t.Index()]++
	}
	return c
}

// Len 返回张数。
func (h *Hand) Len() int {
	if h == nil {
		return 0
	}
	return len(h.tiles)
}

// Tiles 返回手牌快照。
func (h *Hand) Tiles() []tile.Tile {
	if h == nil {
		return nil
	}
	out := make([]tile.Tile, len(h.tiles))
	copy(out, h.tiles)
	return out
}

// Add 加入一张牌。
func (h *Hand) Add(t tile.Tile) {
	if h == nil {
		return
	}
	h.tiles = append(h.tiles, t)
}

// Remove 移除一张等价牌；不存在则返回错误。
func (h *Hand) Remove(t tile.Tile) error {
	if h == nil {
		return fmt.Errorf("nil hand")
	}
	for i := range h.tiles {
		if h.tiles[i] == t {
			h.tiles = append(h.tiles[:i], h.tiles[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("tile not in hand: %v", t)
}
