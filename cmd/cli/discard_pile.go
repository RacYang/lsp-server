package main

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// drawDiscardPile 把弃牌按固定列数折行，保留顺序而不是截成一条难读的长字符串。
func drawDiscardPile(scr tcell.Screen, region Region, label string, discards []string, columns int, style tcell.Style) {
	if region.Empty() || len(discards) == 0 {
		return
	}
	rows := formatDiscardRows(discards, columns)
	for i, row := range rows {
		if i >= region.Height {
			break
		}
		prefix := "   "
		if i == 0 {
			prefix = label
		}
		drawClippedText(scr, region.X, region.Y+i, style, prefix+row, region.Width)
	}
}

func formatDiscardRows(discards []string, columns int) []string {
	if columns <= 0 {
		columns = 4
	}
	rows := make([]string, 0, (len(discards)+columns-1)/columns)
	for start := 0; start < len(discards); start += columns {
		end := start + columns
		if end > len(discards) {
			end = len(discards)
		}
		pretty := make([]string, 0, end-start)
		for _, tile := range discards[start:end] {
			pretty = append(pretty, TileName(tile))
		}
		rows = append(rows, strings.Join(pretty, " "))
	}
	return rows
}
