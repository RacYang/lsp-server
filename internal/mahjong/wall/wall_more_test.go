package wall

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestTilesSnapshot(t *testing.T) {
	w := NewFull108()
	w.ShuffleWithSeed(99)
	head := w.Tiles()
	if len(head) != 108 {
		t.Fatalf("len=%d", len(head))
	}
	if _, err := w.Draw(); err != nil {
		t.Fatal(err)
	}
	if len(w.Tiles()) != 107 {
		t.Fatalf("len=%d", len(w.Tiles()))
	}
}

func TestNilWall(t *testing.T) {
	var w *Wall
	if w.Remaining() != 0 {
		t.Fatal("nil remaining")
	}
	if _, err := w.Draw(); err == nil {
		t.Fatal("expected error")
	}
	w2 := &Wall{}
	w2.Shuffle(nil) // 空牌墙早退
}

func TestShuffleNilSourceUsesDefault(t *testing.T) {
	w := NewFull108()
	w.Shuffle(nil)
	if w.Remaining() != 108 {
		t.Fatalf("remaining=%d", w.Remaining())
	}
}

func TestResetCursor(t *testing.T) {
	w := NewFull108()
	w.ShuffleWithSeed(3)
	a, _ := w.Draw()
	w.ResetCursor()
	b, _ := w.Draw()
	if a != b {
		t.Fatalf("expected same first draw after reset, %v vs %v", a, b)
	}
}

func TestDrawNilWallError(t *testing.T) {
	var w *Wall
	if _, err := w.Draw(); err == nil {
		t.Fatal("expected error")
	}
}

func TestTileFirstDrawIsValid(t *testing.T) {
	w := NewFull108()
	w.ShuffleWithSeed(11)
	ti, err := w.Draw()
	if err != nil {
		t.Fatal(err)
	}
	if ti == (tile.Tile(0)) {
		t.Fatal("unexpected zero tile")
	}
}
