package main

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// stubTableGateway 是 TableGateway 的最小占位实现,所有方法直接返回 nil。
//
// table_screen 单测里只关心键路由对 OverlayState / cursor 的影响,
// 不依赖网络往返,因此 gateway 只需实现接口即可。
type stubTableGateway struct{}

func (stubTableGateway) Ready(context.Context) error                          { return nil }
func (stubTableGateway) Discard(context.Context, string) error                { return nil }
func (stubTableGateway) ExchangeThree(context.Context, []string, int32) error { return nil }
func (stubTableGateway) QueMen(context.Context, int32) error                  { return nil }
func (stubTableGateway) Pong(context.Context) error                           { return nil }
func (stubTableGateway) Gang(context.Context, string) error                   { return nil }
func (stubTableGateway) Hu(context.Context) error                             { return nil }
func (stubTableGateway) Pass(context.Context) error                           { return nil }
func (stubTableGateway) LeaveRoom(context.Context) error                      { return nil }
func (stubTableGateway) AddBot(context.Context, int32) ([]*clientv1.SeatInfo, error) {
	return nil, nil
}

type leaveSignalGateway struct {
	stubTableGateway
	calls chan struct{}
}

func (g leaveSignalGateway) LeaveRoom(context.Context) error {
	select {
	case g.calls <- struct{}{}:
	default:
	}
	return nil
}

type exchangeGateway struct {
	stubTableGateway
	tiles chan []string
	err   error
}

func (g exchangeGateway) ExchangeThree(_ context.Context, tiles []string, _ int32) error {
	select {
	case g.tiles <- append([]string(nil), tiles...):
	default:
	}
	return g.err
}

type discardGateway struct {
	stubTableGateway
	tiles chan string
	err   error
}

func (g discardGateway) Discard(_ context.Context, tile string) error {
	select {
	case g.tiles <- tile:
	default:
	}
	return g.err
}

type queMenGateway struct {
	stubTableGateway
	suits chan int32
}

func (g queMenGateway) QueMen(_ context.Context, suit int32) error {
	select {
	case g.suits <- suit:
	default:
	}
	return nil
}

// discardGatewayCapture 单纯记录 Discard 是否被调用，用于 [D2.1] 静默断言。
type discardGatewayCapture struct {
	stubTableGateway
	called bool
}

func (g *discardGatewayCapture) Discard(_ context.Context, _ string) error {
	g.called = true
	return nil
}

// discardGatewayCounter 用原子计数器记录 Discard 调用次数，用于 [D2.2] 防抖断言。
type discardGatewayCounter struct {
	stubTableGateway
	n int32
}

func (g *discardGatewayCounter) Discard(_ context.Context, _ string) error {
	atomic.AddInt32(&g.n, 1)
	return nil
}

func (g *discardGatewayCounter) count() int { return int(atomic.LoadInt32(&g.n)) }

type addBotGateway struct {
	stubTableGateway
	added []*clientv1.SeatInfo
	err   error
}

func (g addBotGateway) AddBot(context.Context, int32) ([]*clientv1.SeatInfo, error) {
	return g.added, g.err
}

// TestHandleOverlayKeyEnterClosesNonMenuOverlay 验证非菜单浮窗（房间信息 / 玩家详情）
// 下按 Enter 会被当成"关闭",避免出现"Enter 在所有情景下都没反应"的疑惑。
func TestHandleOverlayKeyEnterClosesNonMenuOverlay(t *testing.T) {
	cases := []struct {
		name string
		kind OverlayKind
	}{
		{"room_info", OverlayRoomInfo},
		{"players", OverlayPlayers},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			overlay := &OverlayState{Kind: tc.kind}
			theme := TileThemeUnicode
			cfg := &Config{}
			ev := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
			res := handleOverlayKey(context.Background(), ev, NewAppState("racoo"), stubTableGateway{}, overlay, &theme, cfg)
			require.Nil(t, res.exit, "Enter 关闭非菜单浮窗不应触发主循环退出")
			require.Equal(t, OverlayNone, overlay.Kind, "Enter 应把非菜单浮窗关闭")
		})
	}
}

func TestSubmitAddBotAppliesReturnedSeats(t *testing.T) {
	state := NewAppState("racoo")
	submitAddBot(context.Background(), state, addBotGateway{added: []*clientv1.SeatInfo{
		{SeatIndex: 1, UserId: "bot:r1:1", Nickname: "机器人", IsBot: true, Status: "ready"},
	}}, 1)

	require.Eventually(t, func() bool {
		view := state.Snapshot()
		return view.Players[1].IsBot && view.Players[1].Ready
	}, time.Second, 10*time.Millisecond)
}

func TestSubmitAddBotFailureShowsNotice(t *testing.T) {
	state := NewAppState("racoo")
	submitAddBot(context.Background(), state, addBotGateway{err: errors.New("room full")}, 1)

	require.Eventually(t, func() bool {
		return state.Snapshot().UXNotice == "添加机器人失败: room full"
	}, time.Second, 10*time.Millisecond)
}

// TestHandleOverlayKeyEnterStillTriggersMenuAction 兜底校验:菜单浮窗里的 Enter
// 仍然正常触发动作,不会被新增的"非菜单 Enter 关闭"分支误吃掉。
func TestHandleOverlayKeyEnterStillTriggersMenuAction(t *testing.T) {
	overlay := &OverlayState{Kind: OverlayMenu, SelectedIndex: 1} // 继续游戏
	theme := TileThemeUnicode
	cfg := &Config{}
	ev := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
	res := handleOverlayKey(context.Background(), ev, NewAppState("racoo"), stubTableGateway{}, overlay, &theme, cfg)
	require.Nil(t, res.exit)
	require.Equal(t, OverlayNone, overlay.Kind, "继续游戏菜单项应关闭浮窗")
}

// TestHandleOverlayKeyCtrlJBehavesLikeEnter 兜底校验:终端把回车映射为 \n（KeyCtrlJ）
// 时,与 KeyEnter 走完全相同的关闭路径。
func TestHandleOverlayKeyCtrlJBehavesLikeEnter(t *testing.T) {
	overlay := &OverlayState{Kind: OverlayRoomInfo}
	theme := TileThemeUnicode
	cfg := &Config{}
	ev := tcell.NewEventKey(tcell.KeyCtrlJ, 0, tcell.ModCtrl)
	res := handleOverlayKey(context.Background(), ev, NewAppState("racoo"), stubTableGateway{}, overlay, &theme, cfg)
	require.Nil(t, res.exit)
	require.Equal(t, OverlayNone, overlay.Kind)
}

func TestHandleTableKeyQuestionTogglesHelp(t *testing.T) {
	state := NewAppState("racoo")
	cursor := &HandCursor{Index: -1}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}

	ev := tcell.NewEventKey(tcell.KeyRune, '?', tcell.ModNone)
	res := handleTableKey(context.Background(), ev, state, stubTableGateway{}, cursor, overlay, nil, &theme, cfg, nil)
	require.Nil(t, res.exit)
	require.Equal(t, OverlayHelp, overlay.Kind)

	res = handleOverlayKey(context.Background(), ev, state, stubTableGateway{}, overlay, &theme, cfg)
	require.Nil(t, res.exit)
	require.Equal(t, OverlayNone, overlay.Kind)
}

func TestHandleTableEventResizeKeepsTableOpen(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, scr.Init())
	defer scr.Fini()
	scr.SetSize(MinTableWidth, MinTableHeight)
	state := NewAppState("racoo")
	cursor := &HandCursor{Index: -1}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	res := handleTableEvent(context.Background(), tcell.NewEventResize(120, MinTableHeight), scr, state, stubTableGateway{}, cursor, overlay, nil, &theme, cfg, nil)
	require.Nil(t, res.exit)
	layout, ok := CalcLayout(145, 45, LayoutTierStandard)
	require.True(t, ok)
	require.Equal(t, LayoutTierFull, layout.Tier)
}

// TestHandleTableKeyQExitsEvenWhenLocalRoomEmpty 防回归：用户主动按 q / 返回大厅
// 时永远应当返回到大厅，即便本地状态因外部事件（LeaveRoomResp / RoomDestroy /
// RouteRedirect）已经被清空。修复 "离房失败：尚未进入房间" + UI 卡死的根因。
func TestHandleTableKeyQExitsEvenWhenLocalRoomEmpty(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseLobby
		v.RoomID = ""
		v.SeatIndex = -1
	})
	cursor := &HandCursor{}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	gw := leaveSignalGateway{calls: make(chan struct{}, 1)}

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone), state, gw, cursor, overlay, nil, &theme, cfg, nil)
	require.NotNil(t, res.exit, "q 必须永远触发牌桌退出，避免玩家在残留 UI 上卡死")
	require.Equal(t, TableExitLeaveRoom, res.exit.Reason)

	// 本地已经无房间时不应再骚扰服务端，否则会被回 INVALID_STATE，污染玩家可见日志。
	select {
	case <-gw.calls:
		t.Fatal("本地无 RoomID 时不应再向服务端发 LeaveRoom")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHandleTableKeyQLeavesLocallyAndNotifiesServer(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.Players[0].UserID = "u0"
	})
	cursor := &HandCursor{}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	gw := leaveSignalGateway{calls: make(chan struct{}, 1)}

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone), state, gw, cursor, overlay, nil, &theme, cfg, nil)
	require.NotNil(t, res.exit)
	require.Equal(t, TableExitLeaveRoom, res.exit.Reason)
	view := state.Snapshot()
	require.Equal(t, phaseLobby, view.Phase)
	require.Empty(t, view.RoomID)
	require.Equal(t, "r1", view.PendingLeaveRoomID)

	select {
	case <-gw.calls:
	case <-time.After(time.Second):
		t.Fatal("expected async LeaveRoom call")
	}
}

func TestHandleTableKeyEnterDoesNotLeaveOnSettlement(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.LastSettlement = &clientv1.SettlementNotify{RoomId: "r1"}
	})
	cursor := &HandCursor{}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	gw := leaveSignalGateway{calls: make(chan struct{}, 1)}

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), state, gw, cursor, overlay, nil, &theme, cfg, nil)
	require.Nil(t, res.exit, "结算态 Enter 只停留在当前牌桌,不应直接返回主菜单")

	select {
	case <-gw.calls:
		t.Fatal("结算态 Enter 不应触发 LeaveRoom")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHandleTableKeyRRestartsOnSettlement(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.LastSettlement = &clientv1.SettlementNotify{RoomId: "r1"}
	})
	cursor := &HandCursor{}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone), state, stubTableGateway{}, cursor, overlay, nil, &theme, cfg, nil)
	require.NotNil(t, res.exit)
	require.Equal(t, TableExitRestart, res.exit.Reason)
}

func TestSubmitExchangeRecordsPendingWithoutChangingHand(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.WaitingAction = "exchange_three"
		v.Players[0].Hand = []string{"m1", "m9", "p1", "s1"}
		v.Players[0].HandCnt = 4
	})
	cursor := &HandCursor{Mode: CursorModeMulti3, Index: 0, Marked: []int{0, 1, 2}}
	gw := exchangeGateway{tiles: make(chan []string, 1)}
	view := state.Snapshot()

	res := submitCursorAction(context.Background(), state, cursor, view.Players[0].Hand, gw, view)
	require.Nil(t, res.exit)
	require.True(t, cursor.Pending)
	after := state.Snapshot()
	require.Equal(t, []string{"m1", "m9", "p1", "s1"}, after.Players[0].Hand)
	require.Equal(t, []string{"m1", "m9", "p1"}, after.PendingExchangeAway)

	select {
	case got := <-gw.tiles:
		require.Equal(t, []string{"m1", "m9", "p1"}, got)
	case <-time.After(time.Second):
		t.Fatal("expected exchange request")
	}
}

func TestSubmitDiscardFailureShowsNotice(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.ActingSeat = 0
		v.WaitingAction = "discard"
		v.Players[0].Hand = []string{"m1", "m2"}
		v.Players[0].HandCnt = 2
	})
	cursor := &HandCursor{Mode: CursorModeSingle, Index: 1}
	gw := discardGateway{tiles: make(chan string, 1), err: errors.New("当前不是你的回合")}
	view := state.Snapshot()

	res := submitCursorAction(context.Background(), state, cursor, view.Players[0].Hand, gw, view)
	require.Nil(t, res.exit)

	select {
	case got := <-gw.tiles:
		require.Equal(t, "m2", got)
	case <-time.After(time.Second):
		t.Fatal("expected discard request")
	}
	require.Eventually(t, func() bool {
		return strings.Contains(state.Snapshot().UXNotice, "出牌失败: 当前不是你的回合")
	}, time.Second, 10*time.Millisecond)
}

func TestCursorPendingClearsWhenAuthoritativeStepAdvances(t *testing.T) {
	cursor := &HandCursor{Mode: CursorModeSingle, Index: -1, Pending: true, PendingSinceStep: 10}

	view := RoomView{
		Phase:         phaseTable,
		SeatIndex:     0,
		ActingSeat:    0,
		WaitingAction: "discard",
		LastStep:      11,
	}
	view.Players[0].Hand = []string{"m1", "m2"}

	cursor.SyncMode(view)

	require.False(t, cursor.Pending)
	require.Equal(t, int64(0), cursor.PendingSinceStep)
	require.Equal(t, 1, cursor.Index)
	require.True(t, cursor.CanSubmit(), "bot 快速推进后回到自己出牌时，Enter 不能被旧 Pending 卡住")
}

func TestHandleTableKeyArrowsMoveClaimDialogSelection(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.RoomState = "playing"
		v.SeatIndex = 0
		v.ActingSeat = 1
		v.WaitingAction = "claim_window"
		v.PendingTile = "p5"
		v.ClaimCandidates = map[int32][]string{0: {"hu", "pong", "pass"}}
	})
	cursor := &HandCursor{Index: -1}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	dialog := &ClaimDialogState{Actions: []ClaimAction{ClaimActionHu, ClaimActionPong, ClaimActionPass}}

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone), state, stubTableGateway{}, cursor, overlay, nil, &theme, cfg, dialog)

	require.Nil(t, res.exit)
	require.Equal(t, 1, dialog.SelectedIndex)
	require.Equal(t, -1, cursor.Index, "抢答浮窗打开时左右键应切换按钮，不应移动手牌光标")
}

func TestHandleTableKeyEnterMarksExchangeTileBeforeSubmit(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.WaitingAction = "exchange_three"
		v.Players[0].Hand = []string{"m1", "m2", "m3", "p1"}
		v.Players[0].HandCnt = 4
	})
	cursor := &HandCursor{Mode: CursorModeMulti3, Index: 1}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), state, stubTableGateway{}, cursor, overlay, nil, &theme, cfg, nil)

	require.Nil(t, res.exit)
	require.Equal(t, []int{1}, cursor.Marked)
	require.False(t, cursor.Pending)
}

// claimPassGateway 用于 [C3.3] 显式 pass 路径的单元断言：仅记录 Pass / Hu / Pong / Gang 调用计数。
type claimPassGateway struct {
	stubTableGateway
	passes int32
	hus    int32
	pongs  int32
	gangs  int32
}

func (g *claimPassGateway) Pass(context.Context) error { atomic.AddInt32(&g.passes, 1); return nil }
func (g *claimPassGateway) Hu(context.Context) error   { atomic.AddInt32(&g.hus, 1); return nil }
func (g *claimPassGateway) Pong(context.Context) error { atomic.AddInt32(&g.pongs, 1); return nil }
func (g *claimPassGateway) Gang(context.Context, string) error {
	atomic.AddInt32(&g.gangs, 1)
	return nil
}

// TestPlayerJourney_C3_3_ExplicitPassSendsPassRequest 锁定 [C3.3] 显式过走 PassRequest。
//
// 玩家旅程 [C3.3] 要求「显式过必须发 PassRequest；自动兜底超时也必须发 PassRequest」，
// 不依赖服务端默认动作。本用例直接对 submitClaimAction 注入 ClaimActionPass，
// 断言下发的是 Pass 调用而非 Hu / Pong / Gang；同时验证「未知动作」走默认 Pass 兜底
// （历史上有 commit 误把 default 改成 Hu 导致玩家点过被当成胡牌的事故）。
func TestPlayerJourney_C3_3_ExplicitPassSendsPassRequest(t *testing.T) {
	gw := &claimPassGateway{}
	dialog := &ClaimDialogState{Actions: []ClaimAction{ClaimActionHu, ClaimActionPass}}

	submitClaimAction(context.Background(), gw, dialog, ClaimActionPass)
	require.Eventually(t, func() bool { return atomic.LoadInt32(&gw.passes) == 1 }, time.Second, 20*time.Millisecond,
		"[C3.3] ClaimActionPass 必须下发 PassRequest")
	require.True(t, dialog.Pending, "[C3.3] 显式过提交后 dialog 进入 Pending 防抖")
	require.EqualValues(t, 0, atomic.LoadInt32(&gw.hus), "[C3.3] Pass 路径绝不允许走 Hu")
	require.EqualValues(t, 0, atomic.LoadInt32(&gw.pongs))
	require.EqualValues(t, 0, atomic.LoadInt32(&gw.gangs))

	dialog2 := &ClaimDialogState{Actions: []ClaimAction{ClaimActionPass}}
	gw2 := &claimPassGateway{}
	submitClaimAction(context.Background(), gw2, dialog2, ClaimAction("__bogus__"))
	require.Eventually(t, func() bool { return atomic.LoadInt32(&gw2.passes) == 1 }, time.Second, 20*time.Millisecond,
		"[C3.3] 未知 ClaimAction 必须默认走 PassRequest，不得擅自胡 / 杠 / 碰")
}

// TestPlayerJourney_D1_2_CursorLandsOnFreshlyDrawnTile 锁定 [D1.2] 光标停在新摸牌。
//
// 玩家旅程 [D1.2] 要求 DrawTileNotify 到达后下一帧自家光标默认停在新摸牌位置；
// 此前 cli 把光标固定到 handLen-1，新摸牌按花色排序后可能落在中间位置，玩家就得
// 多按几次方向键才能选到刚摸的牌。本用例先 InitialDeal 一副 12 张，再 DrawTile
// 一张靠左的 m1（排序后应落在最前），断言 cursor.SyncMode 切到 Single 时把 Index
// 设到 m1 的真实位置而非 handLen-1。
func TestPlayerJourney_D1_2_CursorLandsOnFreshlyDrawnTile(t *testing.T) {
	view := RoomView{
		Phase:         phaseTable,
		RoomID:        "r1",
		SeatIndex:     0,
		ActingSeat:    0,
		ActingSeats:   []int32{0},
		WaitingAction: "discard",
		PendingTile:   "m1",
	}
	view.Players[0].Hand = []string{"m1", "p3", "p4", "p5", "s2", "s7", "s8"}
	view.Players[0].HandCnt = len(view.Players[0].Hand)
	for i := range view.QueBySeat {
		view.QueBySeat[i] = -1
	}

	cursor := &HandCursor{Mode: CursorModeNone}
	cursor.SyncMode(view)

	require.Equal(t, CursorModeSingle, cursor.Mode, "[D1.2] discard 阶段必须切到 Single 模式")
	require.Equal(t, 0, cursor.Index, "[D1.2] 光标必须停在 PendingTile (m1) 的索引而非 handLen-1")
}

// TestPlayerJourney_D2_1_EnterSilentWhenNotMyTurn 锁定 [D2.1] 非本家回合 Enter 静默无效。
//
// 玩家旅程 [D2.1] 要求「不满足出牌许可时 Enter 无效且不弹错误」。之前 cli 在
// cursor.Mode==None 时仍走 submitCursorAction → noticeInputRejected，弹出
// 「当前阶段不能操作手牌」的副作用。本用例构造他家回合 + None 模式光标，按 Enter
// 必须既不下发 Discard 也不弹任何 UXTransient 通知。
func TestPlayerJourney_D2_1_EnterSilentWhenNotMyTurn(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.WaitingAction = "discard"
		v.ActingSeat = 1
		v.Players[0].Hand = []string{"m1", "m2"}
	})
	cursor := &HandCursor{Mode: CursorModeNone}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	gw := discardGatewayCapture{}

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), state, &gw, cursor, overlay, nil, &theme, cfg, nil)

	require.Nil(t, res.exit)
	require.False(t, gw.called, "[D2.1] 非本家回合 Enter 不得下发 Discard")
	require.Empty(t, state.Snapshot().UXNotice, "[D2.1] 非本家回合 Enter 不得弹任何 UXTransient")
}

// TestPlayerJourney_D2_2_EnterDebouncedAfterSubmit 锁定 [D2.2] Enter 出牌后防抖。
//
// 玩家旅程 [D2.2] 要求出牌请求发出后下一帧该牌变灰 pending，并且重复按 Enter 不
// 重复下发。SubmitAt 把 cursor.Pending=true，下次 Enter 会因 CanSubmit()=false
// 而拒绝。本用例直连 handleTableKey 连按两次 Enter，断言只有第一次到达 gateway。
func TestPlayerJourney_D2_2_EnterDebouncedAfterSubmit(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.WaitingAction = "discard"
		v.ActingSeat = 0
		v.ActingSeats = []int32{0}
		v.Players[0].Hand = []string{"m1", "m2"}
		v.Players[0].HandCnt = 2
	})
	cursor := &HandCursor{Mode: CursorModeSingle, Index: 1}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	gw := &discardGatewayCounter{}

	_ = handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), state, gw, cursor, overlay, nil, &theme, cfg, nil)
	require.True(t, cursor.Pending, "[D2.2] 第一次 Enter 必须把 cursor 切到 Pending")
	_ = handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), state, gw, cursor, overlay, nil, &theme, cfg, nil)

	require.Eventually(t, func() bool { return gw.count() >= 1 }, time.Second, 20*time.Millisecond, "[D2.2] 第一次 Enter 必须真实下发 Discard")
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 1, gw.count(), "[D2.2] 连按 Enter 不得重复下发 Discard")
}

// TestPlayerJourney_E1_2_ExchangeMarkRejectsCrossSuit 锁定 [E1.2] UI 层同花色硬约束。
//
// 玩家旅程 [E1.2] 要求换三张选第二张异花色时 UI 必须直接拒绝标记，不依赖服务端
// 兜底——避免玩家手误后还得等服务端 round-trip 才发现错误。本用例先把万子的
// 索引标进 Marked，再把光标移到筒子上按 Space：cursor.Marked 必须保持不变，
// 且 UXNotice 必须告诉玩家「换三张必须同一花色」。
func TestPlayerJourney_E1_2_ExchangeMarkRejectsCrossSuit(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.WaitingAction = "exchange_three"
		v.RoundPhase = clientv1.Phase_PHASE_EXCHANGE
		v.ActingSeats = []int32{0, 1, 2, 3}
		v.Players[0].Hand = []string{"m1", "m2", "p3", "p4", "s5"}
		v.Players[0].HandCnt = 5
	})
	cursor := &HandCursor{Mode: CursorModeMulti3, Index: 2, Marked: []int{0, 1}}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	gw := queMenGateway{suits: make(chan int32, 1)}

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone), state, gw, cursor, overlay, nil, &theme, cfg, nil)

	require.Nil(t, res.exit)
	require.Equal(t, []int{0, 1}, cursor.Marked, "[E1.2] 异花色 Space 必须被 UI 层立刻拒绝，不写入 Marked")
	require.Contains(t, state.Snapshot().UXNotice, "同一花色", "[E1.2] UI 拒绝必须用通知告诉玩家原因")
}

// TestPlayerJourney_Q1_1_QueMenKeysOnlyMPS 锁定 [Q1.1] 定缺仅接受 m/p/s 三键。
//
// 玩家旅程 v0.5 [Q1.1] 明确「按 m/p/s 提交缺万/缺筒/缺条；其它键忽略且无副作用」。
// 早期实现把 1/2/3 当作快捷键，与抢答 / 出牌阶段的字符可能撞键，并且在不在
// que_men 阶段时按下数字键会弹出「当前不能定缺」的副作用提示，违反规范。
//
// 本用例在 que_men 阶段分别按 2 与 m：2 必须不下发任何 QueMenReq，m 必须下发
// suit=0。这样既覆盖「白名单收紧到 m/p/s」也覆盖「黑名单不产生副作用」。
func TestPlayerJourney_Q1_1_QueMenKeysOnlyMPS(t *testing.T) {
	mkState := func() *AppState {
		st := NewAppState("racoo")
		st.Mutate(func(v *RoomView) {
			v.Phase = phaseTable
			v.RoomID = "r1"
			v.SeatIndex = 0
			v.WaitingAction = "que_men"
		})
		return st
	}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}

	state := mkState()
	cursor := &HandCursor{}
	gw := queMenGateway{suits: make(chan int32, 1)}
	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyRune, '2', tcell.ModNone), state, gw, cursor, overlay, nil, &theme, cfg, nil)
	require.Nil(t, res.exit)
	select {
	case got := <-gw.suits:
		t.Fatalf("[Q1.1] 数字键 2 不得下发 QueMenReq，got suit=%d", got)
	case <-time.After(100 * time.Millisecond):
	}
	require.Empty(t, state.Snapshot().UXNotice, "[Q1.1] 数字键 2 不得产生 UXTransient 副作用")

	state = mkState()
	cursor = &HandCursor{}
	gw = queMenGateway{suits: make(chan int32, 1)}
	res = handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyRune, 'm', tcell.ModNone), state, gw, cursor, overlay, nil, &theme, cfg, nil)
	require.Nil(t, res.exit)
	select {
	case got := <-gw.suits:
		require.Equal(t, int32(0), got, "[Q1.1] 按 m 必须下发缺万 (suit=0)")
	case <-time.After(time.Second):
		t.Fatal("[Q1.1] 按 m 必须下发 QueMenReq")
	}
}

// TestPlayerJourney_Q1_1_QueMenKeysSilentWhenNotAllowed 锁定 [Q1.1] 不在 que_men 阶段时 m/p/s 静默忽略。
//
// 玩家在抢答 / 摸打阶段若误按 m/p/s，旧实现会立刻弹「当前不能定缺」的 UXTransient
// 通知，污染主提示区。本用例构造 waiting_action=discard 的回合，按下 m，断言
// 既无请求下发也无 UXNotice。
func TestPlayerJourney_Q1_1_QueMenKeysSilentWhenNotAllowed(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.WaitingAction = "discard"
		v.ActingSeat = 1
	})
	cursor := &HandCursor{}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	gw := queMenGateway{suits: make(chan int32, 1)}

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyRune, 'm', tcell.ModNone), state, gw, cursor, overlay, nil, &theme, cfg, nil)
	require.Nil(t, res.exit)
	select {
	case <-gw.suits:
		t.Fatal("[Q1.1] 非 que_men 阶段按 m 不得下发请求")
	case <-time.After(100 * time.Millisecond):
	}
	require.Empty(t, state.Snapshot().UXNotice, "[Q1.1] 非 que_men 阶段按 m 不得弹副作用通知")
}

// TestPlayerJourney_N2_1_OfflineEnterTriggersLeaveRoom 锁定 [N2.1] 长离线 Enter → LeaveRoom + 返回大厅。
//
// 当网络浮窗推进到 NetStatusOffline 时，按 Enter 必须显式调 gateway.LeaveRoom 并以
// TableExitLeaveRoom 回到大厅；不得静默忽略或卡在结算/牌桌页。
func TestPlayerJourney_N2_1_OfflineEnterTriggersLeaveRoom(t *testing.T) {
	state := NewAppState("我")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
	})
	cursor := &HandCursor{Index: -1}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	gw := leaveSignalGateway{calls: make(chan struct{}, 1)}

	netOverlay := NewNetOverlay(time.Second)
	start := time.Now()
	netOverlay.Update(false, start)
	netOverlay.Update(false, start.Add(2*time.Second))
	require.Equal(t, NetStatusOffline, netOverlay.Status, "前置：netOverlay 必须已经升级为 Offline")

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone),
		state, gw, cursor, overlay, netOverlay, &theme, cfg, nil)

	require.NotNil(t, res.exit, "[N2.1] Offline 状态下 Enter 必须触发 TableExit 回大厅")
	require.Equal(t, TableExitLeaveRoom, res.exit.Reason)
	select {
	case <-gw.calls:
	case <-time.After(time.Second):
		t.Fatal("[N2.1] Offline 状态下 Enter 必须显式调用 gateway.LeaveRoom")
	}
}
