package main

import (
	"context"
	"errors"
	"strings"
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
	cursor := &HandCursor{}
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
	cursor := &HandCursor{}
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

func TestHandleTableKeyNumericQueMen(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "r1"
		v.SeatIndex = 0
		v.WaitingAction = "que_men"
	})
	cursor := &HandCursor{}
	overlay := &OverlayState{}
	theme := TileThemeUnicode
	cfg := &Config{}
	gw := queMenGateway{suits: make(chan int32, 1)}

	res := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyRune, '2', tcell.ModNone), state, gw, cursor, overlay, nil, &theme, cfg, nil)

	require.Nil(t, res.exit)
	select {
	case got := <-gw.suits:
		require.Equal(t, int32(1), got)
	case <-time.After(time.Second):
		t.Fatal("expected QueMen request")
	}
}
