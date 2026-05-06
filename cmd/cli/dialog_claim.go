package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

// ClaimTrigger 描述触发 claim 浮窗的事件类型，决定标题模板。
type ClaimTrigger int

const (
	// ClaimTriggerSelfDraw 自摸：标题为「你 自 摸 了 !」，按钮 [胡] [不胡]。
	ClaimTriggerSelfDraw ClaimTrigger = iota
	// ClaimTriggerRon 别家打出可胡的牌：标题为「胡 X 打出的 牌」，按钮 [胡] [过]。
	ClaimTriggerRon
	// ClaimTriggerPong 别家打出可碰的牌：标题为「碰 X 打出的 牌」，按钮 [碰] [过]。
	ClaimTriggerPong
	// ClaimTriggerGang 可杠：标题为「杠 X 打出的 牌」，按钮 [杠] [过]。
	ClaimTriggerGang
	// ClaimTriggerChow 可吃：标题为「吃 X 打出的 牌」，按钮 [吃] [过]。
	ClaimTriggerChow
	// ClaimTriggerPongOrHu 同时可胡可碰：按钮 [胡] [碰] [过]。
	ClaimTriggerPongOrHu
)

// ClaimAction 是玩家在 claim 浮窗中可以做出的选择。
type ClaimAction string

const (
	ClaimActionHu   ClaimAction = "hu"
	ClaimActionPong ClaimAction = "pong"
	ClaimActionGang ClaimAction = "gang"
	ClaimActionChow ClaimAction = "chow"
	ClaimActionPass ClaimAction = "pass"
)

// ClaimDialogState 是 claim 浮窗的有限状态机。
//
// OpenedAt 与 Deadline 由调用方注入，便于在测试中用确定时间驱动进度条。
// SelectedIndex 在 [0, len(Actions)-1] 之间循环；进入 Pending 后忽略输入。
type ClaimDialogState struct {
	Trigger       ClaimTrigger
	TriggerSeat   int32
	TriggerName   string
	Tile          string
	Actions       []ClaimAction
	SelectedIndex int
	OpenedAt      time.Time
	Deadline      time.Time
	Pending       bool // 已发送选择,等待服务端 ack
}

// NewClaimDialog 构造一个新的 claim 浮窗状态。
//
// triggerName 用于荣胡场景下显示"胡 alice 打出的 4 万"，自摸场景忽略。
func NewClaimDialog(trigger ClaimTrigger, triggerSeat int32, triggerName, tile string, actions []ClaimAction, openedAt time.Time, timeout time.Duration) *ClaimDialogState {
	if len(actions) == 0 {
		actions = []ClaimAction{ClaimActionPass}
	}
	return &ClaimDialogState{
		Trigger:     trigger,
		TriggerSeat: triggerSeat,
		TriggerName: triggerName,
		Tile:        tile,
		Actions:     actions,
		OpenedAt:    openedAt,
		Deadline:    openedAt.Add(timeout),
	}
}

// Move 在按钮列表中环绕移动；Pending 时忽略。
func (d *ClaimDialogState) Move(delta int) {
	if d.Pending || len(d.Actions) == 0 {
		return
	}
	n := len(d.Actions)
	d.SelectedIndex = (d.SelectedIndex + delta + n) % n
}

// Selected 返回当前高亮按钮对应的动作。
func (d *ClaimDialogState) Selected() ClaimAction {
	if d.SelectedIndex < 0 || d.SelectedIndex >= len(d.Actions) {
		return ClaimActionPass
	}
	return d.Actions[d.SelectedIndex]
}

// Progress 返回 [0, 1] 之间的剩余比例，1 表示尚未开始消耗，0 表示已用尽。
//
// 拆成纯函数便于测试不同 now 值下的进度条渲染。
func (d *ClaimDialogState) Progress(now time.Time) float64 {
	total := d.Deadline.Sub(d.OpenedAt)
	if total <= 0 {
		return 0
	}
	left := d.Deadline.Sub(now)
	if left <= 0 {
		return 0
	}
	if left >= total {
		return 1
	}
	return float64(left) / float64(total)
}

// Expired 是否已超过 Deadline；调用方据此自动选择 ClaimActionPass。
func (d *ClaimDialogState) Expired(now time.Time) bool {
	return !now.Before(d.Deadline)
}

// title 返回浮窗标题，根据 trigger 选不同模板。
func (d *ClaimDialogState) title() string {
	switch d.Trigger {
	case ClaimTriggerSelfDraw:
		return "你 自 摸 了 !"
	case ClaimTriggerRon:
		return fmt.Sprintf("胡 %s 打出的 %s", displayTrigger(d), d.Tile)
	case ClaimTriggerPong:
		return fmt.Sprintf("碰 %s 打出的 %s", displayTrigger(d), d.Tile)
	case ClaimTriggerGang:
		return fmt.Sprintf("杠 %s 打出的 %s", displayTrigger(d), d.Tile)
	case ClaimTriggerChow:
		return fmt.Sprintf("吃 %s 打出的 %s", displayTrigger(d), d.Tile)
	case ClaimTriggerPongOrHu:
		return fmt.Sprintf("胡/碰 %s 打出的 %s", displayTrigger(d), d.Tile)
	}
	return "请 决 定"
}

func displayTrigger(d *ClaimDialogState) string {
	if d.TriggerName != "" {
		return d.TriggerName
	}
	if d.TriggerSeat >= 0 && d.TriggerSeat < 4 {
		return fmt.Sprintf("%d 号位", d.TriggerSeat+1)
	}
	return "对家"
}

// claimActionLabel 把 ClaimAction 翻译成浮窗按钮上的中文文案。
func claimActionLabel(a ClaimAction, trigger ClaimTrigger) string {
	switch a {
	case ClaimActionHu:
		return "胡"
	case ClaimActionPong:
		return "碰"
	case ClaimActionGang:
		return "杠"
	case ClaimActionChow:
		return "吃"
	case ClaimActionPass:
		// 自摸场景 Pass 表达更友好的"不胡"，其它场景统一用"过"。
		if trigger == ClaimTriggerSelfDraw {
			return "不胡"
		}
		return "过"
	}
	return string(a)
}

// claimDialogLines 渲染浮窗的多行文本（不含外部框线）。
//
// 拆成纯函数让 golden 测试只关心字符串，不依赖 SimulationScreen。
func claimDialogLines(d *ClaimDialogState, now time.Time, innerWidth int) []string {
	if innerWidth < 20 {
		innerWidth = 20
	}
	title := d.title()
	buttons := make([]string, len(d.Actions))
	totalLen := 0
	for i, a := range d.Actions {
		label := claimActionLabel(a, d.Trigger)
		if i == d.SelectedIndex {
			buttons[i] = "[ " + label + " ]"
		} else {
			buttons[i] = "  " + label + "  "
		}
		totalLen += visualWidth(buttons[i])
	}
	gap := 2
	totalLen += gap * (len(buttons) - 1)
	pad := (innerWidth - totalLen) / 2
	if pad < 0 {
		pad = 0
	}
	buttonLine := strings.Repeat(" ", pad) + strings.Join(buttons, strings.Repeat(" ", gap))

	progressLine := claimProgressBar(d, now, innerWidth)
	return []string{
		centerVisual(title, innerWidth),
		"",
		buttonLine,
		"",
		centerVisual(progressLine, innerWidth),
	}
}

// claimProgressBar 把 Progress 比例转成 ████░░░  3.2s 形式的字符串。
func claimProgressBar(d *ClaimDialogState, now time.Time, innerWidth int) string {
	barWidth := innerWidth - 8
	if barWidth < 6 {
		barWidth = 6
	}
	p := d.Progress(now)
	filled := int(float64(barWidth) * p)
	if filled < 0 {
		filled = 0
	}
	if filled > barWidth {
		filled = barWidth
	}
	left := d.Deadline.Sub(now)
	if left < 0 {
		left = 0
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	return fmt.Sprintf("%s  %.1fs", bar, left.Seconds())
}

// DrawClaimDialog 把 claim 浮窗叠加渲染到屏幕中央区域。
//
// 调用方应在 RenderFrame 之后调用本函数，让浮窗覆盖底层内容。
func DrawClaimDialog(scr tcell.Screen, layout TableLayout, d *ClaimDialogState, now time.Time) {
	if d == nil {
		return
	}
	innerWidth := minInt(50, layout.Width-6)
	if innerWidth < 30 {
		innerWidth = 30
	}
	lines := claimDialogLines(d, now, innerWidth)
	height := len(lines) + 2
	width := innerWidth + 2
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
	for i, line := range lines {
		drawText(scr, x, y+1+i, style, "│")
		drawText(scr, x+1, y+1+i, style, padRightVisual(line, innerWidth))
		drawText(scr, x+width-1, y+1+i, style, "│")
	}
	drawText(scr, x, y+1+len(lines), style, "└"+strings.Repeat("─", width-2)+"┘")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// padRightVisual 把 s 右补空格到指定 cell 宽度，CJK 算 2 cell。
func padRightVisual(s string, width int) string {
	w := visualWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
