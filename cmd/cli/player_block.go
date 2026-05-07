package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

func drawPlayerBlock(scr tcell.Screen, region Region, view RoomView, seat int32, alignRight bool) {
	if region.Empty() || seat < 0 || seat > 3 {
		return
	}
	lines := playerBlockLines(view, seat)
	y := region.Y + region.Height/2 - len(lines)/2
	if y < region.Y {
		y = region.Y
	}
	focused := view.ActingSeat == seat && gameStarted(view)
	for i, line := range lines {
		if y+i >= region.Y+region.Height {
			break
		}
		style := defaultStyle()
		if focused && i == 0 {
			style = style.Reverse(true)
		}
		x := region.X
		if alignRight {
			x = region.X + renderMaxInt(0, region.Width-visualWidth(line))
		}
		drawClippedText(scr, x, y+i, style, line, region.Width)
	}
}

func playerBlockLines(view RoomView, seat int32) []string {
	p := view.Players[seat]
	name := playerDisplayName(view, seat)
	state := compactSeatState(view, seat)
	melds := formatMeldGlyphs(p.Melds)
	if melds == "" {
		melds = "-"
	}
	return []string{
		strings.TrimSpace(name + " " + state),
		fmt.Sprintf("缺%s  %d张", queLabel(view.QueBySeat[seat]), handCountForSeat(view, seat)),
		"副露: " + melds,
	}
}

func playerDisplayName(view RoomView, seat int32) string {
	if seat < 0 || seat > 3 {
		return "玩家"
	}
	p := view.Players[seat]
	if seat == view.SeatIndex {
		if view.Nickname != "" {
			return "★ " + view.Nickname
		}
		return "★ 你"
	}
	if p.Nickname != "" {
		return p.Nickname
	}
	if p.UserID != "" {
		return p.UserID
	}
	return seatLabelFallback(seat)
}

func compactSeatState(view RoomView, seat int32) string {
	if seat == view.ActingSeat && gameStarted(view) {
		return "▶思考中"
	}
	if view.QueBySeat[seat] >= 0 {
		return "已定缺"
	}
	if playerSeated(view.Players[seat]) || gameStarted(view) {
		return "等待"
	}
	return "空座"
}

func queLabel(que int32) string {
	switch que {
	case 0:
		return "萬"
	case 1:
		return "筒"
	case 2:
		return "条"
	default:
		return "未定"
	}
}

func formatMeldGlyphs(melds []string) string {
	if len(melds) == 0 {
		return ""
	}
	parts := make([]string, 0, len(melds))
	for _, raw := range melds {
		parts = append(parts, meldGlyph(raw))
	}
	return strings.Join(parts, " ")
}

func meldGlyph(raw string) string {
	i := strings.Index(raw, ":")
	if i <= 0 {
		return raw
	}
	kind := raw[:i]
	tile := strings.Fields(raw[i+1:])
	if len(tile) == 0 {
		return raw
	}
	label := "副露"
	count := 3
	switch kind {
	case "pong":
		label = "碰"
	case "gang":
		label = "杠"
		count = 4
	case "chow":
		label = "吃"
	}
	if kind == "chow" && len(tile) >= 3 {
		return strings.Join([]string{TileGlyph(tile[0]), TileGlyph(tile[1]), TileGlyph(tile[2])}, " ") + " " + label
	}
	glyphs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		glyphs = append(glyphs, TileGlyph(tile[0]))
	}
	return strings.Join(glyphs, " ") + " " + label
}
