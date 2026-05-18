package render

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// Border 统一边框样式。
type Border int

const (
	BorderNone   Border = iota // 无边框
	BorderSingle               // ┌─┐ 单线
	BorderDouble               // ╔═╗ 双线（结算弹窗专用）
)

// ListItem 是列表组件的一行。
type ListItem struct {
	Text     string
	Selected bool
}

// Card 是可选中卡片。
type Card struct {
	Title string
	Desc  string
	Hint  string
}

// ─── 基础文本绘制 ────────────────────────────────────

// DrawText 在 (x, y) 起始位置写入一行字符；返回写入后的下一列坐标。
func DrawText(scr tcell.Screen, x, y int, style tcell.Style, s string) int {
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

// DrawClippedText 在 (x, y) 写文本，超出 maxW 的字符替换为空格。
func DrawClippedText(scr tcell.Screen, x, y int, style tcell.Style, s string, maxW int) {
	if maxW <= 0 {
		return
	}
	w := VisualWidth(s)
	if w <= maxW {
		DrawText(scr, x, y, style, PadRightVisual(s, maxW))
		return
	}
	truncated := ClipVisual(s, maxW)
	DrawText(scr, x, y, style, PadRightVisual(truncated, maxW))
}

// CenterVisual 把文本按 visual width 居中到指定宽度；CJK 字符算 2 cell。
func CenterVisual(s string, width int) string {
	w := VisualWidth(s)
	if w >= width {
		return s
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + s
}

// ClipVisual 截断文本到指定 visual width。
func ClipVisual(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	w := 0
	for i, r := range s {
		w += VisualWidth(string(r))
		if w > maxW {
			return s[:i]
		}
	}
	return s
}

// VisualWidth 估算字符串的终端 cell 宽度；CJK 等宽字符算 2。
func VisualWidth(s string) int {
	return uniseg.StringWidth(s)
}

// MinInt 返回两个整数中的较小值。
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// PadRightVisual 把 s 右补空格到指定 cell 宽度。
func PadRightVisual(s string, width int) string {
	w := VisualWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// ─── 组件 ────────────────────────────────────────────

// DrawBox 绘制带标题的边框。
func DrawBox(scr tcell.Screen, r Region, border Border, title string, titleStyle tcell.Style) {
	if r.Width < 4 || r.Height < 3 {
		return
	}
	style := DefaultStyle()
	var ul, ur, ll, lr, hl, vl rune
	switch border {
	case BorderDouble:
		ul, ur, ll, lr, hl, vl = '╔', '╗', '╚', '╝', '═', '║'
	default:
		ul, ur, ll, lr, hl, vl = '┌', '┐', '└', '┘', '─', '│'
	}
	scr.SetContent(r.X, r.Y, ul, nil, style)
	scr.SetContent(r.X+r.Width-1, r.Y, ur, nil, style)
	for y := r.Y + 1; y < r.Y+r.Height-1; y++ {
		scr.SetContent(r.X, y, vl, nil, style)
		scr.SetContent(r.X+r.Width-1, y, vl, nil, style)
	}
	for x := r.X + 1; x < r.X+r.Width-1; x++ {
		scr.SetContent(x, r.Y, hl, nil, style)
		scr.SetContent(x, r.Y+r.Height-1, hl, nil, style)
	}
	scr.SetContent(r.X, r.Y+r.Height-1, ll, nil, style)
	scr.SetContent(r.X+r.Width-1, r.Y+r.Height-1, lr, nil, style)
	if title != "" && r.Width > 6 {
		DrawClippedText(scr, r.X+2, r.Y, titleStyle, " "+title+" ", r.Width-4)
	}
}

// DrawPanel 渲染居中带框面板。
func DrawPanel(scr tcell.Screen, scrW, scrH int, title string, lines []string) {
	width := MaxInt(40, scrW/2)
	if width > scrW-4 {
		width = scrW - 4
	}
	height := len(lines) + 4
	box := Region{
		X:     MaxInt(0, (scrW-width)/2),
		Y:     MaxInt(2, (scrH-height)/2),
		Width: width, Height: height,
	}
	DrawBox(scr, box, BorderSingle, title, Style(SemEmphasis))
	for i, line := range lines {
		DrawClippedText(scr, box.X+2, box.Y+2+i, DefaultStyle(), line, box.Width-4)
	}
}

// DrawDialog 渲染居中模态对话框。
func DrawDialog(scr tcell.Screen, center Region, title string, lines []string, border Border) {
	innerWidth := MinInt(48, center.Width-6)
	for _, line := range lines {
		if w := VisualWidth(line); w > innerWidth {
			innerWidth = w
		}
	}
	if innerWidth > center.Width-4 {
		innerWidth = center.Width - 4
	}
	if innerWidth < 24 {
		innerWidth = 24
	}
	width := innerWidth + 2
	height := len(lines) + 4
	x := center.X + (center.Width-width)/2
	y := center.Y + (center.Height-height)/2
	// 弹窗不得超出容器区域
	if x < center.X+1 {
		x = center.X + 1
	}
	if x+width > center.X+center.Width-1 {
		x = center.X + center.Width - 1 - width
		if x < center.X+1 {
			x = center.X + 1
			width = center.Width - 2
			innerWidth = width - 2
		}
	}
	if y < center.Y+1 {
		y = center.Y + 1
	}
	if y+height > center.Y+center.Height-1 {
		y = center.Y + center.Height - 1 - height
		if y < center.Y+1 {
			y = center.Y + 1
		}
	}
	r := Region{X: x, Y: y, Width: width, Height: height}
	DrawBox(scr, r, border, title, Style(SemEmphasis))
	for i, line := range lines {
		DrawClippedText(scr, r.X+1, r.Y+2+i, DefaultStyle(), PadRightVisual(line, innerWidth), innerWidth)
	}
}

// DrawList 渲染可选列表。
func DrawList(scr tcell.Screen, r Region, items []ListItem, selected int) {
	for i, item := range items {
		y := r.Y + i
		if y >= r.Y+r.Height {
			break
		}
		prefix := "  "
		st := DefaultStyle()
		if i == selected {
			prefix = "▶ "
			st = Style(SemEmphasis)
		}
		DrawClippedText(scr, r.X, y, st, prefix+item.Text, r.Width)
	}
}

// DrawCardGrid 渲染可选卡片网格。
func DrawCardGrid(scr tcell.Screen, x, y int, cards []Card, cardW, cardH int, selected int) {
	for i, card := range cards {
		cardX := x + i*(cardW+2)
		r := Region{X: cardX, Y: y, Width: cardW, Height: cardH}
		titleStyle := DefaultStyle()
		if i == selected {
			titleStyle = Style(SemEmphasis)
		}
		DrawBox(scr, r, BorderSingle, card.Title, titleStyle)
		if card.Desc != "" {
			DrawClippedText(scr, r.X+2, r.Y+2, DefaultStyle(), CenterVisual(card.Desc, r.Width-4), r.Width-4)
		}
		if card.Hint != "" {
			DrawClippedText(scr, r.X+2, r.Y+r.Height-2, DefaultStyle(), CenterVisual(card.Hint, r.Width-4), r.Width-4)
		}
	}
}

// DrawToast 渲染短暂通知消息（反转色）。
func DrawToast(scr tcell.Screen, r Region, msg string) {
	if msg == "" || r.Empty() {
		return
	}
	DrawClippedText(scr, r.X+2, r.Y, Style(SemEmphasis), msg, r.Width-4)
}

// DrawProgressBar 渲染填充进度条。
func DrawProgressBar(scr tcell.Screen, x, y, w int, fraction float64, label string) {
	barWidth := w - 8
	if barWidth < 6 {
		barWidth = 6
	}
	filled := int(float64(barWidth) * fraction)
	if filled < 0 {
		filled = 0
	}
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	text := fmt.Sprintf("%s  %s", bar, label)
	DrawText(scr, x, y, DefaultStyle(), text)
}

// DrawBandLine 画一条横带：左边文本 + 右边文本，支持反转色。
func DrawBandLine(scr tcell.Screen, r Region, left, right string, revLeft, revRight bool) {
	if r.Empty() {
		return
	}
	fillStyle := DefaultStyle()
	// 填充整行空格
	for xx := r.X; xx < r.X+r.Width; xx++ {
		scr.SetContent(xx, r.Y, ' ', nil, fillStyle)
	}
	left = ClipVisual(left, r.Width)
	if right == "" {
		st := fillStyle
		if revLeft {
			st = st.Reverse(true)
		}
		DrawClippedText(scr, r.X, r.Y, st, left, r.Width)
		return
	}
	right = ClipVisual(right, r.Width)
	leftW := VisualWidth(left)
	rightW := VisualWidth(right)
	gap := r.Width - leftW - rightW
	if gap < 1 {
		st := fillStyle
		if revLeft {
			st = st.Reverse(true)
		}
		DrawClippedText(scr, r.X, r.Y, st, left, r.Width)
		return
	}
	leftSt := fillStyle
	if revLeft {
		leftSt = leftSt.Reverse(true)
	}
	rightSt := fillStyle
	if revRight {
		rightSt = rightSt.Reverse(true)
	}
	DrawText(scr, r.X, r.Y, leftSt, left)
	DrawText(scr, r.X+r.Width-rightW, r.Y, rightSt, right)
}
