package wall

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestNewFull108Count(t *testing.T) {
	w := NewFull108()
	assertWallCopies(t, w, 108, 27, 4, false)
}

func TestNewFull136Count(t *testing.T) {
	w := NewFull136()
	assertWallCopies(t, w, 136, 34, 4, false)
}

func TestNewFull144Count(t *testing.T) {
	w := NewFull144()
	assertWallCopies(t, w, 144, 42, 4, true)
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

func assertWallCopies(t *testing.T, w *Wall, total, distinct, commonCopies int, hasFlowers bool) {
	t.Helper()
	if len(w.tiles) != total {
		t.Fatalf("len=%d", len(w.tiles))
	}
	cnt := make(map[tile.Tile]int)
	for _, ti := range w.tiles {
		cnt[ti]++
	}
	if len(cnt) != distinct {
		t.Fatalf("distinct tiles=%d", len(cnt))
	}
	for ti, c := range cnt {
		want := commonCopies
		if ti.IsFlower() {
			if !hasFlowers {
				t.Fatalf("unexpected flower %v", ti)
			}
			want = 1
		}
		if c != want {
			t.Fatalf("tile %v want %d copies, got %d in %v", ti, want, c, cnt)
		}
	}
}
