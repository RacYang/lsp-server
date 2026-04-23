package wall

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestNewFull108Count(t *testing.T) {
	w := NewFull108()
	if len(w.tiles) != 108 {
		t.Fatalf("len=%d", len(w.tiles))
	}
	cnt := make(map[tile.Tile]int)
	for _, ti := range w.tiles {
		cnt[ti]++
	}
	if len(cnt) != 27 {
		t.Fatalf("distinct tiles=%d", len(cnt))
	}
	for _, c := range cnt {
		if c != 4 {
			t.Fatalf("want 4 each, got %v", cnt)
		}
	}
}

func TestShuffleDeterministic(t *testing.T) {
	a := NewFull108()
	b := NewFull108()
	a.ShuffleWithSeed(42)
	b.ShuffleWithSeed(42)
	for i := range a.tiles {
		if a.tiles[i] != b.tiles[i] {
			t.Fatalf("diff at %d", i)
		}
	}
}

func TestDrawExhaust(t *testing.T) {
	w := NewFull108()
	w.ShuffleWithSeed(1)
	for i := 0; i < 108; i++ {
		if _, err := w.Draw(); err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
	}
	if _, err := w.Draw(); err == nil {
		t.Fatal("expected exhaust")
	}
}
