package main

import (
	"fmt"
	"strings"
	"time"

	"racoo.cn/lsp/cmd/cli/render"
)

// ClaimTrigger 描述触发 claim 浮窗的事件类型。
type ClaimTrigger int

const (
	ClaimTriggerSelfDraw ClaimTrigger = iota
	ClaimTriggerRon
	ClaimTriggerPong
	ClaimTriggerGang
	ClaimTriggerChow
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
type ClaimDialogState struct {
	Trigger       ClaimTrigger
	TriggerSeat   int32
	TriggerName   string
	Tile          string
	Actions       []ClaimAction
	SelectedIndex int
	OpenedAt      time.Time
	Deadline      time.Time
	Pending       bool
}

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

func (d *ClaimDialogState) Move(delta int) {
	if d.Pending || len(d.Actions) == 0 {
		return
	}
	n := len(d.Actions)
	d.SelectedIndex = (d.SelectedIndex + delta + n) % n
}

func (d *ClaimDialogState) Selected() ClaimAction {
	if d.SelectedIndex < 0 || d.SelectedIndex >= len(d.Actions) {
		return ClaimActionPass
	}
	return d.Actions[d.SelectedIndex]
}

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

func (d *ClaimDialogState) Expired(now time.Time) bool {
	return !now.Before(d.Deadline)
}

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
		if trigger == ClaimTriggerSelfDraw {
			return "不胡"
		}
		return "过"
	}
	return string(a)
}

// claimDialogLines 返回浮窗内容行（不含外部框线）。
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
		totalLen += render.VisualWidth(buttons[i])
	}
	gap := 2
	totalLen += gap * (len(buttons) - 1)
	pad := (innerWidth - totalLen) / 2
	if pad < 0 {
		pad = 0
	}
	buttonLine := strings.Repeat(" ", pad) + strings.Join(buttons, strings.Repeat(" ", gap))

	progressLine := claimProgressBar(d, now, innerWidth)
	hint := "快捷键: h胡  g杠  p碰  n过  ←→切换  Enter 确认"
	if d.Pending {
		hint = "已提交，等待服务端确认..."
	}
	return []string{
		centerVisual(title, innerWidth),
		"",
		buttonLine,
		"",
		centerVisual(progressLine, innerWidth),
		centerVisual(hint, innerWidth),
	}
}

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

func centerVisual(s string, width int) string {
	w := render.VisualWidth(s)
	if w >= width {
		return s
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + s
}
