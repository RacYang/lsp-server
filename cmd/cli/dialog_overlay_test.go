package main

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
)

func TestOverlayStateToggleAndClose(t *testing.T) {
	o := &OverlayState{}
	require.False(t, o.IsOpen())

	o.Toggle(OverlayRoomInfo)
	require.True(t, o.IsOpen())
	require.Equal(t, OverlayRoomInfo, o.Kind)

	o.Toggle(OverlayRoomInfo)
	require.False(t, o.IsOpen(), "再次切换同一种应当关闭")

	o.Toggle(OverlayPlayers)
	require.Equal(t, OverlayPlayers, o.Kind)
	o.Toggle(OverlayMenu)
	require.Equal(t, OverlayMenu, o.Kind, "切换到不同种类直接覆盖")
}

func TestOverlayStateMenuMoveWraps(t *testing.T) {
	o := &OverlayState{Kind: OverlayMenu}
	items := overlayMenuItems()
	require.NotEmpty(t, items)

	o.MenuMove(1)
	require.Equal(t, 1, o.SelectedIndex)
	for i := 0; i < len(items); i++ {
		o.MenuMove(1)
	}
	require.Equal(t, 1, o.SelectedIndex, "环绕回到 1")

	o.MenuMove(-2)
	require.Equal(t, len(items)-1, o.SelectedIndex)
}

func TestOverlayStateMenuMoveIgnoredOutsideMenu(t *testing.T) {
	o := &OverlayState{Kind: OverlayRoomInfo, SelectedIndex: 0}
	o.MenuMove(1)
	require.Equal(t, 0, o.SelectedIndex)
}

func TestOverlayStateMenuSelectActions(t *testing.T) {
	items := overlayMenuItems()
	o := &OverlayState{Kind: OverlayMenu}
	for i, want := range items {
		o.SelectedIndex = i
		require.Equal(t, want.Action, o.MenuSelect())
	}
}

func TestOverlayStateMenuSelectOutsideMenuReturnsNone(t *testing.T) {
	o := &OverlayState{Kind: OverlayPlayers}
	require.Equal(t, OverlayMenuActionNone, o.MenuSelect())
}

func TestOverlayRoomInfoLinesContainKeyFields(t *testing.T) {
	view := newWaitingTableView()
	view.RoomID = "R7K2"
	for i := range view.Players {
		view.Players[i].UserID = "u"
	}
	lines := overlayRoomInfoLines(view, OverlayContext{RuleID: "scmj", Theme: TileThemeUnicode})
	joined := strings.Join(lines, "\n")
	require.Contains(t, joined, "R7K2")
	require.Contains(t, joined, "scmj")
	require.Contains(t, joined, "4 / 4")
	require.Contains(t, joined, "unicode")
}

func TestOverlayPlayersLinesShowSelfMarker(t *testing.T) {
	view := newWaitingTableView()
	lines := overlayPlayersLines(view)
	require.Len(t, lines, 4+2, "4 家 + 空行 + 关闭提示")
	hasSelfMark := false
	for _, line := range lines {
		if strings.Contains(line, "★") {
			hasSelfMark = true
		}
	}
	require.True(t, hasSelfMark, "自己一行应有 ★ 标记")
}

func TestDrawOverlayRoomInfoVisibleOnScreen(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, scr.Init())
	defer scr.Fini()
	scr.SetSize(MinTableWidth, MinTableHeight)
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	view := newWaitingTableView()
	view.RoomID = "R7K2"
	o := OverlayState{Kind: OverlayRoomInfo}
	DrawOverlay(scr, layout, view, OverlayContext{RuleID: "scmj", Theme: TileThemeASCII}, o)
	scr.Show()
	out := dumpScreen(scr)
	require.Contains(t, out, "房 间 信 息")
	require.Contains(t, out, "R7K2")
}

func TestDrawOverlayMenuShowsArrow(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, scr.Init())
	defer scr.Fini()
	scr.SetSize(MinTableWidth, MinTableHeight)
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	o := OverlayState{Kind: OverlayMenu, SelectedIndex: 1}
	DrawOverlay(scr, layout, RoomView{}, OverlayContext{Theme: TileThemeUnicode}, o)
	scr.Show()
	out := dumpScreen(scr)
	require.Contains(t, out, "局 内 菜 单")
	require.Contains(t, out, "▶")
	require.Contains(t, out, "返回大厅")
}

func TestDrawOverlayNoneNoOp(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, scr.Init())
	defer scr.Fini()
	scr.SetSize(MinTableWidth, MinTableHeight)
	layout, _ := CalcLayout(MinTableWidth, MinTableHeight)
	DrawOverlay(scr, layout, RoomView{}, OverlayContext{Theme: TileThemeASCII}, OverlayState{})
	scr.Show()
	out := strings.TrimSpace(dumpScreen(scr))
	require.Empty(t, out, "OverlayNone 不应绘制任何字符")
}
