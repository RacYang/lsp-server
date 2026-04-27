// Package wall 负责 108 张序数牌牌墙的构建、洗牌与摸牌，支持注入随机源以便测试复现。
package wall

import (
	"fmt"
	"math/rand/v2"

	"racoo.cn/lsp/internal/mahjong/tile"
)

// Wall 表示可逐张消耗的牌墙。
type Wall struct {
	tiles []tile.Tile
	pos   int
}

// NewFull108 构造标准 108 张牌墙（万筒条各 36 张：1..9 各 4 张）。
func NewFull108() *Wall {
	tiles := make([]tile.Tile, 0, 108)
	for s := tile.SuitCharacters; s <= tile.SuitBamboo; s++ {
		for r := 1; r <= 9; r++ {
			t, _ := tile.New(s, r)
			for k := 0; k < 4; k++ {
				tiles = append(tiles, t)
			}
		}
	}
	return &Wall{tiles: tiles, pos: 0}
}

// NewFromOrderedTiles 使用给定顺序构造牌墙，摸牌按切片顺序从前往后消耗。
// 用于从持久化快照恢复剩余牌墙。
func NewFromOrderedTiles(ts []tile.Tile) *Wall {
	if len(ts) == 0 {
		return &Wall{}
	}
	cp := make([]tile.Tile, len(ts))
	copy(cp, ts)
	return &Wall{tiles: cp}
}

// Tiles 返回当前尚未摸走的牌序列快照（含已摸部分之后的牌），仅用于测试或诊断。
func (w *Wall) Tiles() []tile.Tile {
	if w == nil {
		return nil
	}
	out := make([]tile.Tile, len(w.tiles)-w.pos)
	copy(out, w.tiles[w.pos:])
	return out
}

// Shuffle 使用给定随机源洗牌；seed 仅用于构造默认 Source，便于 YAML 复现。
func (w *Wall) Shuffle(src *rand.Rand) {
	if w == nil || len(w.tiles) == 0 {
		return
	}
	if src == nil {
		src = rand.New(rand.NewPCG(0, 0)) //nolint:gosec // G404：牌墙洗牌需可复现，使用 PCG 而非加密随机
	}
	src.Shuffle(len(w.tiles), func(i, j int) {
		w.tiles[i], w.tiles[j] = w.tiles[j], w.tiles[i]
	})
	w.pos = 0
}

// ShuffleWithSeed 使用确定性 PCG 洗牌，便于夹具与单测。
func (w *Wall) ShuffleWithSeed(seed uint64) {
	src := rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15)) //nolint:gosec // G404：同上，供测试与房间固定种子
	w.Shuffle(src)
}

// Remaining 返回剩余张数。
func (w *Wall) Remaining() int {
	if w == nil {
		return 0
	}
	if w.pos > len(w.tiles) {
		return 0
	}
	return len(w.tiles) - w.pos
}

// Draw 摸一张牌；牌墙耗尽返回错误。
func (w *Wall) Draw() (tile.Tile, error) {
	if w == nil {
		return 0, fmt.Errorf("nil wall")
	}
	if w.Remaining() == 0 {
		return 0, fmt.Errorf("wall exhausted")
	}
	t := w.tiles[w.pos]
	w.pos++
	return t, nil
}

// PushFront 将一张牌放回当前摸牌指针前方，用于撤销尚未生效的摸牌。
func (w *Wall) PushFront(t tile.Tile) error {
	if w == nil {
		return fmt.Errorf("nil wall")
	}
	if w.pos == 0 {
		w.tiles = append([]tile.Tile{t}, w.tiles...)
		return nil
	}
	w.pos--
	w.tiles[w.pos] = t
	return nil
}

// ResetCursor 将摸牌指针重置到起点（不改变牌序），用于测试。
func (w *Wall) ResetCursor() {
	if w == nil {
		return
	}
	w.pos = 0
}
