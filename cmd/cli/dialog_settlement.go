package main

import (
	"fmt"
	"io"
	"strings"
	"time"
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

// SettlementWinner 是单个胡家在结算中的可读视图（[S3.1]）。
//
// 多家胡时（一炮多响 / 血战连续胡）每位胡家应当独立展示，避免合并成「赢家=第一个胡者 + 所有番种」
// 这种把玩家弄混的表达。
type SettlementWinner struct {
	Nickname string
	IsSelf   bool
	Fan      int
	FanNames []string
}

// SettlementPenalty 是查花猪 / 查大叫 / 退税等罚分的可读视图（[S4.1]）。
type SettlementPenalty struct {
	Reason   string
	FromNick string
	ToNick   string
	Amount   int
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
	// Winners 多家胡时按服务端 per_winner_breakdown 顺序展开，每条独立显示（[S3.1]）。
	Winners []SettlementWinner
	// Penalties 流局或查叫罚分（[S4.1]）。
	Penalties []SettlementPenalty
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

// totalRevealLines 是结算所有揭晓行的总数：
// 标题 (1) + 番种 (n) + 多胡家 (w) + 流局罚分 (p) + 总番分隔 (1) + 每家分数 (m)。
func (d *SettlementDialogState) totalRevealLines() int {
	return 1 + len(d.Summary.Fans) + len(d.Summary.Winners) + len(d.Summary.Penalties) + 1 + len(d.Summary.Scores)
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

// AllRevealed 报告是否已经走完所有揭晓行。
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

// allLines 返回结算的全部行（不分可见性），让 VisibleLines 控制揭晓节奏。
func (d *SettlementDialogState) allLines(innerWidth int) []string {
	lines := []string{centerVisual(d.title(), innerWidth)}
	for _, fan := range d.Summary.Fans {
		lines = append(lines, centerVisual(fmt.Sprintf("%s   + %d", fan.Name, fan.Multiplier), innerWidth))
	}
	// [S3.1] 多家胡：每位胡家独立展示，避免和「Fans 总集合」混在一起辨认不出归属。
	for _, w := range d.Summary.Winners {
		label := w.Nickname
		if w.IsSelf {
			label = "你"
		}
		names := strings.Join(w.FanNames, "、")
		if names == "" {
			names = "—"
		}
		lines = append(lines, centerVisual(fmt.Sprintf("胡 · %s   %d 番 · %s", label, w.Fan, names), innerWidth))
	}
	// [S4.1] 流局罚分 / 查叫 / 退税独立显示。
	for _, p := range d.Summary.Penalties {
		lines = append(lines, centerVisual(fmt.Sprintf("罚 · %s  %s→%s  %d", p.Reason, p.FromNick, p.ToNick, p.Amount), innerWidth))
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
	// [S5.1] 键位提示由场景层 key bar 统一渲染，不在此处重复。
	return lines
}

// DrawSettlementDialog 已移除——结算绘制由 scene_table.go 通过 render.DrawDialog(BorderDouble) 处理。

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
	// [S3.1] 多家胡：stdout 摘要按服务端 per_winner_breakdown 顺序逐家打印。
	for _, win := range sum.Winners {
		label := win.Nickname
		if win.IsSelf {
			label = "你"
		}
		names := strings.Join(win.FanNames, " + ")
		if names == "" {
			names = "—"
		}
		_, _ = fmt.Fprintf(w, "胡: %s  %d 番  %s\n", label, win.Fan, names)
	}
	// [S4.1] 流局/查叫罚分独立列出。
	for _, p := range sum.Penalties {
		_, _ = fmt.Fprintf(w, "罚: %s  %s→%s  %d\n", p.Reason, p.FromNick, p.ToNick, p.Amount)
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
