package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// FrameInputs 把渲染单帧需要的所有外部状态打包，便于在测试里固定成纯函数。
type FrameInputs struct {
	View   RoomView
	Layout TableLayout
	Theme  TileTheme
	Cursor *HandCursor // 玩家当前选中的手牌；为 nil 表示未启用光标。
	Now    time.Time
}

// RenderFrame 把雀魂式居中牌桌画到 scr 上；调用方负责 scr.Show()。
func RenderFrame(scr tcell.Screen, inputs FrameInputs) {
	if inputs.Now.IsZero() {
		inputs.Now = time.Now()
	}
	scr.Clear()
	drawTitleBar(scr, inputs)
	drawNorthBand(scr, inputs)
	drawTableEdge(scr, inputs.Layout.TableFrame)
	drawSeatTiles(scr, inputs)
	drawPonds(scr, inputs)
	drawCenterDial(scr, inputs)
	drawSidePlayerBlocks(scr, inputs)
	drawSouthBand(scr, inputs)
	drawKeybar(scr, inputs)
}

// drawText 在 (x, y) 起始位置写入一行字符；返回写入后的下一列坐标。
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

func drawTileGlyph(scr tcell.Screen, x, y int, style tcell.Style, glyph string) {
	width := visualWidth(glyph)
	if width < 1 {
		width = 1
	}
	drawText(scr, x, y, style, strings.Repeat(" ", width))
	drawText(scr, x, y, style, glyph)
}

func drawPonds(scr tcell.Screen, in FrameInputs) {
	if in.View.SeatIndex < 0 || in.View.SeatIndex > 3 {
		return
	}
	slots := in.Layout.Slots
	drawPondMatrix(scr, slots.NorthPond, PondDown, discardsForRelative(in.View, SeatPosTop), 6, defaultStyle())
	drawPondMatrix(scr, slots.WestPond, PondRight, discardsForRelative(in.View, SeatPosLeft), 6, defaultStyle())
	drawPondMatrix(scr, slots.EastPond, PondLeft, discardsForRelative(in.View, SeatPosRight), 6, defaultStyle())
	drawPondMatrix(scr, slots.SouthPond, PondUp, discardsForRelative(in.View, SeatPosBottom), 6, defaultStyle())
}

func discardsForRelative(view RoomView, pos SeatPosition) []string {
	seat := relativeSeatIndex(view.SeatIndex, pos)
	if seat < 0 {
		return nil
	}
	return view.Players[seat].Discards
}

func drawSidePlayerBlocks(scr tcell.Screen, in FrameInputs) {
	if in.View.SeatIndex < 0 || in.View.SeatIndex > 3 {
		return
	}
	drawPlayerBlock(scr, in.Layout.LeftPlayerSlot, in.View, relativeSeatIndex(in.View.SeatIndex, SeatPosLeft), false)
	drawPlayerBlock(scr, in.Layout.RightPlayerSlot, in.View, relativeSeatIndex(in.View.SeatIndex, SeatPosRight), true)
}

// relativeSeatIndex 把目标方位反查回绝对座位号；玩家未入座或方位不合法时返回 -1。
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
	phase := DerivePhase(view, cursor)
	if phase == PhaseExchange && cursor != nil && cursor.Mode == CursorModeMulti3 && view.SeatIndex >= 0 {
		need := 3 - len(cursor.Marked)
		if cursor.Pending {
			return "→ 提交中..."
		}
		if need > 0 {
			return fmt.Sprintf("请用 Space 标记换 3 张牌 (还需 %d)", need)
		}
		return "已选 3 张, 按 Enter 提交  /  Esc 取消"
	}
	if phase == PhaseMyTurnSelected && cursor != nil && cursor.Mode == CursorModeSingle && cursor.Index >= 0 && view.SeatIndex >= 0 {
		hand := view.Players[view.SeatIndex].Hand
		if cursor.Index < len(hand) {
			tile := TileName(hand[cursor.Index])
			if cursor.Pending {
				return fmt.Sprintf("→ 出牌中... %s", tile)
			}
			return fmt.Sprintf("已选 %s,按 Enter 出牌  /  Esc 取消", tile)
		}
	}
	switch phase {
	case PhaseMyTurnIdle:
		switch model.Phase {
		case PhaseDiscard:
			return "◆ 该你出牌 ◆"
		case PhaseQueMen:
			return "请定缺 (m / p / s)"
		case PhaseExchange:
			return "请选择三张换三张"
		case PhaseClaim, PhaseTsumo:
			return "请决定"
		}
	case PhaseClaim:
		return "请决定"
	case PhaseOtherTurn:
		name := view.Players[view.ActingSeat].Nickname
		if name == "" {
			name = fmt.Sprintf("%d 号位", view.ActingSeat+1)
		}
		return fmt.Sprintf("等待 %s", name)
	case PhaseWaiting:
		if emptySeatCount(view) > 0 {
			return "座位未满 - b 补一个 / B 补满"
		}
		return "已自动准备,等待其他玩家就位"
	}
	return "等待开始"
}

// bottomHint 给最下面的提示行输出操作引导。
func bottomHint(view RoomView, cursor *HandCursor) string {
	phase := DerivePhase(view, cursor)
	if phase == PhaseExchange && cursor != nil && cursor.Mode == CursorModeMulti3 && !cursor.Pending {
		return "←→ 选牌    Space 标记/取消    Enter 提交    Esc 取消    i 房间信息"
	}
	if phase == PhaseMyTurnSelected && cursor != nil && cursor.Mode == CursorModeSingle && cursor.Index >= 0 && !cursor.Pending {
		return "←→ 选牌    Enter 出牌    Esc 取消    i 房间信息"
	}
	switch phase {
	case PhaseMyTurnIdle:
		return "←→ 选牌    Enter 出牌    i 房间信息    Esc 菜单"
	case PhaseClaim:
		return "-- 鸣牌 --  ←→ 选项    Enter 确认    P 跳过"
	case PhaseSettlement:
		return "-- 结算 --  R 再来一局    L 离桌    Enter 停留"
	case PhaseWaiting:
		if emptySeatCount(view) > 0 {
			return "b 补 1 个机器人    B 补满    Enter 等真人    Esc 菜单"
		}
		return "Tab 查看玩家    i 房间信息    ? 帮助    Esc 菜单"
	default:
		return "Tab 查看玩家    i 房间信息    ? 帮助    Esc 菜单"
	}
}

// centerVisual 把文本按 visual width 居中到指定宽度；CJK 字符算 2 cell。
func centerVisual(s string, width int) string {
	w := visualWidth(s)
	if w >= width {
		return s
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + s
}
