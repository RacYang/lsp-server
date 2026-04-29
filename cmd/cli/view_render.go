package main

import (
	"fmt"
	"strings"
)

func RenderLines(rv RoomView, opts RenderOptions) []string {
	width := opts.Width
	if width < 96 {
		width = 96
	}
	var out []string
	out = append(out, borderLine(width, "="))
	out = append(out, centerText(fmt.Sprintf("room=%s user=%s seat=%d stage=%s acting=%d rtt=%dms", valueOr(rv.RoomID, "-"), valueOr(rv.UserID, "-"), rv.SeatIndex, rv.Stage, rv.ActingSeat, rv.RTTms), width))
	out = append(out, borderLine(width, "-"))
	out = append(out, renderWideSeat("北家", relativeSeat(rv.SeatIndex, 2), rv, opts, width)...)
	mid := renderMiddle(rv, opts, width)
	out = append(out, mid...)
	out = append(out, renderWideSeat("南家(你)", rv.SeatIndex, rv, opts, width)...)
	out = append(out, borderLine(width, "-"))
	out = append(out, renderLogs(rv, width)...)
	out = append(out, borderLine(width, "="))
	return out
}

func renderWideSeat(label string, seat int32, rv RoomView, opts RenderOptions, width int) []string {
	if seat < 0 || seat > 3 {
		return []string{centerText(label+" 未入座", width)}
	}
	p := rv.Players[seat]
	title := fmt.Sprintf("%s seat=%d user=%s 缺=%s", label, seat, valueOr(p.UserID, "-"), suitName(rv.QueBySeat[seat], opts))
	lines := []string{centerText(title, width)}
	if seat == rv.SeatIndex {
		lines = append(lines, prefixLines("手: ", tileRow(p.Hand, true, opts))...)
	} else {
		lines = append(lines, "手: "+backStack(maxInt(p.HandCnt, 13), opts))
	}
	lines = append(lines, "副: "+strings.Join(defaultStrings(p.Melds), " "))
	lines = append(lines, prefixLines("弃: ", tileGrid(p.Discards, width-4, opts))...)
	return lines
}

func renderMiddle(rv RoomView, opts RenderOptions, width int) []string {
	sideW := width / 4
	centerW := width - sideW*2 - 4
	west := renderSideSeat("西家", relativeSeat(rv.SeatIndex, 1), rv, opts, sideW)
	center := renderCenter(rv, centerW)
	east := renderSideSeat("东家", relativeSeat(rv.SeatIndex, 3), rv, opts, sideW)
	maxRows := maxInt(len(west), maxInt(len(center), len(east)))
	out := make([]string, 0, maxRows)
	for i := 0; i < maxRows; i++ {
		out = append(out, padRight(lineAt(west, i), sideW)+"  "+padRight(lineAt(center, i), centerW)+"  "+padRight(lineAt(east, i), sideW))
	}
	return out
}

func renderSideSeat(label string, seat int32, rv RoomView, opts RenderOptions, width int) []string {
	if seat < 0 || seat > 3 {
		return []string{label + " 未入座"}
	}
	p := rv.Players[seat]
	lines := []string{
		label + " seat=" + itoa(int(seat)),
		"user=" + valueOr(p.UserID, "-"),
		"缺=" + suitName(rv.QueBySeat[seat], opts),
		"手: " + backStack(maxInt(p.HandCnt, 13), opts),
		"副: " + strings.Join(defaultStrings(p.Melds), " "),
	}
	lines = append(lines, prefixLines("弃: ", tileGrid(p.Discards, width-4, opts))...)
	return lines
}

func renderCenter(rv RoomView, width int) []string {
	lines := []string{
		centerText("牌桌中心", width),
		"庄家: " + itoa(int(rv.DealerSeat)),
		"当前: " + itoa(int(rv.ActingSeat)),
		"阶段: " + valueOr(rv.Stage, "-"),
		"待牌: " + valueOr(rv.PendingTile, "-"),
		"可选: " + strings.Join(defaultStrings(rv.AvailableAction), "/"),
	}
	if rv.LastSettlement != nil {
		lines = append(lines, "结算: "+valueOr(rv.LastSettlement.GetDetailText(), "已完成"))
	}
	if rv.LastError != "" {
		lines = append(lines, "错误: "+rv.LastError)
	}
	return lines
}

func renderLogs(rv RoomView, width int) []string {
	logs := rv.Log
	if len(logs) > 6 {
		logs = logs[len(logs)-6:]
	}
	out := []string{"事件:"}
	for _, item := range logs {
		out = append(out, padRight("  "+item.At.Format("15:04:05")+" "+item.Text, width))
	}
	return out
}

func prefixLines(prefix string, lines []string) []string {
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if i == 0 {
			out = append(out, prefix+line)
			continue
		}
		out = append(out, strings.Repeat(" ", len([]rune(prefix)))+line)
	}
	return out
}

func relativeSeat(self int32, offset int32) int32 {
	if self < 0 || self > 3 {
		return -1
	}
	return (self + offset) % 4
}

func suitName(suit int32, opts RenderOptions) string {
	if opts.CJKTiles {
		switch suit {
		case 0:
			return "万"
		case 1:
			return "条"
		case 2:
			return "筒"
		default:
			return "-"
		}
	}
	switch suit {
	case 0:
		return "m"
	case 1:
		return "s"
	case 2:
		return "p"
	default:
		return "-"
	}
}

func borderLine(width int, ch string) string {
	return strings.Repeat(ch, width)
}

func lineAt(lines []string, idx int) string {
	if idx < 0 || idx >= len(lines) {
		return ""
	}
	return lines[idx]
}

func defaultStrings(items []string) []string {
	if len(items) == 0 {
		return []string{"-"}
	}
	return items
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
