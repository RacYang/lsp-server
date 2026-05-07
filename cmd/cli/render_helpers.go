package main

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

func drawClippedText(scr tcell.Screen, x, y int, style tcell.Style, s string, width int) {
	if width <= 0 {
		return
	}
	drawText(scr, x, y, style, clipVisual(s, width))
}

func clipVisual(s string, width int) string {
	if width <= 0 || visualWidth(s) <= width {
		return s
	}
	var out strings.Builder
	used := 0
	for _, r := range s {
		w := visualWidth(string(r))
		if used+w > width-1 {
			break
		}
		out.WriteRune(r)
		used += w
	}
	out.WriteString("…")
	return out.String()
}

func renderMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
