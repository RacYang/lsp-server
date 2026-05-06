package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderTileUnicodeNumbers(t *testing.T) {
	cases := []struct {
		name string
		want [4]string
	}{
		{"m1", [4]string{"┌──┐", "│一│", "│万│", "└──┘"}},
		{"p5", [4]string{"┌──┐", "│五│", "│筒│", "└──┘"}},
		{"s9", [4]string{"┌──┐", "│九│", "│条│", "└──┘"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderTile(tc.name, TileThemeUnicode)
			require.Equal(t, tc.want, got.Lines)
			require.Equal(t, TileArtWidth, got.Width)
		})
	}
}

func TestRenderTileUnicodeHonors(t *testing.T) {
	got := RenderTile("z1", TileThemeUnicode)
	require.Equal(t, "┌──┐", got.Lines[0])
	require.Equal(t, "│东│", got.Lines[1])
	require.Equal(t, "│  │", got.Lines[2])
	require.Equal(t, "└──┘", got.Lines[3])
}

func TestRenderTileASCII(t *testing.T) {
	got := RenderTile("p5", TileThemeASCII)
	require.Equal(t, [4]string{"+--+", "|p5|", "|  |", "+--+"}, got.Lines)
	require.Equal(t, TileArtWidth, got.Width)
}

func TestRenderTileASCIIHonor(t *testing.T) {
	got := RenderTile("z1", TileThemeASCII)
	require.Equal(t, "|E |", got.Lines[1])
}

func TestRenderTileFallbackOnUnknownInput(t *testing.T) {
	got := RenderTile("???", TileThemeUnicode)
	require.Len(t, got.Lines[0], len("┌──┐"))
}

func TestParseTileTheme(t *testing.T) {
	require.Equal(t, TileThemeUnicode, ParseTileTheme(""))
	require.Equal(t, TileThemeUnicode, ParseTileTheme("UNICODE"))
	require.Equal(t, TileThemeASCII, ParseTileTheme("ascii"))
	require.Equal(t, TileThemeUnicode, ParseTileTheme("weird"))
}

func TestJoinTilesHorizontallyGroupsWithSeparator(t *testing.T) {
	groups := [][]TileArt{
		{RenderTile("m1", TileThemeASCII), RenderTile("m2", TileThemeASCII)},
		{RenderTile("p5", TileThemeASCII)},
	}
	rows := JoinTilesHorizontally(groups, "  ")
	require.Equal(t, "+--++--+  +--+", rows[0])
	require.Equal(t, "|m1||m2|  |p5|", rows[1])
	require.Equal(t, "|  ||  |  |  |", rows[2])
	require.Equal(t, "+--++--+  +--+", rows[3])
}

func TestHiddenTilesRowCount(t *testing.T) {
	asciiRow := HiddenTilesRow(3, TileThemeASCII)
	require.Equal(t, "[][][]", asciiRow)

	unicodeRow := HiddenTilesRow(3, TileThemeUnicode)
	require.Equal(t, 3, strings.Count(unicodeRow, "▢"))
}

func TestVisualWidthCJKDouble(t *testing.T) {
	require.Equal(t, 2, visualWidth("一"))
	require.Equal(t, 4, visualWidth("一万"))
	require.Equal(t, 2, visualWidth("ab"))
	require.Equal(t, 4, visualWidth("a一b"))
}
