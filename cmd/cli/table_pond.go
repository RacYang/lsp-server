package main

import "github.com/gdamore/tcell/v2"

type PondDir int

const (
	PondDown PondDir = iota
	PondUp
	PondRight
	PondLeft
)

// drawPondMatrix 按雀魂式 6 列方阵渲染牌河，牌从各家边向桌心铺开。
func drawPondMatrix(scr tcell.Screen, region Region, dir PondDir, tiles []string, cols int, style tcell.Style) {
	if region.Empty() || len(tiles) == 0 {
		return
	}
	if cols <= 0 {
		cols = 6
	}
	for i, tile := range tiles {
		row := i / cols
		col := i % cols
		x, y := pondCell(region, dir, row, col)
		if x < region.X || y < region.Y || x >= region.X+region.Width || y >= region.Y+region.Height {
			continue
		}
		drawTileGlyph(scr, x, y, style, TileGlyph(tile))
	}
}

func pondCell(region Region, dir PondDir, row, col int) (int, int) {
	switch dir {
	case PondUp:
		return region.X + col*3, region.Y + region.Height - 1 - row
	case PondRight:
		return region.X + col*3, region.Y + row
	case PondLeft:
		return region.X + region.Width - 2 - col*3, region.Y + row
	default:
		return region.X + col*3, region.Y + row
	}
}
