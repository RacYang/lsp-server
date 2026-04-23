package hand

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestNilHandOperations(t *testing.T) {
	var h *Hand
	if h.Len() != 0 {
		t.Fatal("nil len")
	}
	if h.Counts() != ([27]int{}) {
		t.Fatal("nil counts")
	}
	if h.Tiles() != nil {
		t.Fatal("nil tiles")
	}
	h.Add(tile.Must(tile.SuitDots, 1)) // 不应崩溃
	if err := h.Remove(tile.Must(tile.SuitDots, 1)); err == nil {
		t.Fatal("expected error on nil remove")
	}
}

func TestFromTilesIsCopy(t *testing.T) {
	ts := []tile.Tile{tile.Must(tile.SuitCharacters, 9)}
	h := FromTiles(ts)
	ts[0] = tile.Must(tile.SuitCharacters, 1)
	if h.Tiles()[0].Rank() != 9 {
		t.Fatalf("FromTiles 应复制切片，避免外部改写影响手牌")
	}
}
