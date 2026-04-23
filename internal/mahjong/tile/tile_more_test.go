package tile

import "testing"

func TestMustPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = Must(SuitDots, 10)
}

func TestAllSuitTiles(t *testing.T) {
	for s := SuitCharacters; s <= SuitBamboo; s++ {
		ts := AllSuitTiles(s)
		if len(ts) != 9 {
			t.Fatalf("suit=%v len=%d", s, len(ts))
		}
	}
}

func TestStringUnknownSuit(t *testing.T) {
	ti := Tile(0xF1)
	if s := ti.String(); len(s) < 2 {
		t.Fatalf("unexpected string %q", s)
	}
}

func TestFromIndexInvalid(t *testing.T) {
	if _, err := FromIndex(-1); err == nil {
		t.Fatal("expected error")
	}
	if _, err := FromIndex(27); err == nil {
		t.Fatal("expected error")
	}
}
