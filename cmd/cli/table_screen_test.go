package main

import (
	"context"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
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
			res := handleOverlayKey(context.Background(), ev, stubTableGateway{}, overlay, &theme, cfg)
			require.Nil(t, res.exit, "Enter 关闭非菜单浮窗不应触发主循环退出")
			require.Equal(t, OverlayNone, overlay.Kind, "Enter 应把非菜单浮窗关闭")
		})
	}
}

// TestHandleOverlayKeyEnterStillTriggersMenuAction 兜底校验:菜单浮窗里的 Enter
// 仍然正常触发动作,不会被新增的"非菜单 Enter 关闭"分支误吃掉。
func TestHandleOverlayKeyEnterStillTriggersMenuAction(t *testing.T) {
	overlay := &OverlayState{Kind: OverlayMenu, SelectedIndex: 2} // 继续游戏
	theme := TileThemeUnicode
	cfg := &Config{}
	ev := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
	res := handleOverlayKey(context.Background(), ev, stubTableGateway{}, overlay, &theme, cfg)
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
	res := handleOverlayKey(context.Background(), ev, stubTableGateway{}, overlay, &theme, cfg)
	require.Nil(t, res.exit)
	require.Equal(t, OverlayNone, overlay.Kind)
}
