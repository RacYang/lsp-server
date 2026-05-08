package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

// SceneID 标识当前全屏 TUI 的主场景。
type SceneID string

const (
	SceneLobby    SceneID = "lobby"
	SceneRoomPrep SceneID = "room_prep"
	SceneTable    SceneID = "table"
	SceneSettle   SceneID = "settle"
	SceneSettings SceneID = "settings"
	SceneError    SceneID = "error"
)

// Scene 是新版全屏 TUI 的场景契约。场景只负责渲染和键盘事件，
// 网络动作通过 gateway 发出，事实状态由 AppState 回填。
type Scene interface {
	ID() SceneID
	Render(scr tcell.Screen, now time.Time)
	HandleKey(ctx context.Context, ev *tcell.EventKey)
	Tick(ctx context.Context, now time.Time)
}

// SceneRouter 持有全屏 TUI 的共享状态，统一调度大厅、预备、牌桌、结算和网络浮层。
type SceneRouter struct {
	state   *AppState
	lobbyGW LobbyGateway
	tableGW TableGateway
	cfg     *Config

	lobby *LobbyScene

	cursor      *HandCursor
	overlay     OverlayState
	netOverlay  *NetOverlayState
	claimDialog *ClaimDialogState
	theme       TileTheme
	lastTier    LayoutTier

	quit bool
	err  error
}

func NewSceneRouter(state *AppState, lobbyGW LobbyGateway, tableGW TableGateway, cfg *Config) *SceneRouter {
	theme := TileThemeUnicode
	if cfg != nil {
		theme = ParseTileTheme(cfg.TileTheme)
	}
	return &SceneRouter{
		state:      state,
		lobbyGW:    lobbyGW,
		tableGW:    tableGW,
		cfg:        cfg,
		lobby:      NewLobbyScene(state, lobbyGW, cfg),
		cursor:     &HandCursor{},
		netOverlay: NewNetOverlay(0),
		theme:      theme,
		lastTier:   LayoutTierStandard,
	}
}

func (r *SceneRouter) CurrentSceneID() SceneID {
	view := r.state.Snapshot()
	if view.Phase == phaseTable && view.RoomID != "" {
		if view.LastSettlement != nil || view.RoomState == "settling" {
			return SceneSettle
		}
		if !gameStarted(view) {
			return SceneRoomPrep
		}
		return SceneTable
	}
	return SceneLobby
}

func (r *SceneRouter) Render(scr tcell.Screen, now time.Time) {
	switch r.CurrentSceneID() {
	case SceneRoomPrep:
		r.renderRoomPrep(scr, now)
	case SceneTable:
		r.renderTable(scr, now)
	case SceneSettle:
		r.renderSettle(scr, now)
	default:
		r.lobby.Render(scr, now)
	}
	r.renderNetwork(scr, now)
	scr.Show()
}

func (r *SceneRouter) HandleEvent(ctx context.Context, ev tcell.Event, scr tcell.Screen) {
	switch e := ev.(type) {
	case *tcell.EventResize:
		scr.Sync()
	case *tcell.EventKey:
		r.handleKey(ctx, e)
	}
}

func (r *SceneRouter) handleKey(ctx context.Context, ev *tcell.EventKey) {
	switch r.CurrentSceneID() {
	case SceneRoomPrep:
		r.handleRoomPrepKey(ctx, ev)
	case SceneTable:
		result := handleTableKey(ctx, ev, r.state, r.tableGW, r.cursor, &r.overlay, r.netOverlay, &r.theme, r.cfg, r.claimDialog)
		r.applyTableExit(result.exit)
	case SceneSettle:
		r.handleSettleKey(ctx, ev)
	default:
		r.lobby.HandleKey(ctx, ev)
		if r.lobby.ShouldQuit() {
			r.quit = true
		}
	}
}

func (r *SceneRouter) Tick(ctx context.Context, now time.Time) {
	_ = ctx
	_ = now
	if r.CurrentSceneID() == SceneTable && r.claimDialog != nil && !r.claimDialog.Pending && r.claimDialog.Expired(time.Now()) {
		r.claimDialog.Pending = true
		go func() { _ = r.tableGW.Pass(ctx) }()
	}
}

func (r *SceneRouter) Done() (bool, error) {
	return r.quit, r.err
}

func (r *SceneRouter) renderTable(scr tcell.Screen, now time.Time) {
	view := r.state.Snapshot()
	model := DeriveInteractionModel(view)
	r.cursor.SyncMode(view)
	w, h := scr.Size()
	layout, ok := CalcLayout(w, h, r.lastTier)
	if !ok {
		scr.Clear()
		drawText(scr, 0, 0, defaultStyle(), fmt.Sprintf("窗口太小,请放大终端到至少 %dx%d", MinTableWidth, MinTableHeight))
		return
	}
	r.lastTier = layout.Tier
	RenderFrame(scr, FrameInputs{
		View:   view,
		Layout: layout,
		Theme:  r.theme,
		Cursor: r.cursor,
		Now:    now,
	})
	DrawOverlay(scr, layout, view, OverlayContext{RuleID: view.RuleID, Theme: r.theme}, r.overlay)
	if model.Claim != nil && model.Claim.Dialog != nil {
		if r.claimDialog == nil || r.claimDialog.Tile != model.Claim.Dialog.Tile || r.claimDialog.Trigger != model.Claim.Dialog.Trigger {
			r.claimDialog = model.Claim.Dialog
			r.claimDialog.OpenedAt = now
			timeout := defaultClaimTimeout
			if r.cfg != nil && r.cfg.ClaimTimeoutMS > 0 {
				timeout = r.cfg.ClaimTimeoutMS
			}
			r.claimDialog.Deadline = now.Add(time.Duration(timeout) * time.Millisecond)
			if view.DeadlineUnixMS > 0 {
				r.claimDialog.Deadline = time.UnixMilli(view.DeadlineUnixMS)
			}
		}
		DrawClaimDialog(scr, layout, r.claimDialog, now)
	} else {
		r.claimDialog = nil
	}
}

func (r *SceneRouter) renderRoomPrep(scr tcell.Screen, now time.Time) {
	_ = now
	view := r.state.Snapshot()
	scr.Clear()
	w, h := scr.Size()
	drawBandLine(scr, Region{X: 0, Y: 0, Width: w, Height: 1}, "lsp · 房间预备 · "+roomLabel(view), networkLabel(view), false, false)
	centerX, centerY := w/2, h/2
	box := Region{X: centerX - 24, Y: centerY - 8, Width: 48, Height: 16}
	drawSimpleBox(scr, box, "座位")
	drawClippedText(scr, box.X+2, box.Y+2, defaultStyle(), centerVisual(seatPrepLabel(view, 2), box.Width-4), box.Width-4)
	drawClippedText(scr, box.X+2, box.Y+7, defaultStyle(), seatPrepLabel(view, 1), box.Width/2-2)
	drawClippedText(scr, box.X+box.Width/2+2, box.Y+7, defaultStyle(), seatPrepLabel(view, 3), box.Width/2-4)
	drawClippedText(scr, box.X+2, box.Y+12, defaultStyle(), centerVisual(seatPrepLabel(view, 0), box.Width-4), box.Width-4)
	drawClippedText(scr, 2, 2, defaultStyle(), "规则: "+ruleLabel(view), w-4)
	drawClippedText(scr, 2, 3, defaultStyle(), "房间: "+view.RoomID, w-4)
	drawBandLine(scr, Region{X: 0, Y: h - 1, Width: w, Height: 1}, "Enter 准备    b 补 1 个机器人    B 补满    q 返回大厅    ? 帮助", "", false, false)
}

func (r *SceneRouter) handleRoomPrepKey(ctx context.Context, ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		go func() { _ = r.tableGW.Ready(ctx) }()
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'b':
			go func() { _, _ = r.tableGW.AddBot(ctx, 1) }()
		case 'B':
			view := r.state.Snapshot()
			go func() { _, _ = r.tableGW.AddBot(ctx, emptySeatCount(view)) }()
		case 'q', 'Q':
			r.applyTableExit(leaveRoomFireAndForget(ctx, r.state, r.tableGW, TableExitLeaveRoom).exit)
		case '?':
			r.overlay.Toggle(OverlayHelp)
		}
	}
}

func (r *SceneRouter) renderSettle(scr tcell.Screen, now time.Time) {
	view := r.state.Snapshot()
	scr.Clear()
	w, h := scr.Size()
	drawBandLine(scr, Region{X: 0, Y: 0, Width: w, Height: 1}, "lsp · 结算 · "+roomLabel(view), networkLabel(view), false, false)
	box := Region{X: renderMaxInt(0, w/2-32), Y: renderMaxInt(2, h/2-10), Width: 64, Height: 18}
	if box.X+box.Width > w {
		box.Width = w - box.X
	}
	drawSimpleBox(scr, box, "本局结算")
	summary := snapshotSettlementSummary(view)
	y := box.Y + 2
	if summary == nil {
		drawClippedText(scr, box.X+2, y, defaultStyle(), "暂无结算数据", box.Width-4)
	} else {
		drawClippedText(scr, box.X+2, y, defaultStyle().Bold(true), fmt.Sprintf("结果: %s    总番: %d", settlementOutcomeLabel(summary.Outcome), summary.TotalFan), box.Width-4)
		y += 2
		for _, score := range summary.Scores {
			mark := " "
			if score.IsSelf {
				mark = ">"
			}
			drawClippedText(scr, box.X+2, y, defaultStyle(), fmt.Sprintf("%s %-12s %+d", mark, score.Nickname, score.Delta), box.Width-4)
			y++
		}
		if len(summary.Fans) > 0 {
			y++
			names := make([]string, 0, len(summary.Fans))
			for _, fan := range summary.Fans {
				names = append(names, fan.Name)
			}
			drawClippedText(scr, box.X+2, y, defaultStyle(), "番种: "+strings.Join(names, "、"), box.Width-4)
		}
	}
	_ = now
	drawBandLine(scr, Region{X: 0, Y: h - 1, Width: w, Height: 1}, "r 再开一桌    l 离桌    Enter 停留", "", false, false)
}

func (r *SceneRouter) handleSettleKey(ctx context.Context, ev *tcell.EventKey) {
	if ev.Key() != tcell.KeyRune {
		return
	}
	switch ev.Rune() {
	case 'r', 'R':
		go func() {
			_ = restartAfterSettlement(ctx, r.state, r.lobbyGW)
		}()
	case 'l', 'L', 'q', 'Q':
		r.applyTableExit(leaveRoomFireAndForget(ctx, r.state, r.tableGW, TableExitGameOver).exit)
	}
}

func (r *SceneRouter) renderNetwork(scr tcell.Screen, now time.Time) {
	view := r.state.Snapshot()
	if r.netOverlay == nil {
		return
	}
	r.netOverlay.Update(view.Connected, now)
	w, _ := scr.Size()
	if !view.Connected || view.Reconnecting {
		drawClippedText(scr, renderMaxInt(0, w-24), 0, defaultStyle().Reverse(true), networkLabel(view), 24)
	}
}

func (r *SceneRouter) applyTableExit(exit *TableExit) {
	if exit == nil {
		return
	}
	if exit.Err != nil && !errors.Is(exit.Err, context.Canceled) {
		r.err = exit.Err
	}
	if exit.Reason == TableExitContextDone {
		r.quit = true
	}
}

func drawSimpleBox(scr tcell.Screen, region Region, title string) {
	if region.Width < 4 || region.Height < 3 {
		return
	}
	drawText(scr, region.X, region.Y, defaultStyle(), "┌"+strings.Repeat("─", region.Width-2)+"┐")
	for y := region.Y + 1; y < region.Y+region.Height-1; y++ {
		drawText(scr, region.X, y, defaultStyle(), "│")
		drawText(scr, region.X+region.Width-1, y, defaultStyle(), "│")
	}
	drawText(scr, region.X, region.Y+region.Height-1, defaultStyle(), "└"+strings.Repeat("─", region.Width-2)+"┘")
	if title != "" && region.Width > 6 {
		drawClippedText(scr, region.X+2, region.Y, defaultStyle().Bold(true), " "+title+" ", region.Width-4)
	}
}

func seatPrepLabel(view RoomView, seat int) string {
	if seat < 0 || seat >= len(view.Players) {
		return "□ 空座"
	}
	p := view.Players[seat]
	name := p.Nickname
	if name == "" {
		name = p.UserID
	}
	if name == "" {
		return fmt.Sprintf("□ %s 空座", windLabel(seat))
	}
	mark := "●"
	switch {
	case p.IsBot:
		mark = "▣"
	case p.AutoPlay:
		mark = "◐"
	case !p.Online:
		mark = "○"
	}
	return fmt.Sprintf("%s %s %s", mark, windLabel(seat), name)
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

func settlementOutcomeLabel(out SettlementOutcome) string {
	switch out {
	case SettlementOutcomeWin:
		return "胜"
	case SettlementOutcomeLose:
		return "负"
	default:
		return "荒局"
	}
}
