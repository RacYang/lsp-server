package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

// SettlementOutcome 描述一局结束的整体结果。
type SettlementOutcome int

const (
	// SettlementOutcomeWin 你赢了（胡牌或剩牌后翻倍）。
	SettlementOutcomeWin SettlementOutcome = iota
	// SettlementOutcomeLose 你输了（点炮或被胡）。
	SettlementOutcomeLose
	// SettlementOutcomeDraw 流局（无人胡牌且牌墙耗尽）。
	SettlementOutcomeDraw
)

// SettlementFan 是一行番种的可读描述。
type SettlementFan struct {
	Name       string
	Multiplier int
}

// SettlementScore 描述结算时一家的得失分。
type SettlementScore struct {
	Nickname string
	Delta    int
	IsSelf   bool
}

// SettlementSummary 是从服务端结果转译过来的玩家可读结构。
//
// 它在 RoomView 之上做了一层"为人看的"翻译：把番种英文 ID 翻成中文、把座位翻成昵称。
// 同一份 SettlementSummary 同时驱动浮窗渲染与离桌后的 stdout 摘要。
type SettlementSummary struct {
	RoomID     string
	RuleID     string
	Outcome    SettlementOutcome
	WinnerID   string
	WinnerNick string
	WinTile    string
	Fans       []SettlementFan
	TotalFan   int
	Scores     []SettlementScore
}

// SettlementDialogState 是结算浮窗的逐行揭晓状态。
//
// VisibleLines 表示当前已揭晓的"行"数；调用方按 RevealInterval 推进。
// 完整行数由 totalRevealLines 派生：标题 + 每个番种 1 行 + 总番分隔 1 行 + 每家分数 1 行 + 提示 1 行。
type SettlementDialogState struct {
	Summary        SettlementSummary
	OpenedAt       time.Time
	RevealInterval time.Duration
}

// NewSettlementDialog 构造结算浮窗状态。
//
// 当 revealInterval <= 0 时退化为「一次性全部展示」，方便测试用 0 间隔走完所有揭晓状态。
func NewSettlementDialog(summary SettlementSummary, openedAt time.Time, revealInterval time.Duration) *SettlementDialogState {
	return &SettlementDialogState{Summary: summary, OpenedAt: openedAt, RevealInterval: revealInterval}
}

// totalRevealLines 是结算所有揭晓行的总数：标题 (1) + 番种 (n) + 总番分隔 (1) + 每家分数 (m) + 提示 (1)。
func (d *SettlementDialogState) totalRevealLines() int {
	return 1 + len(d.Summary.Fans) + 1 + len(d.Summary.Scores) + 1
}

// VisibleLines 返回当前应该显示的行数；由 OpenedAt 与 RevealInterval 共同决定。
func (d *SettlementDialogState) VisibleLines(now time.Time) int {
	if d.RevealInterval <= 0 {
		return d.totalRevealLines()
	}
	elapsed := now.Sub(d.OpenedAt)
	if elapsed < 0 {
		return 1
	}
	n := int(elapsed/d.RevealInterval) + 1
	if n > d.totalRevealLines() {
		n = d.totalRevealLines()
	}
	return n
}

// AllRevealed 报告是否已经走完所有揭晓行，调用方据此关闭"按 Enter 继续"提示。
func (d *SettlementDialogState) AllRevealed(now time.Time) bool {
	return d.VisibleLines(now) >= d.totalRevealLines()
}

// settlementTitle 根据 Outcome 选不同标题。
func (d *SettlementDialogState) title() string {
	switch d.Summary.Outcome {
	case SettlementOutcomeWin:
		return "胡 了 !"
	case SettlementOutcomeLose:
		return "输 了"
	case SettlementOutcomeDraw:
		return "流 局"
	}
	return "本 局 结 束"
}

// renderLines 输出浮窗内部内容（不含外部框线）。
//
// 行可见性按 VisibleLines 决定；尚未揭晓的行仍占位但显示空白，避免浮窗大小跳变。
func (d *SettlementDialogState) renderLines(now time.Time, innerWidth int) []string {
	visible := d.VisibleLines(now)
	all := d.allLines(innerWidth)
	out := make([]string, len(all))
	for i := range all {
		if i < visible {
			out[i] = all[i]
		} else {
			out[i] = strings.Repeat(" ", innerWidth)
		}
	}
	return out
}

// allLines 返回结算的全部行（不分可见性），让 VisibleLines 控制揭晓节奏。
func (d *SettlementDialogState) allLines(innerWidth int) []string {
	lines := []string{centerVisual(d.title(), innerWidth)}
	for _, fan := range d.Summary.Fans {
		lines = append(lines, centerVisual(fmt.Sprintf("%s   + %d", fan.Name, fan.Multiplier), innerWidth))
	}
	lines = append(lines, centerVisual(fmt.Sprintf("──── 共 %d 番 ────", d.Summary.TotalFan), innerWidth))
	for _, s := range d.Summary.Scores {
		sign := ""
		if s.Delta > 0 {
			sign = "+"
		}
		label := s.Nickname
		if s.IsSelf {
			label = "你"
		}
		lines = append(lines, centerVisual(fmt.Sprintf("%s   %s%d", label, sign, s.Delta), innerWidth))
	}
	lines = append(lines, centerVisual("Enter 继续  /  q 离桌", innerWidth))
	return lines
}

// DrawSettlementDialog 在中央区域绘制结算浮窗。
func DrawSettlementDialog(scr tcell.Screen, layout TableLayout, d *SettlementDialogState, now time.Time) {
	if d == nil {
		return
	}
	innerWidth := minInt(40, layout.Width-6)
	if innerWidth < 28 {
		innerWidth = 28
	}
	lines := d.renderLines(now, innerWidth)
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
	drawText(scr, x, y, style, "╭"+strings.Repeat("─", width-2)+"╮")
	for i, line := range lines {
		drawText(scr, x, y+1+i, style, "│")
		drawText(scr, x+1, y+1+i, style, padRightVisual(line, innerWidth))
		drawText(scr, x+width-1, y+1+i, style, "│")
	}
	drawText(scr, x, y+1+len(lines), style, "╰"+strings.Repeat("─", width-2)+"╯")
}

// WriteStdoutSummary 把结算摘要写到 io.Writer (生产里是 os.Stdout)。
//
// 离桌后玩家会回到 lobby，结算浮窗已经消失；摘要打到 stdout 留在 scrollback 供回看。
func WriteStdoutSummary(w io.Writer, sum SettlementSummary) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintln(w, "== 本局摘要 ==")
	if sum.RoomID != "" {
		_, _ = fmt.Fprintf(w, "房间: %s  规则: %s\n", sum.RoomID, sum.RuleID)
	}
	switch sum.Outcome {
	case SettlementOutcomeWin:
		fanNames := make([]string, len(sum.Fans))
		for i, f := range sum.Fans {
			fanNames[i] = f.Name
		}
		_, _ = fmt.Fprintf(w, "胜者: 你 (%s, %d 番)\n", strings.Join(fanNames, " + "), sum.TotalFan)
	case SettlementOutcomeLose:
		if sum.WinnerNick != "" {
			_, _ = fmt.Fprintf(w, "胜者: %s\n", sum.WinnerNick)
		}
	case SettlementOutcomeDraw:
		_, _ = fmt.Fprintln(w, "流局")
	}
	for _, s := range sum.Scores {
		label := s.Nickname
		if s.IsSelf {
			label = "你"
		}
		sign := ""
		if s.Delta > 0 {
			sign = "+"
		}
		_, _ = fmt.Fprintf(w, "%s: %s%d   ", label, sign, s.Delta)
	}
	_, _ = fmt.Fprintln(w)
}
