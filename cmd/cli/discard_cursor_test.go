package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func makeMyTurnDiscardView() RoomView {
	v := RoomView{Phase: phaseTable, SeatIndex: 1, ActingSeat: 1, WaitingAction: "discard"}
	v.Players[1].Hand = []string{"m1", "m2", "m3", "m4", "m5"}
	for i := range v.QueBySeat {
		v.QueBySeat[i] = -1
	}
	return v
}

func TestDeriveCursorMode(t *testing.T) {
	v := makeMyTurnDiscardView()
	require.Equal(t, CursorModeSingle, DeriveCursorMode(v))

	v.WaitingAction = "exchange_three"
	require.Equal(t, CursorModeMulti3, DeriveCursorMode(v))

	v.ActingSeat = 0
	require.Equal(t, CursorModeNone, DeriveCursorMode(v))

	v = makeMyTurnDiscardView()
	v.SeatIndex = -1
	require.Equal(t, CursorModeNone, DeriveCursorMode(v))
}

func TestHandCursorMoveSingle(t *testing.T) {
	c := &HandCursor{Mode: CursorModeSingle, Index: -1}
	c.Move(1, 5)
	require.Equal(t, 0, c.Index)

	c.Move(1, 5)
	require.Equal(t, 1, c.Index)

	c.Move(-1, 5)
	require.Equal(t, 0, c.Index)

	c.Move(-1, 5)
	require.Equal(t, 0, c.Index, "已经在最左侧再左移仍停留")

	c.Index = 4
	c.Move(1, 5)
	require.Equal(t, 4, c.Index, "已经在最右侧再右移仍停留")
}

func TestHandCursorMoveFromUnsetGoesLeftToEnd(t *testing.T) {
	c := &HandCursor{Mode: CursorModeSingle, Index: -1}
	c.Move(-1, 5)
	require.Equal(t, 4, c.Index, "首次按左方向键应跳到最右,符合摸到新牌后顺手出最右的常见操作")
}

func TestHandCursorPendingIgnoresMove(t *testing.T) {
	c := &HandCursor{Mode: CursorModeSingle, Index: 2, Pending: true}
	c.Move(1, 5)
	require.Equal(t, 2, c.Index)
}

func TestHandCursorToggleMarkAddsAndRemoves(t *testing.T) {
	c := &HandCursor{Mode: CursorModeMulti3, Index: 0}
	require.True(t, c.ToggleMark())
	require.Equal(t, []int{0}, c.Marked)
	c.Index = 2
	require.True(t, c.ToggleMark())
	require.Equal(t, []int{0, 2}, c.Marked)
	c.Index = 0
	require.True(t, c.ToggleMark())
	require.Equal(t, []int{2}, c.Marked)
}

func TestHandCursorToggleMarkRespectsLimitOf3(t *testing.T) {
	c := &HandCursor{Mode: CursorModeMulti3}
	for _, idx := range []int{0, 1, 2} {
		c.Index = idx
		require.True(t, c.ToggleMark())
	}
	c.Index = 3
	require.False(t, c.ToggleMark(), "已经选满 3 张应拒绝再标记")
	require.Equal(t, []int{0, 1, 2}, c.Marked)
}

func TestHandCursorToggleMarkBlockedInSingleMode(t *testing.T) {
	c := &HandCursor{Mode: CursorModeSingle, Index: 0}
	require.False(t, c.ToggleMark())
}

func TestHandCursorCanSubmit(t *testing.T) {
	single := &HandCursor{Mode: CursorModeSingle, Index: -1}
	require.False(t, single.CanSubmit())
	single.Index = 0
	require.True(t, single.CanSubmit())
	single.Pending = true
	require.False(t, single.CanSubmit())

	multi := &HandCursor{Mode: CursorModeMulti3}
	require.False(t, multi.CanSubmit())
	multi.Marked = []int{0, 1}
	require.False(t, multi.CanSubmit())
	multi.Marked = []int{0, 1, 2}
	require.True(t, multi.CanSubmit())
}

func TestHandCursorSubmitTransitions(t *testing.T) {
	c := &HandCursor{Mode: CursorModeSingle, Index: 1}
	require.True(t, c.Submit())
	require.True(t, c.Pending)
	require.False(t, c.Submit(), "Pending 期间再次提交不应重复触发")

	c.RollbackPending()
	require.False(t, c.Pending)
	require.Equal(t, 1, c.Index)
}

func TestHandCursorCancelClearsButNotPending(t *testing.T) {
	c := &HandCursor{Mode: CursorModeMulti3, Index: 1, Marked: []int{0, 1}}
	c.Cancel()
	require.Equal(t, -1, c.Index)
	require.Empty(t, c.Marked)

	c = &HandCursor{Mode: CursorModeSingle, Index: 1, Pending: true}
	c.Cancel()
	require.Equal(t, 1, c.Index, "Pending 状态拒绝 cancel")
}

func TestHandCursorSyncModeResetsOnChange(t *testing.T) {
	c := &HandCursor{Mode: CursorModeSingle, Index: 2}
	v := makeMyTurnDiscardView()
	v.WaitingAction = "exchange_three"
	c.SyncMode(v)
	require.Equal(t, CursorModeMulti3, c.Mode)
	require.Equal(t, -1, c.Index, "模式切换后旧索引必须清掉,避免错位")
}

func TestHandCursorIsMarked(t *testing.T) {
	c := &HandCursor{Mode: CursorModeMulti3, Marked: []int{1, 3}}
	require.True(t, c.IsMarked(1))
	require.True(t, c.IsMarked(3))
	require.False(t, c.IsMarked(0))
	require.False(t, c.IsMarked(2))
}
