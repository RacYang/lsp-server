package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// OverlayKind 标识当前打开的叠加层；同时只有一个叠加层处于打开态。
type OverlayKind int

const (
	// OverlayNone 没有叠加层，玩家直接看到牌桌主帧。
	OverlayNone OverlayKind = iota
	// OverlayRoomInfo 房间元数据叠加层（i 键）：房号、规则、人数、规则要点。
	OverlayRoomInfo
	// OverlayPlayers 玩家详情叠加层（Tab 键）：每家昵称、积分、状态。
	OverlayPlayers
	// OverlayMenu 局内菜单（Esc 键）：返回大厅或继续游戏。
	OverlayMenu
	// OverlayHelp 快速参考叠加层（? 键）：展示牌桌核心键位。
	OverlayHelp
)

// OverlayMenuAction 是局内菜单项可触发的高层动作。
type OverlayMenuAction string

const (
	OverlayMenuActionNone      OverlayMenuAction = ""
	OverlayMenuActionLeaveRoom OverlayMenuAction = "leave_room"
	OverlayMenuActionResume    OverlayMenuAction = "resume"
)

// OverlayState 维护当前叠加层种类与菜单选中项；非菜单类叠加层仅依赖 Kind。
type OverlayState struct {
	Kind          OverlayKind
	SelectedIndex int
}

// IsOpen 是否有叠加层处于打开态；调用方据此决定是否拦截普通牌桌按键。
func (o *OverlayState) IsOpen() bool { return o.Kind != OverlayNone }

// Toggle 在「打开/关闭」之间切换；如果当前是别的叠加层则切换到新种类。
func (o *OverlayState) Toggle(kind OverlayKind) {
	if o.Kind == kind {
		o.Close()
		return
	}
	o.Kind = kind
	o.SelectedIndex = 0
}

// Close 关闭当前叠加层。
func (o *OverlayState) Close() {
	o.Kind = OverlayNone
	o.SelectedIndex = 0
}

// MenuMove 在局内菜单中移动选中项；非菜单类叠加层忽略。
func (o *OverlayState) MenuMove(delta int) {
	if o.Kind != OverlayMenu {
		return
	}
	items := overlayMenuItems()
	n := len(items)
	if n == 0 {
		return
	}
	o.SelectedIndex = (o.SelectedIndex + delta + n) % n
}

// MenuSelect 返回当前选中菜单项对应的动作。
func (o *OverlayState) MenuSelect() OverlayMenuAction {
	if o.Kind != OverlayMenu {
		return OverlayMenuActionNone
	}
	items := overlayMenuItems()
	if o.SelectedIndex < 0 || o.SelectedIndex >= len(items) {
		return OverlayMenuActionNone
	}
	return items[o.SelectedIndex].Action
}

type overlayMenuItem struct {
	Label  string
	Action OverlayMenuAction
}

func overlayMenuItems() []overlayMenuItem {
	return []overlayMenuItem{
		{Label: "返回大厅", Action: OverlayMenuActionLeaveRoom},
		{Label: "继续游戏", Action: OverlayMenuActionResume},
	}
}

// OverlayContext 给叠加层提供 RoomView 之外的辅助上下文。
//
// RuleID 不在 RoomView 中（协议层用 RoomMeta 单独承载），由调用方根据当前 JoinResp 注入。
type OverlayContext struct {
	RuleID string
	Theme  TileTheme
}

// DrawOverlay 把当前叠加层画到屏幕上。
//
// 调用方应在 RenderFrame 之后调用本函数；OverlayNone 时直接返回。
func DrawOverlay(scr tcell.Screen, layout TableLayout, view RoomView, ctx OverlayContext, o OverlayState) {
	if !o.IsOpen() {
		return
	}
	switch o.Kind {
	case OverlayRoomInfo:
		drawOverlayBox(scr, layout, "房 间 信 息", overlayRoomInfoLines(view, ctx))
	case OverlayPlayers:
		drawOverlayBox(scr, layout, "玩 家 详 情", overlayPlayersLines(view))
	case OverlayMenu:
		drawOverlayMenu(scr, layout, o.SelectedIndex)
	case OverlayHelp:
		drawOverlayBox(scr, layout, "快 速 参 考", overlayHelpLines())
	}
}

// overlayRoomInfoLines 返回房间信息叠加层的内容行。
func overlayRoomInfoLines(view RoomView, ctx OverlayContext) []string {
	rule := ctx.RuleID
	if rule == "" {
		rule = "未知规则"
	}
	roomID := view.RoomID
	if roomID == "" {
		roomID = "(未进房)"
	}
	count := 0
	for _, p := range view.Players {
		if p.UserID != "" {
			count++
		}
	}
	return []string{
		"房号:    " + roomID,
		"规则:    " + rule,
		fmt.Sprintf("人数:    %d / 4", count),
		"主题:    " + ctx.Theme.String(),
		"",
		"按 i 关闭",
	}
}

// overlayPlayersLines 返回玩家详情叠加层的内容行。
func overlayPlayersLines(view RoomView) []string {
	lines := make([]string, 0, len(view.Players)+2)
	for i, p := range view.Players {
		nickname := p.Nickname
		if nickname == "" && p.UserID == "" {
			nickname = "(空座)"
		} else if nickname == "" {
			nickname = p.UserID
		}
		mark := " "
		if i == int(view.SeatIndex) {
			mark = "★"
		}
		ready := ""
		if p.Ready {
			ready = "  ✓ ready"
		}
		if p.IsBot {
			ready += "  [BOT]"
		}
		if p.Surrendered {
			ready += "  托管中"
		}
		lines = append(lines, fmt.Sprintf(" %s %d 号位  %-12s 手:%2d%s", mark, i+1, nickname, p.HandCnt, ready))
	}
	lines = append(lines, "")
	lines = append(lines, "按 Tab 关闭")
	return lines
}

func overlayHelpLines() []string {
	return []string{
		"←→ 选牌    Enter 出牌 / 确认",
		"换三张: 空格标记三张,Enter 提交",
		"定缺: m 万 / p 筒 / s 条",
		"碰杠胡: p 碰 / g 杠 / h 胡 / n 过",
		"机器人: waiting 阶段 b 补一个 / B 补满",
		"信息: i 房间信息 / Tab 玩家详情",
		"离桌: q 返回大厅 / Esc 菜单",
		"",
		"按 ? 或 Enter 关闭",
	}
}

func drawOverlayMenu(scr tcell.Screen, layout TableLayout, selectedIdx int) {
	items := overlayMenuItems()
	lines := make([]string, len(items)+2)
	for i, item := range items {
		prefix := "   "
		if i == selectedIdx {
			prefix = " ▶ "
		}
		lines[i] = prefix + item.Label
	}
	lines[len(items)] = ""
	lines[len(items)+1] = " ↑↓ 选项    Enter 确认    Esc 关闭"
	drawOverlayBox(scr, layout, "局 内 菜 单", lines)
}

// drawOverlayBox 是三种叠加层共用的"居中带框"绘制工具。
func drawOverlayBox(scr tcell.Screen, layout TableLayout, title string, lines []string) {
	innerWidth := minInt(48, layout.Width-6)
	for _, line := range lines {
		if w := visualWidth(line); w > innerWidth {
			innerWidth = w
		}
	}
	if innerWidth > layout.Width-4 {
		innerWidth = layout.Width - 4
	}
	if innerWidth < 24 {
		innerWidth = 24
	}
	width := innerWidth + 2
	height := len(lines) + 4 // top + title + sep + body + bottom
	x := layout.CenterArea.X + (layout.CenterArea.Width-width)/2
	y := layout.CenterArea.Y + (layout.CenterArea.Height-height)/2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	style := defaultStyle()
	drawText(scr, x, y, style, "┌"+strings.Repeat("─", width-2)+"┐")
	drawText(scr, x, y+1, style, "│")
	drawText(scr, x+1, y+1, style, padRightVisual(centerVisual(title, innerWidth), innerWidth))
	drawText(scr, x+width-1, y+1, style, "│")
	drawText(scr, x, y+2, style, "├"+strings.Repeat("─", width-2)+"┤")
	for i, line := range lines {
		drawText(scr, x, y+3+i, style, "│")
		drawText(scr, x+1, y+3+i, style, padRightVisual(line, innerWidth))
		drawText(scr, x+width-1, y+3+i, style, "│")
	}
	drawText(scr, x, y+3+len(lines), style, "└"+strings.Repeat("─", width-2)+"┘")
}
