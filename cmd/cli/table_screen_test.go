package main

import (
	"context"
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

func (stubTableGateway) Ready(context.Context) error                   { return nil }
func (stubTableGateway) Discard(context.Context, string) error         { return nil }
func (stubTableGateway) ExchangeThree(context.Context, []string) error { return nil }
func (stubTableGateway) QueMen(context.Context, int32) error           { return nil }
func (stubTableGateway) Pong(context.Context) error                    { return nil }
func (stubTableGateway) Gang(context.Context, string) error            { return nil }
func (stubTableGateway) Hu(context.Context) error                      { return nil }
func (stubTableGateway) Pass(context.Context) error                    { return nil }
func (stubTableGateway) LeaveRoom(context.Context) error               { return nil }
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
	layout, ok := CalcLayout(120, MinTableHeight)
	require.True(t, ok)
	require.True(t, layout.Wide)
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
