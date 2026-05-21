package tile

import "testing"

func TestNewAndAccessors(t *testing.T) {
	ti, err := New(SuitDots, 5)
	if err != nil {
		t.Fatal(err)
	}
	if ti.Suit() != SuitDots || ti.Rank() != 5 || ti.Index() != 9+4 {
		t.Fatalf("unexpected: %#v idx=%d", ti, ti.Index())
	}
}

func TestNewInvalid(t *testing.T) {
	if _, err := New(SuitBamboo, 0); err == nil {
		t.Fatal("expected error")
	}
}

func TestFromIndex(t *testing.T) {
	tests := []struct {
		idx  int
		suit Suit
		rank int
	}{
		{idx: 26, suit: SuitBamboo, rank: 9},
		{idx: 27, suit: SuitHonor, rank: 1},
		{idx: 33, suit: SuitHonor, rank: 7},
		{idx: 34, suit: SuitFlower, rank: 1},
		{idx: 41, suit: SuitFlower, rank: 8},
	}
	for _, tt := range tests {
		ti, err := FromIndex(tt.idx)
		if err != nil {
			t.Fatalf("idx %d: %v", tt.idx, err)
		}
		if ti.Suit() != tt.suit || ti.Rank() != tt.rank {
			t.Fatalf("idx %d got %v", tt.idx, ti)
		}
	}
}

func TestParseRoundTrip(t *testing.T) {
	for _, s := range []string{"m1", "P9", " s5 ", "z7", "F8"} {
		ti, err := Parse(s)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if _, err := Parse(ti.String()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, s := range []string{"", "m", "m10", "x1", "m0"} {
		if _, err := Parse(s); err == nil {
			t.Fatalf("expected error for %q", s)
		}
	}
}
