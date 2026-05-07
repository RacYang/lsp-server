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

func drawBox(scr tcell.Screen, region Region, title string) {
	if region.Width < 4 || region.Height < 3 {
		return
	}
	top := "┌" + strings.Repeat("─", region.Width-2) + "┐"
	bottom := "└" + strings.Repeat("─", region.Width-2) + "┘"
	if title != "" && visualWidth(title)+4 < region.Width {
		top = "┌─ " + title + " " + strings.Repeat("─", region.Width-visualWidth(title)-5) + "┐"
	}
	drawText(scr, region.X, region.Y, defaultStyle(), top)
	for y := region.Y + 1; y < region.Y+region.Height-1; y++ {
		drawText(scr, region.X, y, defaultStyle(), "│")
		drawText(scr, region.X+region.Width-1, y, defaultStyle(), "│")
	}
	drawText(scr, region.X, region.Y+region.Height-1, defaultStyle(), bottom)
}

func joinLimited(values []string, limit int) string {
	if len(values) == 0 {
		return ""
	}
	if limit <= 0 || len(values) <= limit {
		return strings.Join(values, "/")
	}
	return strings.Join(values[:limit], "/") + "…"
}

func renderMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func renderMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
