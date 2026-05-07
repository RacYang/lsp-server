package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

func drawTitleBar(scr tcell.Screen, in FrameInputs) {
	left := fmt.Sprintf("%s · %s · %s · 剩%s", ruleLabel(in.View), roomLabel(in.View), phaseLabel(DerivePhase(in.View, in.Cursor)), remainingLabel(in.View))
	right := scoreSummary(in.View)
	drawBandLine(scr, in.Layout.TitleBar, left, right, false, false)
}

func drawNorthBand(scr tcell.Screen, in FrameInputs) {
	if in.View.SeatIndex < 0 || in.View.SeatIndex > 3 {
		drawBandLine(scr, in.Layout.NorthBand, "▼ 等待入座", focusSummary(in.View, in.Now), false, false)
		return
	}
	seat := relativeSeatIndex(in.View.SeatIndex, SeatPosTop)
	drawBandLine(scr, in.Layout.NorthBand, "▼ "+compactPlayerLine(in.View, seat), focusSummary(in.View, in.Now), focusOnSeat(in.View, seat), false)
}

func drawSouthBand(scr tcell.Screen, in FrameInputs) {
	if in.View.SeatIndex < 0 || in.View.SeatIndex > 3 {
		drawBandLine(scr, in.Layout.SouthBand, "▲ 等待入座", bottomActionHint(in.View, in.Cursor, in.Now), false, false)
		return
	}
	drawBandLine(scr, in.Layout.SouthBand, "▲ "+compactPlayerLine(in.View, in.View.SeatIndex), bottomActionHint(in.View, in.Cursor, in.Now), focusOnSeat(in.View, in.View.SeatIndex), false)
}

func drawKeybar(scr tcell.Screen, in FrameInputs) {
	hint := bottomHint(in.View, in.Cursor)
	if hint == "" {
		hint = "Esc 菜单    ? 帮助"
	}
	drawBandLine(scr, in.Layout.KeyBar, hint, "", false, false)
}

func compactPlayerLine(view RoomView, seat int32) string {
	if seat < 0 || seat > 3 {
		return "玩家"
	}
	p := view.Players[seat]
	name := playerDisplayName(view, seat)
	parts := []string{
		name,
		"缺" + queLabel(view.QueBySeat[seat]),
		fmt.Sprintf("%d张", handCountForSeat(view, seat)),
	}
	if melds := formatMeldGlyphs(p.Melds); melds != "" {
		parts = append(parts, melds)
	}
	return strings.Join(parts, "·")
}

func focusSummary(view RoomView, now time.Time) string {
	if view.ActingSeat < 0 || view.ActingSeat > 3 || !gameStarted(view) {
		return ""
	}
	out := "▶ " + sideLabel(view, view.ActingSeat) + " " + playerDisplayName(view, view.ActingSeat) + " 出牌中"
	if left, ok := actionCountdown(view, now); ok {
		out += fmt.Sprintf(" · %02ds", left)
	}
	return out
}

func bottomActionHint(view RoomView, cursor *HandCursor, now time.Time) string {
	phase := DerivePhase(view, cursor)
	countdown := ""
	if left, ok := actionCountdown(view, now); ok {
		countdown = fmt.Sprintf("  %02ds", left)
	}
	switch phase {
	case PhaseMyTurnIdle, PhaseMyTurnSelected:
		return "◆ 该你出牌 ◆" + countdown
	case PhaseExchange:
		return "◆ 换三张 ◆" + countdown
	case PhaseQueMen:
		return "◆ 定缺 ◆" + countdown
	case PhaseClaim, PhaseTsumo:
		return "◆ 请决定 ◆" + countdown
	default:
		return ""
	}
}

func actionCountdown(view RoomView, now time.Time) (int, bool) {
	if now.IsZero() || view.ActionStartedAt.IsZero() {
		return 0, false
	}
	var total time.Duration
	switch view.WaitingAction {
	case "exchange_three", "que_men", "discard":
		total = 15 * time.Second
	case "claim_window", "tsumo_window":
		total = 5 * time.Second
	default:
		return 0, false
	}
	left := total - now.Sub(view.ActionStartedAt)
	if left < 0 {
		left = 0
	}
	return int(math.Ceil(left.Seconds())), true
}

func focusOnSeat(view RoomView, seat int32) bool {
	return seat >= 0 && seat == view.ActingSeat && gameStarted(view)
}

func sideLabel(view RoomView, seat int32) string {
	switch RelativeSeat(view.SeatIndex, seat) {
	case SeatPosBottom:
		return "南家"
	case SeatPosTop:
		return "北家"
	case SeatPosLeft:
		return "西家"
	case SeatPosRight:
		return "东家"
	default:
		return "玩家"
	}
}

func remainingLabel(view RoomView) string {
	if n, ok := remainingTilesEstimate(view); ok {
		return fmt.Sprintf("%d", n)
	}
	return "--"
}

func scoreSummary(view RoomView) string {
	// 协议尚未提供实时筹码；保留天凤式位置，避免用假分数误导。
	if !gameStarted(view) {
		return ""
	}
	return "你 --  西 --  北 --  东 --"
}
