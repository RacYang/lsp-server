package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
)

// TableGateway 抽象牌桌阶段对网络层的动作能力。
//
// 与 LobbyGateway 一样，所有方法都是同步「发请求 → 等响应或 ack」。
// 牌桌主循环依赖这个接口在不同按键路径上发出动作，方便测试用 fake 注入。
type TableGateway interface {
	Ready(ctx context.Context) error
	Discard(ctx context.Context, tile string) error
	ExchangeThree(ctx context.Context, tiles []string) error
	QueMen(ctx context.Context, suit int32) error
	Pong(ctx context.Context) error
	Gang(ctx context.Context, tile string) error
	Hu(ctx context.Context) error
	Pass(ctx context.Context) error
	LeaveRoom(ctx context.Context) error
}

// TableExitReason 描述牌桌主循环退出的原因。
type TableExitReason int

const (
	// TableExitGameOver 服务端推送结算且玩家按 Enter 离桌。
	TableExitGameOver TableExitReason = iota
	// TableExitLeaveRoom 玩家在局内菜单选择"返回大厅"。
	TableExitLeaveRoom
	// TableExitContextDone 主循环外层 ctx 被取消。
	TableExitContextDone
	// TableExitTerminalTooSmall 终端尺寸不足，无法继续。
	TableExitTerminalTooSmall
)

// TableExit 描述退出时的状态，包含可选错误信息让上层 lobby 决定如何反馈。
type TableExit struct {
	Reason TableExitReason
	Err    error
}

// RunTableScreen 是牌桌全屏阶段的主循环。
//
// 进入时切到 alternate screen，退出时通过 TerminalSwitch 恢复 lobby 行式输出；
// state 由 EventBus 在外部持续更新；本函数读取 state 渲染并把按键转发到 gateway。
//
// 当前版本是"最小可行"：覆盖渲染、光标、主题切换、叠加层；具体动作（碰/杠/胡）
// 等到 step 7-10 的浮窗状态机被实际 envelope 触发后再接入。
func RunTableScreen(ctx context.Context, switcher *TerminalSwitch, state *AppState, gateway TableGateway, cfg *Config) TableExit {
	scr, err := switcher.EnterFullscreen()
	if err != nil {
		return TableExit{Reason: TableExitContextDone, Err: fmt.Errorf("打开终端失败: %w", err)}
	}
	defer switcher.LeaveFullscreen()

	w, h := scr.Size()
	if w < MinTableWidth || h < MinTableHeight {
		return TableExit{Reason: TableExitTerminalTooSmall, Err: errors.New("窗口太小,请放大终端到至少 64x20 后重试")}
	}

	overlay := OverlayState{}
	cursor := &HandCursor{}
	netOverlay := NewNetOverlay(0)
	var claimDialog *ClaimDialogState
	var settlementDialog *SettlementDialogState
	theme := ParseTileTheme(cfg.TileTheme)

	go func() {
		_ = gateway.Ready(ctx)
	}()

	eventCh := make(chan tcell.Event, 16)
	tcellCtx, cancelTcell := context.WithCancel(ctx)
	defer cancelTcell()
	go func() {
		for {
			if tcellCtx.Err() != nil {
				return
			}
			ev := scr.PollEvent()
			if ev == nil {
				return
			}
			select {
			case eventCh <- ev:
			case <-tcellCtx.Done():
				return
			}
		}
	}()

	redraw := func() {
		view := state.Snapshot()
		model := DeriveInteractionModel(view)
		cursor.SyncMode(view)
		w, h := scr.Size()
		layout, ok := CalcLayout(w, h)
		if !ok {
			scr.Clear()
			drawText(scr, 0, 0, defaultStyle(), "窗口太小,请放大终端到至少 64x20")
			scr.Show()
			return
		}
		RenderFrame(scr, FrameInputs{
			View:   view,
			Layout: layout,
			Theme:  theme,
			Cursor: cursor,
		})
		netOverlay.Update(view.Connected, time.Now())
		ctxOverlay := OverlayContext{RuleID: view.RuleID, Theme: theme}
		DrawOverlay(scr, layout, view, ctxOverlay, overlay)
		DrawNetOverlay(scr, layout, netOverlay, time.Now())
		now := time.Now()
		if model.Claim != nil && model.Claim.Dialog != nil {
			if claimDialog == nil || claimDialog.Tile != model.Claim.Dialog.Tile || claimDialog.Trigger != model.Claim.Dialog.Trigger {
				claimDialog = model.Claim.Dialog
				claimDialog.OpenedAt = now
				claimDialog.Deadline = now.Add(time.Duration(cfg.ClaimTimeoutMS) * time.Millisecond)
			}
			DrawClaimDialog(scr, layout, claimDialog, now)
		} else {
			claimDialog = nil
		}
		if model.Settlement != nil {
			if settlementDialog == nil || settlementDialog.Summary.RoomID != model.Settlement.RoomID {
				settlementDialog = NewSettlementDialog(*model.Settlement, now, 120*time.Millisecond)
			}
			DrawSettlementDialog(scr, layout, settlementDialog, now)
		} else {
			settlementDialog = nil
		}
		scr.Show()
	}

	redraw()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return TableExit{Reason: TableExitContextDone, Err: ctx.Err()}
		case <-ticker.C:
			if claimDialog != nil && !claimDialog.Pending && claimDialog.Expired(time.Now()) {
				claimDialog.Pending = true
				go func() {
					_ = gateway.Pass(ctx)
				}()
			}
			redraw()
		case ev, ok := <-eventCh:
			if !ok {
				return TableExit{Reason: TableExitContextDone, Err: errors.New("事件流已关闭")}
			}
			result := handleTableEvent(ctx, ev, scr, state, gateway, cursor, &overlay, netOverlay, &theme, cfg, claimDialog)
			if result.exit != nil {
				return *result.exit
			}
			redraw()
		}
	}
}

// tableEventResult 是 handleTableEvent 的内部返回值；exit != nil 时主循环退出。
type tableEventResult struct {
	exit *TableExit
}

// handleTableEvent 把 tcell 事件分派给具体动作；保持纯函数风格便于后续测试。
func handleTableEvent(ctx context.Context, ev tcell.Event, scr tcell.Screen, state *AppState, gateway TableGateway, cursor *HandCursor, overlay *OverlayState, netOverlay *NetOverlayState, theme *TileTheme, cfg *Config, claimDialog *ClaimDialogState) tableEventResult {
	switch e := ev.(type) {
	case *tcell.EventResize:
		scr.Sync()
		return tableEventResult{}
	case *tcell.EventKey:
		return handleTableKey(ctx, e, state, gateway, cursor, overlay, netOverlay, theme, cfg, claimDialog)
	}
	return tableEventResult{}
}

func handleTableKey(ctx context.Context, ev *tcell.EventKey, state *AppState, gateway TableGateway, cursor *HandCursor, overlay *OverlayState, netOverlay *NetOverlayState, theme *TileTheme, cfg *Config, claimDialog *ClaimDialogState) tableEventResult {
	if overlay.IsOpen() {
		return handleOverlayKey(ctx, ev, gateway, overlay, theme, cfg)
	}
	view := state.Snapshot()
	model := DeriveInteractionModel(view)
	hand := []string{}
	if view.SeatIndex >= 0 && view.SeatIndex < 4 {
		hand = view.Players[view.SeatIndex].Hand
	}
	switch ev.Key() {
	case tcell.KeyEscape:
		overlay.Toggle(OverlayMenu)
	case tcell.KeyTAB:
		overlay.Toggle(OverlayPlayers)
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'i', 'I':
			overlay.Toggle(OverlayRoomInfo)
		case ' ':
			cursor.ToggleMark()
		case 'q', 'Q':
			return leaveIfSettlement(ctx, gateway, model)
		case 'm', 'M':
			return submitQueMen(ctx, gateway, model, 0)
		case 'p', 'P':
			if claimDialog != nil && model.Phase == PhaseClaim {
				return submitClaimAction(ctx, gateway, claimDialog, ClaimActionPong)
			}
			return submitQueMen(ctx, gateway, model, 1)
		case 's', 'S':
			return submitQueMen(ctx, gateway, model, 2)
		case 'h', 'H':
			return submitClaimAction(ctx, gateway, claimDialog, ClaimActionHu)
		case 'g', 'G':
			return submitClaimAction(ctx, gateway, claimDialog, ClaimActionGang)
		case 'n', 'N':
			return submitClaimAction(ctx, gateway, claimDialog, ClaimActionPass)
		}
	case tcell.KeyLeft:
		cursor.Move(-1, len(hand))
	case tcell.KeyRight:
		cursor.Move(1, len(hand))
	// KeyEnter (KeyCR=\r) 与 KeyCtrlJ (\n) 都视为提交,
	// 兼容部分终端将回车映射为换行符的情况（如 stty icrnl 关闭、tmux 配置等）。
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		if netOverlay != nil && netOverlay.Status == NetStatusOffline {
			_ = gateway.LeaveRoom(ctx)
			return tableEventResult{exit: &TableExit{Reason: TableExitLeaveRoom}}
		}
		if model.Phase == PhaseSettlement {
			_ = gateway.LeaveRoom(ctx)
			return tableEventResult{exit: &TableExit{Reason: TableExitGameOver}}
		}
		if claimDialog != nil && (model.Phase == PhaseClaim || model.Phase == PhaseTsumo) {
			return submitClaimAction(ctx, gateway, claimDialog, claimDialog.Selected())
		}
		return submitCursorAction(ctx, cursor, hand, gateway, view)
	}
	return tableEventResult{}
}

func handleOverlayKey(ctx context.Context, ev *tcell.EventKey, gateway TableGateway, overlay *OverlayState, theme *TileTheme, cfg *Config) tableEventResult {
	switch ev.Key() {
	case tcell.KeyEscape:
		overlay.Close()
	case tcell.KeyUp:
		overlay.MenuMove(-1)
	case tcell.KeyDown:
		overlay.MenuMove(1)
	case tcell.KeyTAB:
		if overlay.Kind == OverlayPlayers {
			overlay.Close()
		} else {
			overlay.Toggle(OverlayPlayers)
		}
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'i', 'I':
			if overlay.Kind == OverlayRoomInfo {
				overlay.Close()
			}
		}
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		// 非菜单类浮窗（房间信息 / 玩家详情）只承载只读展示,
		// Enter 在这里没有"选择"语义,直接当成"关闭"以避免 Enter 静默无效。
		if overlay.Kind != OverlayMenu {
			overlay.Close()
			return tableEventResult{}
		}
		switch overlay.MenuSelect() {
		case OverlayMenuActionToggleTheme:
			if *theme == TileThemeUnicode {
				*theme = TileThemeASCII
			} else {
				*theme = TileThemeUnicode
			}
			cfg.TileTheme = theme.String()
			overlay.Close()
		case OverlayMenuActionResume:
			overlay.Close()
		case OverlayMenuActionLeaveRoom:
			_ = gateway.LeaveRoom(ctx)
			return tableEventResult{exit: &TableExit{Reason: TableExitLeaveRoom}}
		}
	}
	return tableEventResult{}
}

func leaveIfSettlement(ctx context.Context, gateway TableGateway, model InteractionModel) tableEventResult {
	if model.Phase != PhaseSettlement {
		return tableEventResult{}
	}
	_ = gateway.LeaveRoom(ctx)
	return tableEventResult{exit: &TableExit{Reason: TableExitGameOver}}
}

func submitQueMen(ctx context.Context, gateway TableGateway, model InteractionModel, suit int32) tableEventResult {
	if model.Phase != PhaseQueMen || model.SelfSeat != model.ActingSeat {
		return tableEventResult{}
	}
	go func() {
		_ = gateway.QueMen(ctx, suit)
	}()
	return tableEventResult{}
}

func submitClaimAction(ctx context.Context, gateway TableGateway, dialog *ClaimDialogState, action ClaimAction) tableEventResult {
	if dialog == nil || dialog.Pending {
		return tableEventResult{}
	}
	dialog.Pending = true
	go func() {
		var err error
		switch action {
		case ClaimActionHu:
			err = gateway.Hu(ctx)
		case ClaimActionPong:
			err = gateway.Pong(ctx)
		case ClaimActionGang:
			err = gateway.Gang(ctx, dialog.Tile)
		default:
			err = gateway.Pass(ctx)
		}
		if err != nil {
			dialog.Pending = false
		}
	}()
	return tableEventResult{}
}

// submitCursorAction 处理 Enter 提交：单选模式直接 Discard，多选模式打 ExchangeThree。
func submitCursorAction(ctx context.Context, cursor *HandCursor, hand []string, gateway TableGateway, view RoomView) tableEventResult {
	if !cursor.CanSubmit() {
		return tableEventResult{}
	}
	switch cursor.Mode {
	case CursorModeSingle:
		idx := cursor.Index
		if idx < 0 || idx >= len(hand) {
			return tableEventResult{}
		}
		tile := hand[idx]
		cursor.Submit()
		go func() {
			if err := gateway.Discard(ctx, tile); err != nil {
				cursor.RollbackPending()
			}
		}()
	case CursorModeMulti3:
		if len(cursor.Marked) != 3 {
			return tableEventResult{}
		}
		tiles := make([]string, 0, 3)
		for _, idx := range cursor.Marked {
			if idx >= 0 && idx < len(hand) {
				tiles = append(tiles, hand[idx])
			}
		}
		cursor.Submit()
		go func() {
			if err := gateway.ExchangeThree(ctx, tiles); err != nil {
				cursor.RollbackPending()
			}
		}()
	}
	return tableEventResult{}
}
