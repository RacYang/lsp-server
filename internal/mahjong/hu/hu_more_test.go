package hu

import "testing"

func TestSevenPairsRejectOddCount(t *testing.T) {
	var c Counts
	c[0] = 3
	c[1] = 11
	if SevenPairs(c) {
		t.Fatal("expected false")
	}
}

func TestStandardFormRequiresFourteen(t *testing.T) {
	var c Counts
	c[0] = 2
	if StandardForm(c) {
		t.Fatal("expected false when总张数不是 14")
	}
}

func TestIsWinningRejectsNonFourteen(t *testing.T) {
	var c Counts
	c[0] = 2
	if IsWinning(c) {
		t.Fatal("expected false")
	}
}

func TestIsWinningRejectsMoreThanFour(t *testing.T) {
	var c Counts
	c[0] = 14
	if IsWinning(c) {
		t.Fatal("expected false when单张计数超过 4")
	}
}
