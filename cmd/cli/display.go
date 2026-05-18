package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"

	"racoo.cn/lsp/cmd/cli/render"
)

// ─── 牌名映射 ────────────────────────────────────────

// TileName 把协议层牌名转为玩家可读中文名。
func TileName(name string) string {
	return render.TileGlyph(name)
}

// ─── 座位与方位 ──────────────────────────────────────

// SeatPosition 表示某个玩家在屏幕上的相对方位（以"我"为南家固定坐标）。
type SeatPosition int

const (
	SeatPosBottom SeatPosition = iota // 自己（南家）
	SeatPosTop                        // 对家（北家）
	SeatPosLeft                       // 上家（西家）
	SeatPosRight                      // 下家（东家）
)

// RelativeSeat 把绝对座位（0=东 1=南 2=西 3=北）映射为以 selfSeat 为底部的相对方位。
func RelativeSeat(selfSeat, targetSeat int32) SeatPosition {
	if selfSeat < 0 || selfSeat > 3 || targetSeat < 0 || targetSeat > 3 {
		return SeatPosBottom
	}
	diff := (targetSeat - selfSeat + 4) % 4
	switch diff {
	case 0:
		return SeatPosBottom
	case 1:
		return SeatPosLeft
	case 2:
		return SeatPosTop
	case 3:
		return SeatPosRight
	}
	return SeatPosBottom
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

func windLabel(seat int) string {
	switch seat {
	case 0:
		return "南"
	case 1:
		return "西"
	case 2:
		return "北"
	case 3:
		return "东"
	default:
		return "?"
	}
}

// ─── 玩家状态 ────────────────────────────────────────

// [G12] 局内座位状态值域：● 在线 / √ 已准备 / ○ 离线 / ▲ 弃局 / ✓ 已胡 / ▣ 机器人 / □ 空座。
func seatStatusMark(p PlayerView) string {
	switch {
	case p.UserID == "":
		return "□"
	case p.Hued:
		return "✓"
	case p.Surrendered:
		return "▲"
	case p.Ready:
		return "√"
	case p.IsBot:
		return "▣"
	case !p.Online:
		return "○"
	default:
		return "●"
	}
}

func playerDisplayName(view RoomView, seat int32) string {
	if seat < 0 || seat > 3 {
		return "玩家"
	}
	p := view.Players[seat]
	if seat == view.SeatIndex {
		if view.Nickname != "" {
			return "★ " + view.Nickname
		}
		return "★ 你"
	}
	if p.Nickname != "" {
		return p.Nickname
	}
	if p.UserID != "" {
		return p.UserID
	}
	return seatLabelFallback(seat)
}

func seatLabelFallback(seat int32) string {
	return fmt.Sprintf("%d号位", seat+1)
}

func compactSeatState(view RoomView, seat int32) string {
	if seat == view.ActingSeat && gameStarted(view) {
		return "▶思考中"
	}
	if view.QueBySeat[seat] >= 0 {
		return "已定缺"
	}
	if playerSeated(view.Players[seat]) || gameStarted(view) {
		return "等待"
	}
	return "空座"
}

func playerSeated(p PlayerView) bool { return p.UserID != "" || p.Nickname != "" }

func queLabel(que int32) string {
	switch que {
	case 0:
		return "萬"
	case 1:
		return "筒"
	case 2:
		return "条"
	default:
		return "未定"
	}
}

func gameStarted(view RoomView) bool {
	switch view.RoomState {
	case "playing", "settling":
		return true
	}
	return view.LastSettlement != nil
}

func handCountForSeat(view RoomView, seat int32) int {
	if seat < 0 || seat > 3 {
		return 0
	}
	p := view.Players[seat]
	if len(p.Hand) > 0 {
		return len(p.Hand)
	}
	return p.HandCnt
}

func emptySeatCount(view RoomView) int32 {
	var n int32
	for _, player := range view.Players {
		if player.UserID == "" && player.Nickname == "" {
			n++
		}
	}
	return n
}

// ─── 副露 ────────────────────────────────────────────

func formatMeldGlyphs(melds []string) string {
	if len(melds) == 0 {
		return ""
	}
	parts := make([]string, 0, len(melds))
	for _, raw := range melds {
		parts = append(parts, meldGlyph(raw))
	}
	return strings.Join(parts, " ")
}

func meldGlyph(raw string) string {
	i := strings.Index(raw, ":")
	if i <= 0 {
		return raw
	}
	kind := raw[:i]
	tile := strings.Fields(raw[i+1:])
	if len(tile) == 0 {
		return raw
	}
	label := "副露"
	switch kind {
	case "pong":
		label = "碰"
	case "gang":
		label = "杠"
	case "chow":
		label = "吃"
	}
	if kind == "chow" && len(tile) >= 3 {
		return strings.Join([]string{
			render.TileGlyph(tile[0]),
			render.TileGlyph(tile[1]),
			render.TileGlyph(tile[2]),
		}, " ") + " " + label
	}
	return render.TileGlyph(tile[0]) + " " + label
}

// ─── 标签映射 ────────────────────────────────────────

var ruleDisplayNames = map[string]string{
	"default":                           "默认麻将",
	"blood":                             "川麻血战",
	"sichuan":                           "川麻血战",
	"sichuan_xz":                        "川麻血战",
	"sichuan_xuezhandaodi_huansanzhang": "川麻血战 (血战到底)",
	"sichuan_dx":                        "川麻倒下",
	"changsha":                          "长沙麻将",
	"japanese":                          "日麻立直",
	"international":                     "国标麻将",
}

func ruleLabel(view RoomView) string {
	return ruleNameFromID(view.RuleID)
}

func ruleNameFromID(ruleID string) string {
	if name, ok := ruleDisplayNames[ruleID]; ok {
		return name
	}
	return "麻将"
}

func roomLabel(view RoomView) string {
	if view.DisplayName != "" {
		return view.DisplayName
	}
	if view.RoomID != "" {
		return view.RoomID
	}
	return "--"
}

func phaseLabel(phase TablePhase) string {
	switch phase {
	case PhaseWaiting:
		return "等待开局"
	case PhaseExchange:
		return "换三张"
	case PhaseQueMen:
		return "定缺"
	case PhaseDiscard:
		return "出牌"
	case PhaseMyTurnIdle, PhaseMyTurnSelected:
		return "你的回合"
	case PhaseOtherTurn:
		return "等待他家"
	case PhaseClaim, PhaseTsumo:
		return "鸣牌"
	case PhaseSettlement:
		return "结算"
	default:
		return "牌桌"
	}
}

func networkLabel(view RoomView) string {
	if view.Reconnecting {
		return "○ 重连中"
	}
	if !view.Connected {
		return "○ 离线"
	}
	if view.RTTms > 0 {
		return fmt.Sprintf("● %dms", view.RTTms)
	}
	return "● 在线"
}

// ─── 焦点与倒计时 ────────────────────────────────────

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

func actionCountdown(view RoomView, now time.Time) (int, bool) {
	if now.IsZero() {
		return 0, false
	}
	if view.DeadlineUnixMS > 0 {
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

// seatPrepLabel 已移至 scene_table.go 的 drawRoomPrepOverlay 内联实现。

// tileSortKey 给 m/p/s/z 四种花色一个稳定排序权重。
func tileSortKey(name string) string {
	if len(name) < 2 {
		return "z9_" + name
	}
	suit := name[0]
	prefix := "z8"
	switch suit {
	case 'm':
		prefix = "a"
	case 'p':
		prefix = "b"
	case 's':
		prefix = "c"
	case 'z':
		prefix = "d"
	}
	return prefix + string(name[1])
}
