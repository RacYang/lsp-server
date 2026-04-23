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
	ti, err := FromIndex(26)
	if err != nil {
		t.Fatal(err)
	}
	if ti.Suit() != SuitBamboo || ti.Rank() != 9 {
		t.Fatalf("got %v", ti)
	}
}

func TestParseRoundTrip(t *testing.T) {
	for _, s := range []string{"m1", "P9", " s5 "} {
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
