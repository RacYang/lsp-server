package tile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSuitedIsHonorIsFlower(t *testing.T) {
	tests := []struct {
		tile   Tile
		suited bool
		honor  bool
		flower bool
	}{
		{Must(SuitCharacters, 5), true, false, false},
		{Must(SuitDots, 1), true, false, false},
		{Must(SuitBamboo, 9), true, false, false},
		{Must(SuitHonor, 1), false, true, false},
		{Must(SuitHonor, 7), false, true, false},
		{Must(SuitFlower, 1), false, false, true},
		{Must(SuitFlower, 8), false, false, true},
	}
	for _, tc := range tests {
		require.Equal(t, tc.suited, tc.tile.IsSuited(), "IsSuited %v", tc.tile)
		require.Equal(t, tc.honor, tc.tile.IsHonor(), "IsHonor %v", tc.tile)
		require.Equal(t, tc.flower, tc.tile.IsFlower(), "IsFlower %v", tc.tile)
	}
}

func TestIndexHonorAndFlower(t *testing.T) {
	for rank := 1; rank <= 7; rank++ {
		ti := Must(SuitHonor, rank)
		require.Equal(t, SuitedTileCount+rank-1, ti.Index(), "Honor rank %d", rank)
	}
	for rank := 1; rank <= 8; rank++ {
		ti := Must(SuitFlower, rank)
		require.Equal(t, PlayableTileCount+rank-1, ti.Index(), "Flower rank %d", rank)
	}
}

func TestIndexDefaultBranch(t *testing.T) {
	ti := Tile(0xF1)
	require.Equal(t, -1, ti.Index())
}

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
	if _, err := FromIndex(FullTileCount); err == nil {
		t.Fatal("expected error")
	}
}
