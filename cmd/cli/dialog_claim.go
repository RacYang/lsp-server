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
	tile := claimTileName(d.Tile)
	switch d.Trigger {
	case ClaimTriggerSelfDraw:
		return fmt.Sprintf("你摸到 %s", tile)
	case ClaimTriggerRon:
		return fmt.Sprintf("%s 打出 %s", displayTrigger(d), tile)
	case ClaimTriggerPong:
		return fmt.Sprintf("%s 打出 %s", displayTrigger(d), tile)
	case ClaimTriggerGang:
		return fmt.Sprintf("%s 打出 %s", displayTrigger(d), tile)
	case ClaimTriggerChow:
		return fmt.Sprintf("%s 打出 %s", displayTrigger(d), tile)
	case ClaimTriggerPongOrHu:
		return fmt.Sprintf("%s 打出 %s", displayTrigger(d), tile)
	}
	return "请决定这一手"
}

func claimTileName(tile string) string {
	if tile == "" {
		return "这张牌"
	}
	name := TileName(tile)
	if name == "??" {
		return tile
	}
	return name
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
	lines := []string{centerVisual(d.title(), innerWidth), ""}
	for i, a := range d.Actions {
		prefix := "  "
		if i == d.SelectedIndex {
			prefix = "▶ "
		}
		lines = append(lines, centerVisual(prefix+claimActionSentence(a, d.Trigger), innerWidth))
	}
	lines = append(lines, "", centerVisual(claimProgressBar(d, now, innerWidth), innerWidth))
	hint := "现在：←→ 选择　Enter 确认"
	if d.Pending {
		hint = "已经递出决定，等牌桌回应"
	}
	lines = append(lines, centerVisual(hint, innerWidth))
	return lines
}

func claimActionSentence(a ClaimAction, trigger ClaimTrigger) string {
	label := claimActionLabel(a, trigger)
	switch a {
	case ClaimActionHu:
		if trigger == ClaimTriggerSelfDraw {
			return label + "：宣告自摸"
		}
		return label + "：就这张牌和牌"
	case ClaimActionPong:
		return label + "：拿下这张牌"
	case ClaimActionGang:
		return label + "：开杠"
	case ClaimActionChow:
		return label + "：吃进这张牌"
	case ClaimActionPass:
		if trigger == ClaimTriggerSelfDraw {
			return label + "：留在手里继续打"
		}
		return label + "：放过这次机会"
	}
	return label
}

func claimProgressBar(d *ClaimDialogState, now time.Time, innerWidth int) string {
	barWidth := innerWidth - 12
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
	return fmt.Sprintf("还剩 %.1f 秒  %s", left.Seconds(), bar)
}

func centerVisual(s string, width int) string {
	w := render.VisualWidth(s)
	if w >= width {
		return s
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + s
}
