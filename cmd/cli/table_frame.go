package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// drawTableEdge 只画唯一的牌桌边界；全屏其他区域不再画方框。
func drawTableEdge(scr tcell.Screen, region Region) {
	if region.Width < 4 || region.Height < 4 {
		return
	}
	style := defaultStyle()
	drawText(scr, region.X, region.Y, style, "┌"+strings.Repeat("─", region.Width-2)+"┐")
	for y := region.Y + 1; y < region.Y+region.Height-1; y++ {
		drawText(scr, region.X, y, style, "│")
		drawText(scr, region.X+region.Width-1, y, style, "│")
	}
	drawText(scr, region.X, region.Y+region.Height-1, style, "└"+strings.Repeat("─", region.Width-2)+"┘")
}

func drawCenterDial(scr tcell.Screen, in FrameInputs) {
	region := in.Layout.Slots.Dial
	if region.Empty() {
		return
	}
	label := "--"
	if in.View.WallRemaining > 0 {
		label = fmt.Sprintf("%02d", in.View.WallRemaining)
	} else if n, ok := remainingTilesEstimate(in.View); ok {
		label = fmt.Sprintf("%02d", n)
	}
	text := label
	if in.View.LastAction != nil && in.View.LastAction.GetTile() != "" {
		text += " " + TileName(in.View.LastAction.GetTile())
	}
	if in.View.DealerSeat == in.View.SeatIndex && in.View.SeatIndex >= 0 {
		text += " ★"
	}
	drawClippedText(scr, region.X, region.Y, defaultStyle(), centerVisual(text, region.Width), region.Width)
}

func remainingTilesEstimate(view RoomView) (int, bool) {
	if !gameStarted(view) {
		return 0, false
	}
	used := 0
	for _, p := range view.Players {
		if len(p.Hand) > 0 {
			used += len(p.Hand)
		} else {
			used += p.HandCnt
		}
		used += len(p.Discards)
		for _, meld := range p.Melds {
			if strings.HasPrefix(meld, "gang:") {
				used += 4
			} else {
				used += 3
			}
		}
	}
	remaining := 108 - used
	if remaining < 0 {
		remaining = 0
	}
	return remaining, true
}

func drawBandLine(scr tcell.Screen, region Region, left, right string, reverseLeft bool, reverseRight bool) {
	if region.Empty() {
		return
	}
	fillStyle := defaultStyle()
	drawText(scr, region.X, region.Y, fillStyle, strings.Repeat(" ", region.Width))
	left = clipVisual(left, region.Width)
	if right == "" {
		style := fillStyle
		if reverseLeft {
			style = style.Reverse(true)
		}
		drawClippedText(scr, region.X, region.Y, style, left, region.Width)
		return
	}
	right = clipVisual(right, region.Width)
	leftWidth := visualWidth(left)
	rightWidth := visualWidth(right)
	gap := region.Width - leftWidth - rightWidth
	if gap < 1 {
		style := fillStyle
		if reverseLeft {
			style = style.Reverse(true)
		}
		drawClippedText(scr, region.X, region.Y, style, left, region.Width)
		return
	}
	leftStyle := fillStyle
	if reverseLeft {
		leftStyle = leftStyle.Reverse(true)
	}
	rightStyle := fillStyle
	if reverseRight {
		rightStyle = rightStyle.Reverse(true)
	}
	drawText(scr, region.X, region.Y, leftStyle, left)
	drawText(scr, region.X+region.Width-rightWidth, region.Y, rightStyle, right)
}

func tileStyle(cursor *HandCursor, idx int) (int, tcell.Style) {
	offset := 0
	style := defaultStyle()
	if cursor == nil {
		return offset, style
	}
	cursorOn := cursor.Index == idx
	marked := cursor.IsMarked(idx)
	if cursorOn {
		style = style.Bold(true).Underline(true)
	} else if marked {
		style = style.Bold(true)
	}
	if cursor.Pending && (cursorOn || marked) {
		style = style.Foreground(tcell.ColorGray)
	}
	return offset, style
}
