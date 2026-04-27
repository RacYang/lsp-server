package hu

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestTingTiles(t *testing.T) {
	var c Counts
	for _, tt := range []tile.Tile{
		tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1),
		tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 4),
		tile.Must(tile.SuitCharacters, 5), tile.Must(tile.SuitCharacters, 6), tile.Must(tile.SuitCharacters, 7),
		tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 3), tile.Must(tile.SuitDots, 4),
		tile.Must(tile.SuitBamboo, 8), tile.Must(tile.SuitBamboo, 8),
	} {
		c[tt.Index()]++
	}
	got := TingTiles(c)
	if len(got) == 0 {
		t.Fatal("expected ting tiles")
	}
	if !IsTing(c) {
		t.Fatal("expected hand to be ting")
	}
}

func TestTingTilesRejectsNonThirteenTiles(t *testing.T) {
	var c Counts
	c[tile.Must(tile.SuitCharacters, 1).Index()] = 14
	if got := TingTiles(c); len(got) != 0 {
		t.Fatalf("TingTiles len = %d", len(got))
	}
}
