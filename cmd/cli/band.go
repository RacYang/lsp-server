package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func drawTitleBar(scr tcell.Screen, in FrameInputs) {
	ux := DeriveTableUXModel(in.View, in.Cursor, in.Now)
	left := fmt.Sprintf("%s · %s · %s · 剩%s", ruleLabel(in.View), roomLabel(in.View), phaseLabel(ux.Phase), remainingLabel(in.View))
	right := scoreSummary(in.View)
	drawBandLine(scr, in.Layout.TitleBar, left, right, false, false)
}

func drawNorthBand(scr tcell.Screen, in FrameInputs) {
	if in.View.SeatIndex < 0 || in.View.SeatIndex > 3 {
		drawBandLine(scr, in.Layout.NorthBand, "▼ 等待入座", focusSummary(in.View, in.Now), false, false)
		return
	}
	seat := relativeSeatIndex(in.View.SeatIndex, SeatPosTop)
	drawBandLine(scr, in.Layout.NorthBand, "▼ "+compactPlayerLine(in.View, seat), focusSummary(in.View, in.Now), focusOnSeat(in.View, in.Cursor, seat), false)
}

func drawSouthBand(scr tcell.Screen, in FrameInputs) {
	if in.View.SeatIndex < 0 || in.View.SeatIndex > 3 {
		drawBandLine(scr, in.Layout.SouthBand, "▲ 等待入座", bottomActionHint(in.View, in.Cursor, in.Now), false, false)
		return
	}
	ux := DeriveTableUXModel(in.View, in.Cursor, in.Now)
	drawBandLine(scr, in.Layout.SouthBand, "▲ "+compactPlayerLine(in.View, in.View.SeatIndex), ux.PrimaryPrompt, focusOnSeat(in.View, in.Cursor, in.View.SeatIndex), false)
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
		seatStatusMark(p) + name,
		"缺" + queLabel(view.QueBySeat[seat]),
		fmt.Sprintf("%d张", handCountForSeat(view, seat)),
	}
	if p.TotalScore != 0 {
		parts = append(parts, fmt.Sprintf("%+d", p.TotalScore))
	}
	if melds := formatMeldGlyphs(p.Melds); melds != "" {
		parts = append(parts, melds)
	}
	return strings.Join(parts, "·")
}

func focusSummary(view RoomView, now time.Time) string {
	if !gameStarted(view) {
		return ""
	}
	ux := DeriveTableUXModel(view, nil, now)
	seat := view.ActingSeat
	var action string
	switch ux.Phase {
	case PhaseExchange:
		seat = view.SeatIndex
		action = "换三张"
	case PhaseQueMen:
		seat = view.SeatIndex
		action = "定缺"
	case PhaseClaim:
		action = "等待响应"
	case PhaseMyTurnIdle, PhaseMyTurnSelected, PhaseDiscard:
		action = "出牌中"
	case PhaseOtherTurn:
		if view.RoundPhase == clientv1.Phase_PHASE_DRAW {
			action = "摸牌中"
		} else {
			action = "出牌中"
		}
	default:
		action = phaseLabel(ux.Phase)
	}
	if seat < 0 || seat > 3 {
		return ""
	}
	out := "▶ " + sideLabel(view, seat) + " " + playerDisplayName(view, seat) + " " + action
	if left, ok := actionCountdown(view, now); ok {
		out += fmt.Sprintf(" · %02ds", left)
	}
	return out
}

func bottomActionHint(view RoomView, cursor *HandCursor, now time.Time) string {
	ux := DeriveTableUXModel(view, cursor, now)
	if left, ok := actionCountdown(view, now); ok {
		if ux.PrimaryPrompt != "" {
			return fmt.Sprintf("%s  %02ds", ux.PrimaryPrompt, left)
		}
	}
	return ux.PrimaryPrompt
}

func actionCountdown(view RoomView, now time.Time) (int, bool) {
	if now.IsZero() {
		return 0, false
	}
	if view.DeadlineUnixMS > 0 {
		// 倒计时按服务端时间计算（详见 ADR-0045）：
		// 客户端可能笔记本休眠、跨时区、容器与宿主时钟偏移，本地 time.Now 不可信；
		// PhaseUpdate.server_now_unix_ms 维护的 ServerClockOffsetMS 是唯一权威修正。
		serverNowMS := now.UnixMilli() + view.ServerClockOffsetMS
		leftMS := view.DeadlineUnixMS - serverNowMS
		if leftMS < 0 {
			leftMS = 0
		}
		return int(math.Ceil(float64(leftMS) / 1000.0)), true
	}
	if view.ActionStartedAt.IsZero() {
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

func focusOnSeat(view RoomView, cursor *HandCursor, seat int32) bool {
	if seat < 0 || !gameStarted(view) {
		return false
	}
	if seat == view.SeatIndex && selfHasActionFocus(view, cursor) {
		return true
	}
	return seat == view.ActingSeat
}

func selfHasActionFocus(view RoomView, cursor *HandCursor) bool {
	switch DerivePhase(view, cursor) {
	case PhaseExchange, PhaseQueMen, PhaseClaim, PhaseTsumo, PhaseMyTurnIdle, PhaseMyTurnSelected:
		return true
	}
	switch DeriveInteractionModel(view).Phase {
	case PhaseExchange, PhaseQueMen, PhaseClaim, PhaseTsumo:
		return true
	default:
		return false
	}
}

func sideLabel(view RoomView, seat int32) string {
	switch RelativeSeat(view.SeatIndex, seat) {
	case SeatPosBottom:
		return "我"
	case SeatPosTop:
		return "对家"
	case SeatPosLeft:
		return "下家"
	case SeatPosRight:
		return "上家"
	default:
		return "玩家"
	}
}

func remainingLabel(view RoomView) string {
	if view.WallRemaining > 0 {
		return fmt.Sprintf("%d", view.WallRemaining)
	}
	if n, ok := remainingTilesEstimate(view); ok {
		return fmt.Sprintf("%d", n)
	}
	return "--"
}

func scoreSummary(view RoomView) string {
	if !gameStarted(view) {
		return ""
	}
	if len(view.TotalScores) > 0 {
		parts := make([]string, 0, len(view.TotalScores))
		for _, score := range view.TotalScores {
			parts = append(parts, fmt.Sprintf("%s %+d", sideLabel(view, score.GetSeatIndex()), score.GetTotalFan()))
		}
		return strings.Join(parts, "  ")
	}
	return fmt.Sprintf("第%d局 第%d手", view.RoundIndex+1, view.HandIndex+1)
}

// [G12] 局内座位状态值域：● 在线 / ○ 离线 / ▲ 弃局 / ✓ 已胡 / ▣ 机器人 / □ 空座。
// 托管 feature 暂不推进，AutoPlay 字段不再参与 cli 渲染。
func seatStatusMark(p PlayerView) string {
	switch {
	case p.UserID == "":
		return "□"
	case p.Hued:
		return "✓"
	case p.Surrendered:
		return "▲"
	case p.IsBot:
		return "▣"
	case !p.Online:
		return "○"
	default:
		return "●"
	}
}
