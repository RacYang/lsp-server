package main

import "github.com/gdamore/tcell/v2"

// drawSeatTiles 只在桌内画 4 家手牌带/牌墙带，不混入任何玩家文字。
func drawSeatTiles(scr tcell.Screen, in FrameInputs) {
	if in.View.SeatIndex < 0 || in.View.SeatIndex > 3 {
		return
	}
	drawNorthHand(scr, in)
	drawSideWall(scr, in.Layout.Slots.WestWall, handCountForSeat(in.View, relativeSeatIndex(in.View.SeatIndex, SeatPosLeft)))
	drawSideWall(scr, in.Layout.Slots.EastWall, handCountForSeat(in.View, relativeSeatIndex(in.View.SeatIndex, SeatPosRight)))
	drawSouthHand(scr, in)
}

func drawNorthHand(scr tcell.Screen, in FrameInputs) {
	seat := relativeSeatIndex(in.View.SeatIndex, SeatPosTop)
	count := handCountForSeat(in.View, seat)
	drawHiddenTileLine(scr, in.Layout.Slots.NorthHand, count)
}

func drawSouthHand(scr tcell.Screen, in FrameInputs) {
	seat := in.View.SeatIndex
	if seat < 0 || seat > 3 {
		return
	}
	hand := in.View.Players[seat].Hand
	if len(hand) == 0 {
		return
	}
	region := in.Layout.Slots.SouthHand
	x := region.X
	queSuit := in.View.QueBySeat[seat]
	for i, tile := range hand {
		offset, style := tileStyle(in.Cursor, i)
		// [Q2.2] 自家手牌：缺门花色一律灰显，提示「优先打缺门」；
		// 光标 / Marked / Pending 三种高亮覆盖缺门灰，避免与正在操作的牌互相吃掉。
		if isQueSuitTile(tile, queSuit) && !cursorHighlightedAt(in.Cursor, i) {
			style = style.Foreground(tcell.ColorGray)
		}
		if marker := handSelectionMarker(in.Cursor, i); marker != "" && region.Y > 0 {
			drawText(scr, x+i*3, region.Y-1, style, marker)
		}
		drawTileGlyph(scr, x+i*3, region.Y+offset, style, TileGlyph(tile))
	}
}

// isQueSuitTile 判断 tile 是否落在 que 花色上；que=-1 / tile 非 m/p/s 时返回 false。
func isQueSuitTile(tile string, que int32) bool {
	if que < 0 || que > 2 || len(tile) == 0 {
		return false
	}
	switch tile[0] {
	case 'm':
		return que == 0
	case 'p':
		return que == 1
	case 's':
		return que == 2
	}
	return false
}

// cursorHighlightedAt 让 [Q2.2] 灰显规则在「玩家正在操作」的格子上让位，
// 避免缺门灰把光标 / Marked / Pending 的高亮吃掉。
func cursorHighlightedAt(cursor *HandCursor, idx int) bool {
	if cursor == nil {
		return false
	}
	return cursor.Index == idx || cursor.IsMarked(idx)
}

func handSelectionMarker(cursor *HandCursor, idx int) string {
	if cursor == nil {
		return ""
	}
	if cursor.IsMarked(idx) {
		return "◆"
	}
	if cursor.Index == idx {
		return "▲"
	}
	return ""
}

func drawHiddenTileLine(scr tcell.Screen, region Region, count int) {
	if region.Empty() || count <= 0 {
		return
	}
	if count > 14 {
		count = 14
	}
	for i := 0; i < count; i++ {
		drawTileGlyph(scr, region.X+i*3, region.Y, defaultStyle(), HiddenGlyph())
	}
}

func drawSideWall(scr tcell.Screen, region Region, count int) {
	if region.Empty() || count <= 0 {
		return
	}
	if count > region.Height {
		count = region.Height
	}
	for i := 0; i < count; i++ {
		drawTileGlyph(scr, region.X, region.Y+i, defaultStyle(), HiddenGlyph())
	}
}

func handCountForSeat(view RoomView, seat int32) int {
	if seat < 0 || seat > 3 {
		return 0
	}
	p := view.Players[seat]
	if len(p.Hand) > 0 {
		return len(p.Hand)
	}
	if p.HandCnt > 0 {
		return p.HandCnt
	}
	if gameStarted(view) || playerSeated(p) {
		return defaultStartingHandSize
	}
	return 0
}
