package main

import (
	"context"
	"errors"
	"time"

	"github.com/gdamore/tcell/v2"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// TableGateway 抽象牌桌阶段对网络层的动作能力。
//
// 与 LobbyGateway 一样，所有方法都是同步「发请求 → 等响应或 ack」。
// 牌桌主循环依赖这个接口在不同按键路径上发出动作，方便测试用 fake 注入。
type TableGateway interface {
	Ready(ctx context.Context) error
	Discard(ctx context.Context, tile string) error
	OpeningAction(ctx context.Context, action PlayerAction, tiles []string, direction, suit int32) error
	Pong(ctx context.Context) error
	Chi(ctx context.Context, tiles []string) error
	Gang(ctx context.Context, tile string) error
	Hu(ctx context.Context) error
	Pass(ctx context.Context) error
	LeaveRoom(ctx context.Context) error
	AddBot(ctx context.Context, count int32) ([]*clientv1.SeatInfo, error)
}

// TableExitReason 描述牌桌主循环退出的原因。
type TableExitReason int

const (
	// TableExitGameOver 服务端推送结算且玩家选择离桌。
	TableExitGameOver TableExitReason = iota
	// TableExitRestart 服务端推送结算且玩家选择再来一局。
	TableExitRestart
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

// tableEventResult 是牌桌按键处理的内部返回值；exit != nil 时主循环退出。
type tableEventResult struct {
	exit *TableExit
}

func handleTableKey(ctx context.Context, ev *tcell.EventKey, state *AppState, gateway TableGateway, cursor *HandCursor, overlay *OverlayState, netOverlay *NetOverlayState, cfg *Config, claimDialog *ClaimDialogState) tableEventResult {
	if overlay.IsOpen() {
		return handleOverlayKey(ctx, ev, state, gateway, overlay, cfg)
	}
	view := state.Snapshot()
	cursor.SyncMode(view)
	model := DeriveInteractionModel(view)
	ux := DeriveTableUXModel(view, cursor, time.Now())
	local := TableLocalUI{}
	if cursor != nil {
		local.Cursor = *cursor
	}
	if claimDialog != nil {
		local.ActionSelected = claimDialog.SelectedIndex
		local.ActionPending = claimDialog.Pending
		local.ActionOpenedAt = claimDialog.OpenedAt
	}
	frontend := BuildTableFrontendModel(view, local, time.Now())
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
		case '?':
			overlay.Toggle(OverlayHelp)
		case ' ':
			// [E1.2] 换三张：选第二张异花色时 UI 必须立刻拒绝标记，不下发到服务端。
			// 仅在 Multi3 模式 + 当前光标落在合法手牌位置 + 已存在标记时校验花色。
			if cursor.Mode == CursorModeMulti3 && cursor.Index >= 0 && cursor.Index < len(hand) && len(cursor.Marked) > 0 && !cursor.IsMarked(cursor.Index) {
				if !sameExchangeSuit(hand, cursor.Marked, cursor.Index) {
					noticeInputRejected(state, ux, "换三张必须同一花色")
					return tableEventResult{}
				}
			}
			if !cursor.ToggleMark() {
				noticeInputRejected(state, ux, "当前不能标记手牌")
			}
		case 'q', 'Q':
			return tableEventResult{}
		case 'b':
			return submitAddBot(ctx, state, gateway, 1)
		case 'B':
			return submitAddBot(ctx, state, gateway, emptySeatCount(view))
		case 'r', 'R':
			if model.Phase == PhaseSettlement {
				return tableEventResult{exit: &TableExit{Reason: TableExitRestart}}
			}
		case 'l', 'L':
			if model.Phase == PhaseSettlement {
				return leaveRoomFireAndForget(ctx, state, gateway, TableExitGameOver)
			}
		// [Q1.1] 仅 m/p/s 三键提交定缺；其它键（含 1/2/3）按"其它键忽略且无副作用"
		// 收口，避免误触发"当前不能定缺"的提示成为副作用。
		case 'm', 'M':
			if !containsAction(frontend.AllowedActions, ActionQueMen) {
				return tableEventResult{}
			}
			cursor.SetIndex(0, 3)
			return submitQueMen(ctx, gateway, model, 0)
		case 'p', 'P':
			// 抢答弹窗下 p 优先解释为碰，避免与定缺冲突；[G2] 对局动作级键的层级。
			if claimDialog != nil && model.Phase == PhaseClaim {
				if !containsAction(frontend.AllowedActions, ActionPong) {
					noticeInputRejected(state, ux, "当前不能碰")
					return tableEventResult{}
				}
				return submitClaimAction(ctx, gateway, claimDialog, ClaimActionPong)
			}
			if !containsAction(frontend.AllowedActions, ActionQueMen) {
				return tableEventResult{}
			}
			cursor.SetIndex(1, 3)
			return submitQueMen(ctx, gateway, model, 1)
		case 'c', 'C':
			if claimDialog == nil || frontend.ActionWindow == nil || !containsAction(frontend.AllowedActions, ActionChi) {
				noticeInputRejected(state, ux, "当前不能吃")
				return tableEventResult{}
			}
			return submitClaimAction(ctx, gateway, claimDialog, ClaimActionChow)
		case 's', 'S':
			if !containsAction(frontend.AllowedActions, ActionQueMen) {
				return tableEventResult{}
			}
			cursor.SetIndex(2, 3)
			return submitQueMen(ctx, gateway, model, 2)
		case 'h', 'H':
			if claimDialog == nil || frontend.ActionWindow == nil || !containsAction(frontend.AllowedActions, ActionHu) {
				noticeInputRejected(state, ux, "当前不能胡")
				return tableEventResult{}
			}
			return submitClaimAction(ctx, gateway, claimDialog, ClaimActionHu)
		case 'g', 'G':
			if claimDialog != nil {
				if !containsAction(frontend.AllowedActions, ActionGang) {
					noticeInputRejected(state, ux, "当前不能杠")
					return tableEventResult{}
				}
				return submitClaimAction(ctx, gateway, claimDialog, ClaimActionGang)
			}
			if containsAction(frontend.AllowedActions, ActionGang) && cursor.Mode == CursorModeSingle {
				return submitSelfGang(ctx, state, cursor, hand, gateway, view)
			}
			noticeInputRejected(state, ux, "当前不能杠")
			return tableEventResult{}
		case 'n', 'N':
			if claimDialog == nil || frontend.ActionWindow == nil || !containsAction(frontend.AllowedActions, ActionPass) {
				noticeInputRejected(state, ux, "当前不能过")
				return tableEventResult{}
			}
			return submitClaimAction(ctx, gateway, claimDialog, ClaimActionPass)
		}
	case tcell.KeyLeft:
		if claimDialog != nil && (model.Phase == PhaseClaim || model.Phase == PhaseTsumo) {
			claimDialog.Move(-1)
			return tableEventResult{}
		}
		cursor.Move(-1, cursorMoveSpan(cursor, len(hand)))
	case tcell.KeyRight:
		if claimDialog != nil && (model.Phase == PhaseClaim || model.Phase == PhaseTsumo) {
			claimDialog.Move(1)
			return tableEventResult{}
		}
		cursor.Move(1, cursorMoveSpan(cursor, len(hand)))
	// KeyEnter (KeyCR=\r) 与 KeyCtrlJ (\n) 都视为提交,
	// 兼容部分终端将回车映射为换行符的情况（如 stty icrnl 关闭、tmux 配置等）。
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		if netOverlay != nil && netOverlay.Status == NetStatusOffline {
			_ = gateway.LeaveRoom(ctx)
			return tableEventResult{exit: &TableExit{Reason: TableExitLeaveRoom}}
		}
		if model.Phase == PhaseSettlement {
			return tableEventResult{}
		}
		if model.Phase == PhaseWaiting && emptySeatCount(view) == 0 {
			return submitReady(ctx, state, gateway, view)
		}
		if claimDialog != nil && frontend.ActionWindow != nil && (model.Phase == PhaseClaim || model.Phase == PhaseTsumo) {
			return submitClaimAction(ctx, gateway, claimDialog, claimDialog.Selected())
		}
		if cursor.Mode == CursorModeMulti3 && !cursor.CanSubmit() {
			if !cursor.ToggleMark() {
				noticeInputRejected(state, ux, "当前不能标记手牌")
			}
			return tableEventResult{}
		}
		return submitCursorAction(ctx, state, cursor, hand, gateway, view)
	}
	return tableEventResult{}
}

func submitReady(ctx context.Context, state *AppState, gateway TableGateway, view RoomView) tableEventResult {
	if view.SeatIndex < 0 || view.SeatIndex > 3 {
		return tableEventResult{}
	}
	go func() {
		if err := gateway.Ready(ctx); err != nil {
			noticeAsyncFailure(state, "准备开局失败", err)
			return
		}
		if state == nil {
			return
		}
		state.Mutate(func(v *RoomView) {
			if v.SeatIndex >= 0 && v.SeatIndex < 4 {
				v.Players[v.SeatIndex].Ready = true
				v.Players[v.SeatIndex].Status = "ready"
			}
		})
	}()
	return tableEventResult{}
}

func submitAddBot(ctx context.Context, state *AppState, gateway TableGateway, count int32) tableEventResult {
	if count <= 0 {
		return tableEventResult{}
	}
	go func() {
		added, err := gateway.AddBot(ctx, count)
		if err != nil {
			noticeAsyncFailure(state, "添加机器人失败", err)
			return
		}
		if len(added) == 0 {
			noticeAsyncFailure(state, "添加机器人失败", errors.New("没有可补的空座"))
			return
		}
		if state == nil {
			return
		}
		state.Mutate(func(v *RoomView) {
			applySeatRoster(v, added)
		})
	}()
	return tableEventResult{}
}

func handleOverlayKey(ctx context.Context, ev *tcell.EventKey, state *AppState, gateway TableGateway, overlay *OverlayState, cfg *Config) tableEventResult {
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
		case '?':
			if overlay.Kind == OverlayHelp {
				overlay.Close()
			} else {
				overlay.Toggle(OverlayHelp)
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
		case OverlayMenuActionResume:
			overlay.Close()
		case OverlayMenuActionLeaveRoom:
			overlay.Close()
			return leaveRoomFireAndForget(ctx, state, gateway, TableExitLeaveRoom)
		}
	}
	return tableEventResult{}
}

// leaveRoomFireAndForget 让玩家立即回到大厅。
//
// 设计要点：
//   - 用户主动按 q / Esc→返回大厅 永远会得到 exit；之前的"本地 RoomID 为空就吃掉退出"分支
//     会让被服务端事件提前清空房间状态的玩家卡死在牌桌界面，已删除。
//   - 仅当本地确实持有过 RoomID 时才向服务端补一发 LeaveRoom；否则会反复触发
//     "尚未进入房间" 噪声日志（见 internal/handler/ws_handlers_room.go 的 INVALID_STATE 分支）。
func leaveRoomFireAndForget(ctx context.Context, state *AppState, gateway TableGateway, reason TableExitReason) tableEventResult {
	roomID := ""
	if state != nil {
		roomID = state.LeaveRoomLocally("已返回大厅，正在通知服务端离房")
	}
	if roomID != "" {
		go retryLeaveRoom(ctx, gateway, 6, 5*time.Second)
	}
	return tableEventResult{exit: &TableExit{Reason: reason}}
}

func retryLeaveRoom(ctx context.Context, gateway TableGateway, attempts int, interval time.Duration) {
	if attempts <= 0 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		if err := gateway.LeaveRoom(ctx); err == nil {
			return
		}
		if i == attempts-1 {
			return
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// submitQueMen 把 m/p/s 三个键路由成"定缺"动作。
//
// 川麻血战的定缺与换三张同样是 4 家并发（不是轮到谁才能定缺），所以这里只校验
// 本地 SeatIndex 合法，不再要求 model.SelfSeat == model.ActingSeat，避免出现
// "我能看到提示但按 m/p/s 没反应" 的死锁。
func submitQueMen(ctx context.Context, gateway TableGateway, model InteractionModel, suit int32) tableEventResult {
	if model.Phase != PhaseOpening || !containsAction(model.Allowed, ActionQueMen) || model.SelfSeat < 0 || model.SelfSeat > 3 {
		return tableEventResult{}
	}
	go func() {
		_ = gateway.OpeningAction(ctx, ActionQueMen, nil, 0, suit)
	}()
	return tableEventResult{}
}

func cursorMoveSpan(cursor *HandCursor, handLen int) int {
	if cursor != nil && cursor.Mode == CursorModeQueMen {
		return 3
	}
	return handLen
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
		case ClaimActionChow:
			err = gateway.Chi(ctx, nil)
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

func submitSelfGang(ctx context.Context, state *AppState, cursor *HandCursor, hand []string, gateway TableGateway, view RoomView) tableEventResult {
	if cursor == nil || cursor.Pending || cursor.Index < 0 || cursor.Index >= len(hand) {
		return tableEventResult{}
	}
	tile := hand[cursor.Index]
	if countTile(hand, tile) < 4 {
		noticeInputRejected(state, DeriveTableUXModel(view, cursor, time.Now()), "选中的牌不能杠")
		return tableEventResult{}
	}
	cursor.SubmitAt(view.LastStep)
	go func() {
		if err := gateway.Gang(ctx, tile); err != nil {
			cursor.RollbackPending()
			noticeAsyncFailure(state, "杠失败", err)
		}
	}()
	return tableEventResult{}
}

func countTile(hand []string, tile string) int {
	n := 0
	for _, current := range hand {
		if current == tile {
			n++
		}
	}
	return n
}

// submitCursorAction 处理 Enter 提交：单选模式直接 Discard，多选模式打 ExchangeThree。
//
// [D2.1] 若当前不满足出牌许可（cursor.Mode == None 或 acting_seats 不含本家），
// Enter 必须静默无效，不弹任何 UXTransient；只有「光标已落在合法位置但尚未选完」
// 这种"可推进 + 用户欠操作"语义才走 noticeInputRejected。
func submitCursorAction(ctx context.Context, state *AppState, cursor *HandCursor, hand []string, gateway TableGateway, view RoomView) tableEventResult {
	if cursor == nil || cursor.Mode == CursorModeNone {
		return tableEventResult{}
	}
	local := TableLocalUI{Cursor: *cursor}
	frontend := BuildTableFrontendModel(view, local, time.Now())
	if !cursor.CanSubmit() {
		ux := DeriveTableUXModel(view, cursor, time.Now())
		noticeInputRejected(state, ux, cursorSubmitDisabledReason(cursor))
		return tableEventResult{}
	}
	switch cursor.Mode {
	case CursorModeSingle:
		if !containsAction(frontend.AllowedActions, ActionDiscard) {
			return tableEventResult{}
		}
		idx := cursor.Index
		if idx < 0 || idx >= len(hand) {
			return tableEventResult{}
		}
		tile := hand[idx]
		cursor.SubmitAt(view.LastStep)
		go func() {
			if err := gateway.Discard(ctx, tile); err != nil {
				cursor.RollbackPending()
				noticeAsyncFailure(state, "出牌失败", err)
			}
		}()
	case CursorModeMulti3:
		if !containsAction(frontend.AllowedActions, ActionExchangeThree) {
			return tableEventResult{}
		}
		if len(cursor.Marked) != 3 {
			return tableEventResult{}
		}
		tiles := make([]string, 0, 3)
		for _, idx := range cursor.Marked {
			if idx >= 0 && idx < len(hand) {
				tiles = append(tiles, hand[idx])
			}
		}
		if len(tiles) != 3 {
			return tableEventResult{}
		}
		recordPendingExchange(state, view.SeatIndex, tiles)
		cursor.SubmitAt(view.LastStep)
		go func() {
			if err := gateway.OpeningAction(ctx, ActionExchangeThree, tiles, 0, 0); err != nil {
				cursor.RollbackPending()
				clearPendingExchange(state, view.SeatIndex)
				noticeAsyncFailure(state, "换三张失败", err)
			}
		}()
	case CursorModeQueMen:
		if !containsAction(frontend.AllowedActions, ActionQueMen) || cursor.Index < 0 || cursor.Index > 2 {
			return tableEventResult{}
		}
		suit := int32(cursor.Index)
		cursor.SubmitAt(view.LastStep)
		go func() {
			if err := gateway.OpeningAction(ctx, ActionQueMen, nil, 0, suit); err != nil {
				cursor.RollbackPending()
				noticeAsyncFailure(state, "定缺失败", err)
			}
		}()
	}
	return tableEventResult{}
}

func noticeInputRejected(state *AppState, ux TableUXModel, fallback string) {
	if state == nil {
		return
	}
	msg := ux.DisabledReason
	if msg == "" {
		msg = fallback
	}
	state.SetNotice(msg, 2*time.Second)
}

func noticeAsyncFailure(state *AppState, label string, err error) {
	if state == nil || err == nil {
		return
	}
	state.SetNotice(label+": "+err.Error(), 2*time.Second)
	state.AddLog(label + ": " + err.Error())
}

// sameExchangeSuit 校验换三张候选索引与已 Mark 的索引是否同一花色。
//
// 协议层手牌以 `m1/p3/s9` 形式编码，suit 即首字符。任何花色不在 m/p/s
// 三种之内（如字牌 z*）均直接判失败，避免规则边界外的误标。
func sameExchangeSuit(hand []string, marked []int, candidate int) bool {
	suit := func(idx int) byte {
		if idx < 0 || idx >= len(hand) || len(hand[idx]) == 0 {
			return 0
		}
		return hand[idx][0]
	}
	cand := suit(candidate)
	if cand != 'm' && cand != 'p' && cand != 's' {
		return false
	}
	for _, idx := range marked {
		if suit(idx) != cand {
			return false
		}
	}
	return true
}

func cursorSubmitDisabledReason(cursor *HandCursor) string {
	if cursor == nil {
		return "当前不能提交"
	}
	if cursor.Pending {
		return "正在等待服务端确认"
	}
	switch cursor.Mode {
	case CursorModeSingle:
		return "请先用 ←→ 选择要出的牌"
	case CursorModeMulti3:
		return "请先用回车标记 3 张牌"
	default:
		return "当前阶段不能操作手牌"
	}
}

func recordPendingExchange(state *AppState, seat int32, tiles []string) {
	if state == nil || seat < 0 || seat > 3 || len(tiles) == 0 {
		return
	}
	state.Mutate(func(v *RoomView) {
		v.PendingExchangeAway = append([]string(nil), tiles...)
	})
}

func clearPendingExchange(state *AppState, seat int32) {
	if state == nil || seat < 0 || seat > 3 {
		return
	}
	state.Mutate(func(v *RoomView) {
		v.PendingExchangeAway = nil
	})
}
