package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// FrameInputs 把渲染单帧需要的所有外部状态打包，便于在测试里固定成纯函数。
type FrameInputs struct {
	View   RoomView
	Layout TableLayout
	Theme  TileTheme
	Cursor *HandCursor // 玩家当前选中的手牌；为 nil 表示未启用光标。
}

// RenderFrame 按 inputs 把整个牌桌画到 scr 上；调用方负责 scr.Show()。
//
// 这一层只做"幂等的位置→字符"映射，不维护任何状态；所有状态（光标、浮窗）
// 都通过 FrameInputs 传入，让测试可以直接构造任意场景的 golden。
func RenderFrame(scr tcell.Screen, inputs FrameInputs) {
	scr.Clear()
	drawStatusBar(scr, inputs)
	drawTopArea(scr, inputs)
	drawLeftRightAreas(scr, inputs)
	drawCenterInfo(scr, inputs)
	drawSelfArea(scr, inputs)
	drawKeybar(scr, inputs)
}

// drawText 在 (x, y) 起始位置写入一行字符；返回写入后的下一列坐标。
//
// 按 grapheme cluster 遍历并复用 uniseg 给出的 width:
//   - 全角中日韩字符、全角标点（如「（」「：」）等宽字符占 2 列;
//   - 普通 ASCII 与半角字符占 1 列;
//   - 组合字符（width=0）会作为前一 cluster 的附加 rune 写入同一 cell,
//     不再单独占列,避免老实现按 rune 计宽导致全角符号错位。
func drawText(scr tcell.Screen, x, y int, style tcell.Style, s string) int {
	col := x
	state := -1
	rest := s
	for len(rest) > 0 {
		var cluster string
		var width int
		cluster, rest, width, state = uniseg.FirstGraphemeClusterInString(rest, state)
		runes := []rune(cluster)
		if len(runes) == 0 {
			continue
		}
		scr.SetContent(col, y, runes[0], runes[1:], style)
		col += width
	}
	return col
}

func defaultStyle() tcell.Style { return tcell.StyleDefault }

// 顶部对家区域：行 0 名字 + 简短打过牌；行 1 隐藏手牌行；行 2 鸣牌行。
func drawTopArea(scr tcell.Screen, in FrameInputs) {
	region := in.Layout.TopArea
	if region.Empty() {
		return
	}
	seat := relativeSeatIndex(in.View.SeatIndex, SeatPosTop)
	if seat < 0 {
		return
	}
	player := in.View.Players[seat]
	nameLine := centerVisual(seatLabel("对家", player), region.Width)
	drawText(scr, region.X, region.Y, defaultStyle(), nameLine)

	hidden := HiddenTilesRow(player.HandCnt, in.Theme)
	drawText(scr, region.X, region.Y+1, defaultStyle(), centerVisual(hidden, region.Width))

	mLine := formatMelds(player.Melds)
	if mLine != "" {
		drawText(scr, region.X, region.Y+2, defaultStyle(), centerVisual("鸣: "+mLine, region.Width))
	}
	drawDiscardPile(scr, Region{X: region.X, Y: region.Y + 3, Width: region.Width, Height: region.Height - 3}, "打: ", player.Discards, in.Layout.DiscardColumns, defaultStyle())
}

// 左右家：分两栏并排显示，每栏 3 行。
//
// 注意：左侧位置是出牌顺序的下家 (self+1)，右侧是上家 (self+3)，
// 与雀魂等主流客户端布局一致。
func drawLeftRightAreas(scr tcell.Screen, in FrameInputs) {
	drawSidePlayer(scr, in, in.Layout.LeftArea, SeatPosLeft, "下家")
	drawSidePlayer(scr, in, in.Layout.RightArea, SeatPosRight, "上家")
}

func drawSidePlayer(scr tcell.Screen, in FrameInputs, region Region, pos SeatPosition, label string) {
	if region.Empty() {
		return
	}
	seat := relativeSeatIndex(in.View.SeatIndex, pos)
	if seat < 0 {
		return
	}
	player := in.View.Players[seat]
	drawText(scr, region.X, region.Y, defaultStyle(), seatLabel(label, player))
	drawText(scr, region.X, region.Y+1, defaultStyle(), HiddenTilesRow(player.HandCnt, in.Theme))
	if mLine := formatMelds(player.Melds); mLine != "" {
		drawText(scr, region.X, region.Y+2, defaultStyle(), "鸣: "+mLine)
	}
	drawDiscardPile(scr, Region{X: region.X, Y: region.Y + 3, Width: region.Width, Height: region.Height - 3}, "打: ", player.Discards, in.Layout.DiscardColumns, defaultStyle())
}

// drawSelfArea 统一渲染自家牌河、鸣牌与手牌，避免 B0 后自家区域被拆散到多个调用点。
func drawSelfArea(scr tcell.Screen, in FrameInputs) {
	drawSelfDiscardsArea(scr, in)
	drawSelfMeldsArea(scr, in)
	drawHandArea(scr, in)
}

// drawSelfMeldsArea 在手牌正上方渲染玩家自己的鸣牌（碰/杠/吃），
// 与其他三家的"鸣: ..."信息保持一致；没有鸣牌时整行留空。
func drawSelfMeldsArea(scr tcell.Screen, in FrameInputs) {
	region := in.Layout.SelfMeldsArea
	if region.Empty() {
		return
	}
	if in.View.SeatIndex < 0 || in.View.SeatIndex > 3 {
		return
	}
	melds := in.View.Players[in.View.SeatIndex].Melds
	mLine := formatMelds(melds)
	if mLine == "" {
		return
	}
	drawText(scr, region.X, region.Y, defaultStyle(), centerVisual("鸣: "+mLine, region.Width))
}

// drawSelfDiscardsArea 在自家鸣牌行正上方渲染玩家自己的牌河（最近若干弃张）,
// 与其他三家"打: ..."信息保持一致;没有弃张时整行留空,layout 占位仍保留以避免重排版。
func drawSelfDiscardsArea(scr tcell.Screen, in FrameInputs) {
	region := in.Layout.SelfDiscardsArea
	if region.Empty() {
		return
	}
	if in.View.SeatIndex < 0 || in.View.SeatIndex > 3 {
		return
	}
	discards := in.View.Players[in.View.SeatIndex].Discards
	drawDiscardPile(scr, region, "打: ", discards, in.Layout.DiscardColumns, defaultStyle())
}

// 自己的手牌：用 tile_art 渲染每张牌，光标处的牌整体上移一行（凸起）。
func drawHandArea(scr tcell.Screen, in FrameInputs) {
	region := in.Layout.HandArea
	if region.Empty() {
		return
	}
	if in.View.SeatIndex < 0 || in.View.SeatIndex > 3 {
		return
	}
	hand := in.View.Players[in.View.SeatIndex].Hand
	if len(hand) == 0 {
		return
	}
	tiles := make([]TileArt, len(hand))
	for i, t := range hand {
		tiles[i] = RenderTile(t, in.Theme)
	}
	totalWidth := len(tiles) * TileArtWidth
	startX := region.X + (region.Width-totalWidth)/2
	if startX < region.X {
		startX = region.X
	}
	for i, tile := range tiles {
		col := startX + i*TileArtWidth
		yOffset := 0
		style := defaultStyle()
		if in.Cursor != nil {
			cursorOn := in.Cursor.Index == i
			marked := in.Cursor.IsMarked(i)
			if cursorOn || marked {
				yOffset = -1 // 凸起：光标牌或已标记牌都上移一行
			}
			if cursorOn {
				style = style.Reverse(true)
			} else if marked {
				style = style.Foreground(tcell.ColorYellow)
			}
			if in.Cursor.Pending && (cursorOn || marked) {
				style = style.Foreground(tcell.ColorGray)
			}
		}
		for r := 0; r < TileArtHeight; r++ {
			drawText(scr, col, region.Y+r+yOffset, style, tile.Lines[r])
		}
	}
}

// relativeSeatIndex 把目标方位反查回绝对座位号；玩家未入座或方位不合法时返回 -1。
//
// 与 RelativeSeat 保持互逆：left 是下家 (self+1)，right 是上家 (self+3)。
func relativeSeatIndex(selfSeat int32, pos SeatPosition) int32 {
	if selfSeat < 0 || selfSeat > 3 {
		return -1
	}
	switch pos {
	case SeatPosBottom:
		return selfSeat
	case SeatPosLeft:
		return (selfSeat + 1) % 4
	case SeatPosTop:
		return (selfSeat + 2) % 4
	case SeatPosRight:
		return (selfSeat + 3) % 4
	}
	return -1
}

func seatLabel(prefix string, p PlayerView) string {
	name := p.Nickname
	if name == "" {
		if p.UserID != "" {
			name = p.UserID
		} else {
			name = "(空座)"
		}
	}
	if p.IsBot {
		name += " [BOT]"
	}
	if p.Surrendered {
		name += " (托管中)"
	}
	return prefix + " " + name
}

func formatMelds(melds []string) string {
	if len(melds) == 0 {
		return ""
	}
	parts := make([]string, 0, len(melds))
	for _, m := range melds {
		parts = append(parts, prettifyMeld(m))
	}
	return strings.Join(parts, "  ")
}

// prettifyMeld 把 state 里 "pong:5p" / "gang:1m" 形式的内部记法转成 "[5p]碰"。
func prettifyMeld(raw string) string {
	if i := strings.Index(raw, ":"); i > 0 {
		kind := raw[:i]
		tile := raw[i+1:]
		label := kind
		switch kind {
		case "pong":
			label = "碰"
		case "gang":
			label = "杠"
		case "chow":
			label = "吃"
		}
		return "[" + prettifyTileList(tile) + "]" + label
	}
	return raw
}

func prettifyTileList(raw string) string {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return TileName(raw)
	}
	for i, part := range parts {
		parts[i] = TileName(part)
	}
	return strings.Join(parts, " ")
}

// centralPrompt 是中央区域的玩家可读提示语，纯函数便于单测。
func centralPrompt(view RoomView, cursor *HandCursor) string {
	model := DeriveInteractionModel(view)
	if cursor != nil && cursor.Mode == CursorModeMulti3 && view.SeatIndex >= 0 {
		need := 3 - len(cursor.Marked)
		if cursor.Pending {
			return "→ 提交中..."
		}
		if need > 0 {
			return fmt.Sprintf("请用 Space 标记换 3 张牌 (还需 %d)", need)
		}
		return "已选 3 张, 按 Enter 提交  /  Esc 取消"
	}
	if cursor != nil && cursor.Mode == CursorModeSingle && cursor.Index >= 0 && view.SeatIndex >= 0 {
		hand := view.Players[view.SeatIndex].Hand
		if cursor.Index < len(hand) {
			tile := TileName(hand[cursor.Index])
			if cursor.Pending {
				return fmt.Sprintf("→ 出牌中... %s", tile)
			}
			return fmt.Sprintf("已选 %s,按 Enter 出牌  /  Esc 取消", tile)
		}
	}
	if view.SeatIndex >= 0 && view.SeatIndex == view.ActingSeat {
		switch model.Phase {
		case PhaseDiscard:
			return "◆ 该 你 出牌 ◆"
		case PhaseQueMen:
			return "请定缺 (m / p / s)"
		case PhaseExchange:
			return "请选择三张换三张"
		case PhaseClaim, PhaseTsumo:
			return "请决定"
		}
	}
	if view.ActingSeat >= 0 && view.ActingSeat < 4 {
		name := view.Players[view.ActingSeat].Nickname
		if name == "" {
			name = fmt.Sprintf("%d 号位", view.ActingSeat+1)
		}
		return fmt.Sprintf("等待 %s", name)
	}
	if view.Phase == phaseTable {
		if emptySeatCount(view) > 0 {
			return "座位未满 - b 补一个 / B 补满"
		}
		return "已自动准备,等待其他玩家就位"
	}
	return "等待开始"
}

// bottomHint 给最下面的提示行输出操作引导。
func bottomHint(view RoomView, cursor *HandCursor) string {
	model := DeriveInteractionModel(view)
	if cursor != nil && cursor.Mode == CursorModeMulti3 && !cursor.Pending {
		return "←→ 选牌    Space 标记/取消    Enter 提交    Esc 取消    i 房间信息"
	}
	if cursor != nil && cursor.Mode == CursorModeSingle && cursor.Index >= 0 && !cursor.Pending {
		return "←→ 选牌    Enter 出牌    Esc 取消    i 房间信息"
	}
	if view.SeatIndex >= 0 && view.SeatIndex == view.ActingSeat && model.Phase == PhaseDiscard {
		return "←→ 选牌    Enter 出牌    i 房间信息    Esc 菜单"
	}
	if emptySeatCount(view) > 0 {
		return "b 补 1 个机器人    B 补满    Enter 等真人    Esc 菜单"
	}
	return "i 房间信息    Tab 查看玩家    Esc 菜单"
}

// centerVisual 把文本按 visual width 居中到指定宽度；CJK 字符算 2 cell。
//
// 与旧 layout.go 中的 centerText 区别在于：centerText 用 rune 数计算（不分宽度），
// 这里精确按 cell。新的牌桌渲染统一用 centerVisual，避免双宽字符错位。
func centerVisual(s string, width int) string {
	w := visualWidth(s)
	if w >= width {
		return s
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + s
}
