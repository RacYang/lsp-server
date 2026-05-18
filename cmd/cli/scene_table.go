package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"

	"racoo.cn/lsp/cmd/cli/render"
)

// TableScene 是统一的牌桌场景——预备、打牌、结算三个阶段共享四向对称牌桌。
type TableScene struct {
	state            *AppState
	lobbyGW          LobbyGateway
	tableGW          TableGateway
	cfg              *Config
	cursor           *HandCursor
	overlay          OverlayState
	claimDialog      *ClaimDialogState
	settlementDialog *SettlementDialogState
	netOverlay       *NetOverlayState
	lastPhase        tablePhase
	quit             bool
	err              error
}

func NewTableScene(state *AppState, lobbyGW LobbyGateway, tableGW TableGateway, cfg *Config) *TableScene {
	return &TableScene{
		state:      state,
		lobbyGW:    lobbyGW,
		tableGW:    tableGW,
		cfg:        cfg,
		cursor:     &HandCursor{},
		netOverlay: NewNetOverlay(0),
	}
}

func (ts *TableScene) ID() SceneID      { return SceneTable }
func (ts *TableScene) ShouldQuit() bool { return ts.quit }
func (ts *TableScene) Err() error       { return ts.err }

type tablePhase int

const (
	tablePhaseRoomPrep tablePhase = iota
	tablePhasePlaying
	tablePhaseSettlement
)

func (ts *TableScene) phase() tablePhase {
	view := ts.state.Snapshot()
	if view.LastSettlement != nil {
		return tablePhaseSettlement
	}
	if view.RoomState == "playing" || view.RoomState == "settling" {
		return tablePhasePlaying
	}
	return tablePhaseRoomPrep
}

// ─── 渲染 ────────────────────────────────────────────

func (ts *TableScene) Render(scr tcell.Screen, now time.Time) {
	w, h := scr.Size()
	scr.Clear()
	page := render.CalcPage(w, h)
	view := ts.state.Snapshot()
	ph := ts.phase()

	ts.netOverlay.Update(view.Connected, now)

	render.DrawBandLine(scr, page.TitleBar, "lsp · 牌桌 · "+view.Nickname, networkLabel(view), false, false)

	layout, ok := render.CalcTable(w, h)
	if !ok {
		render.DrawBandLine(scr, page.KeyBar, "窗口太小，请调大终端", "", false, false)
		return
	}

	ux := DeriveTableUXModel(view, ts.cursor, now)
	data := ts.buildTableData(view, ux, now, ph)
	render.DrawTableFrame(scr, layout, data)

	switch ph {
	case tablePhaseRoomPrep:
		ts.drawRoomPrepOverlay(scr, layout, view)
	case tablePhaseSettlement:
		ts.drawSettlementOverlay(scr, layout, view, now)
	default:
		ts.drawPlayingOverlays(scr, layout, view, ux, now)
	}

	ts.drawNetworkOverlay(scr, layout.Frame)

	if view.UXNotice != "" && !view.UXNoticeUntil.IsZero() && now.Before(view.UXNoticeUntil) {
		render.DrawToast(scr, page.Toast, view.UXNotice)
	}

	keyHint := ux.KeyHint
	if ph == tablePhaseRoomPrep {
		if emptySeatCount(view) > 0 {
			keyHint = "等人入座：b 补一个机器人　B 补满　? 帮助"
		} else {
			keyHint = "准备开局：Enter 确认　? 帮助"
		}
	}
	render.DrawBandLine(scr, page.KeyBar, keyHint, "", false, false)
}

// buildTableData 把 RoomView 翻译为渲染层 TableData。
// 按相对座位重排：[0]=南(自己), [1]=西(左), [2]=北(上), [3]=东(右)。
func (ts *TableScene) buildTableData(view RoomView, ux TableUXModel, now time.Time, ph tablePhase) render.TableData {
	data := render.TableData{
		Phase:       "playing",
		RoomLabel:   roomLabel(view),
		RuleLabel:   ruleLabel(view),
		WallRemain:  int(view.WallRemaining),
		RoundHand:   fmt.Sprintf("第%d局 第%d手", view.RoundIndex+1, view.HandIndex+1),
		PhasePrompt: ux.PrimaryPrompt,
	}
	if ux.HasCountdown {
		data.Countdown = ux.CountdownSeconds
	}
	if ph == tablePhaseRoomPrep {
		data.Phase = "room_prep"
		data.PhasePrompt = ""
		data.Countdown = 0
	}
	if ph == tablePhaseSettlement {
		data.Phase = "settlement"
		data.PhasePrompt = "本局结束"
		data.Countdown = 0
	}

	// 相对→绝对座位映射
	relToAbs := [4]int32{
		view.SeatIndex,
		(view.SeatIndex + 1) % 4,
		(view.SeatIndex + 2) % 4,
		(view.SeatIndex + 3) % 4,
	}
	for rel := 0; rel < 4; rel++ {
		seat := relToAbs[rel]
		p := view.Players[seat]
		name := playerDisplayName(view, seat)
		if len(name) >= 2 && name[:2] == "★ " {
			name = name[2:]
		}
		data.Players[rel] = render.PlayerInfo{
			Name:      name,
			SeatLabel: ux.Seats[seat].RelativeLabel,
			Status:    seatStatusMark(p),
			IsFocus:   ux.Seats[seat].IsFocus,
			Que:       queLabel(view.QueBySeat[seat]),
			Melds:     formatMeldGlyphs(p.Melds),
			Score:     fmt.Sprintf("%+d", p.TotalScore),
		}
	}

	// 日志（最近 8 条）
	for i := len(view.Log) - 1; i >= 0 && len(data.Events) < 8; i-- {
		entry := view.Log[i]
		data.Events = append(data.Events, entry.At.Format("15:04:05")+" "+entry.Text)
	}

	// 手牌与弃牌（按相对座位重排）
	if ph == tablePhasePlaying || ph == tablePhaseSettlement {
		absHands := [4][]render.TileFace{}
		absDiscards := [4][]render.TileFace{}
		for abs := int32(0); abs < 4; abs++ {
			p := view.Players[abs]
			for _, t := range p.Hand {
				absHands[abs] = append(absHands[abs], render.TileFace{
					Glyph: render.TileGlyph(t),
					Rank:  render.TileRank(t),
					Suit:  render.TileSuit(t),
				})
			}
			for _, t := range p.Discards {
				absDiscards[abs] = append(absDiscards[abs], render.TileFace{
					Glyph: render.TileGlyph(t),
					Rank:  render.TileRank(t),
					Suit:  render.TileSuit(t),
				})
			}
		}
		for rel := 0; rel < 4; rel++ {
			abs := relToAbs[rel]
			data.Hands[rel] = absHands[abs]
			data.Discards[rel] = absDiscards[abs]
		}
	}

	data.Cursor = render.CursorState{
		Mode:    cursorModeString(ts.cursor.Mode),
		Index:   ts.cursor.Index,
		Marked:  append([]int(nil), ts.cursor.Marked...),
		Pending: ts.cursor.Pending,
	}

	return data
}

func cursorModeString(m CursorMode) string {
	switch m {
	case CursorModeSingle:
		return "single"
	case CursorModeMulti3:
		return "multi3"
	case CursorModeQueMen:
		return "quemen"
	default:
		return "none"
	}
}

// ─── 覆盖层 ──────────────────────────────────────────

func (ts *TableScene) drawRoomPrepOverlay(scr tcell.Screen, layout render.TableLayout, view RoomView) {
	status := "等待入座"
	if emptySeatCount(view) == 0 {
		status = "人已坐齐，按 Enter 准备"
	}
	render.DrawClippedText(scr, layout.Center.X, layout.Center.Y, render.DefaultStyle(),
		render.CenterVisual(status, layout.Center.Width), layout.Center.Width)
	if view.RoomID != "" && view.Private {
		codeLine := "房间码：" + view.RoomID
		render.DrawClippedText(scr, layout.Center.X, layout.Center.Y+2,
			render.DefaultStyle(), render.CenterVisual(codeLine, layout.Center.Width), layout.Center.Width)
	}
}

func (ts *TableScene) drawSettlementOverlay(scr tcell.Screen, layout render.TableLayout, view RoomView, now time.Time) {
	if view.LastSettlement == nil {
		return
	}
	if ts.settlementDialog == nil {
		summary := snapshotSettlementSummary(view)
		if summary == nil {
			return
		}
		ts.settlementDialog = NewSettlementDialog(*summary, now, 120*time.Millisecond)
	}
	dlg := ts.settlementDialog
	visible := dlg.VisibleLines(now)
	if visible <= 0 {
		return
	}
	innerW := layout.Frame.Width - 4
	if innerW > 48 {
		innerW = 48
	}
	all := dlg.allLines(innerW)
	if visible > len(all) {
		visible = len(all)
	}
	render.DrawDialog(scr, layout.Frame, "本局结算", all[:visible], render.BorderDouble)
}

func (ts *TableScene) drawPlayingOverlays(scr tcell.Screen, layout render.TableLayout, view RoomView, ux TableUXModel, now time.Time) {
	if ts.overlay.IsOpen() {
		switch ts.overlay.Kind {
		case OverlayRoomInfo:
			render.DrawPanel(scr, layout.Frame.Width, layout.Frame.Height, "房间信息", overlayRoomInfoLines(view))
		case OverlayPlayers:
			render.DrawPanel(scr, layout.Frame.Width, layout.Frame.Height, "玩家", overlayPlayersLines(view))
		case OverlayMenu:
			ts.drawMenuOverlay(scr, layout)
		case OverlayHelp:
			render.DrawPanel(scr, layout.Frame.Width, layout.Frame.Height, "帮助", overlayHelpLines(view))
		}
		return
	}

	if ts.claimDialog != nil && !ts.claimDialog.Pending {
		if ts.claimDialog.Expired(now) {
			ts.claimDialog = nil
			return
		}
		w := layout.Frame.Width - 4
		if w > 48 {
			w = 48
		}
		lines := claimDialogLines(ts.claimDialog, now, w)
		render.DrawDialog(scr, layout.Frame, "", lines, render.BorderSingle)
	}
}

func (ts *TableScene) drawMenuOverlay(scr tcell.Screen, layout render.TableLayout) {
	items := overlayMenuItems()
	lines := make([]string, 0, len(items))
	for i, item := range items {
		prefix := "  "
		if i == ts.overlay.SelectedIndex {
			prefix = "▶ "
		}
		lines = append(lines, prefix+item.Label)
	}
	render.DrawPanel(scr, layout.Frame.Width, layout.Frame.Height, "菜单", lines)
}

func (ts *TableScene) drawNetworkOverlay(scr tcell.Screen, frame render.Region) {
	if ts.netOverlay == nil || !ts.netOverlay.Visible() {
		return
	}
	msg := ts.netOverlay.Title()
	render.DrawToast(scr, render.Region{
		X: frame.X, Y: frame.Y, Width: frame.Width, Height: 1,
	}, msg)
}

// ─── 按键处理 ────────────────────────────────────────

func (ts *TableScene) HandleKey(ctx context.Context, ev *tcell.EventKey) {
	result := handleTableKey(ctx, ev, ts.state, ts.tableGW, ts.cursor, &ts.overlay, ts.netOverlay, ts.cfg, ts.claimDialog)
	if result.exit != nil {
		switch result.exit.Reason {
		case TableExitLeaveRoom:
			ts.overlay.Close()
		case TableExitGameOver:
			ts.quit = true
		case TableExitRestart:
			ts.quit = true
		case TableExitContextDone:
			ts.quit = true
			ts.err = result.exit.Err
		}
	}
}

// ─── 定时 ────────────────────────────────────────────

func (ts *TableScene) Tick(ctx context.Context, now time.Time) {
	view := ts.state.Snapshot()
	ts.cursor.SyncMode(view)

	ph := ts.phase()
	if ph != ts.lastPhase {
		ts.overlay.Close()
		ts.lastPhase = ph
	}

	// 鸣牌超时自动过
	if ts.claimDialog != nil && !ts.claimDialog.Pending && ts.claimDialog.Expired(now) {
		go func() { _ = ts.tableGW.Pass(ctx) }()
		ts.claimDialog = nil
	}

	ts.netOverlay.Update(view.Connected, now)

	// 非覆盖层状态下同步 claim 弹窗
	if !ts.overlay.IsOpen() && ts.claimDialog == nil {
		model := DeriveInteractionModel(view)
		if model.Claim != nil && model.Claim.Dialog != nil {
			ts.claimDialog = model.Claim.Dialog
			ts.claimDialog.OpenedAt = now
			if ts.claimDialog.Deadline.IsZero() {
				ts.claimDialog.Deadline = now.Add(14000 * time.Millisecond)
			}
		}
	}
	if view.LastSettlement == nil {
		ts.settlementDialog = nil
	}
}
