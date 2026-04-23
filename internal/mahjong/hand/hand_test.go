package hand

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/tile"
)

func TestCounts(t *testing.T) {
	h := New()
	m3, _ := tile.Parse("m3")
	h.Add(m3)
	h.Add(m3)
	c := h.Counts()
	if c[m3.Index()] != 2 {
		t.Fatalf("counts=%v", c)
	}
}

func TestRemove(t *testing.T) {
	h := New()
	a := tile.Must(tile.SuitDots, 1)
	h.Add(a)
	if err := h.Remove(a); err != nil {
		t.Fatal(err)
	}
	if h.Len() != 0 {
		t.Fatalf("len=%d", h.Len())
	}
	if err := h.Remove(a); err == nil {
		t.Fatal("expected error")
	}
}
