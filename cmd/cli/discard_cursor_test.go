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

	v.WaitingAction = "que_men"
	require.Equal(t, CursorModeQueMen, DeriveCursorMode(v))

	// 川麻血战换三张是 4 家并发；本地 SeatIndex=1 即便 ActingSeat=0 也应能选牌，
	// 否则非 dealer 玩家在换三张阶段会陷入死锁（按任何键都没反应）。
	v.WaitingAction = "exchange_three"
	v.ActingSeat = 0
	require.Equal(t, CursorModeMulti3, DeriveCursorMode(v))

	// PhaseDiscard 仍然是轮转出牌，非 ActingSeat 不能操作手牌。
	v.WaitingAction = "discard"
	require.Equal(t, CursorModeNone, DeriveCursorMode(v))

	v = makeMyTurnDiscardView()
	v.SeatIndex = -1
	require.Equal(t, CursorModeNone, DeriveCursorMode(v))

	// SeatIndex 合法但还没入座成功（极端情况）也应该退回 None。
	v.WaitingAction = "exchange_three"
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

	que := &HandCursor{Mode: CursorModeQueMen, Index: -1}
	require.False(t, que.CanSubmit())
	que.Index = 1
	require.True(t, que.CanSubmit())
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
	// 模式切换时旧索引必须先 Reset,然后按新模式的预选规则重新派生。
	// 这里从 Single(Index=2) 切到 Multi3, Reset 清掉旧 Marked 与 Pending,
	// 再由 Multi3 预选规则把 Index 落到 0,避免出现"换模式后 Index 残留越位"。
	c := &HandCursor{Mode: CursorModeSingle, Index: 2, Marked: []int{0, 1}, Pending: true}
	v := makeMyTurnDiscardView()
	v.WaitingAction = "exchange_three"
	c.SyncMode(v)
	require.Equal(t, CursorModeMulti3, c.Mode)
	require.Equal(t, 0, c.Index, "切到 Multi3 应按新模式预选第一张,而不是残留旧 Index")
	require.Empty(t, c.Marked, "Reset 必须清空旧 Marked")
	require.False(t, c.Pending, "Reset 必须清掉 Pending")
}

func TestHandCursorSyncModeIntoSinglePreselectsLastTile(t *testing.T) {
	// 切入单选出牌模式时,应自动把光标定到最右一张（通常是刚摸的牌）,
	// 让玩家直接 Enter 即可顺手出最右那张,避免出现"按 Enter 没反应"的疑惑。
	c := &HandCursor{}
	v := makeMyTurnDiscardView()
	c.SyncMode(v)
	require.Equal(t, CursorModeSingle, c.Mode)
	require.Equal(t, len(v.Players[v.SeatIndex].Hand)-1, c.Index)
	require.True(t, c.CanSubmit(), "进入出牌阶段后无需先按方向键即可提交")
}

func TestHandCursorSyncModeIntoMulti3PreselectsFirstTile(t *testing.T) {
	// 多选换三张模式: Index 必须落在合法位置,否则 Space (ToggleMark) 会因
	// Index<0 静默无效,玩家会以为 Space 也"没反应"。
	// 但仍然不能替玩家自动 Mark,Mark 操作必须由玩家显式触发。
	c := &HandCursor{}
	v := makeMyTurnDiscardView()
	v.WaitingAction = "exchange_three"
	c.SyncMode(v)
	require.Equal(t, CursorModeMulti3, c.Mode)
	require.Equal(t, 0, c.Index, "Multi3 切入应让光标落在第一张,Space 立刻可用")
	require.Empty(t, c.Marked, "切入 Multi3 时不应自动标记任何牌")
	require.False(t, c.CanSubmit(), "未标记 3 张时不能提交")
}

func TestHandCursorSyncModeIntoQueMenPreselectsWeakestSuit(t *testing.T) {
	c := &HandCursor{}
	v := makeMyTurnDiscardView()
	v.WaitingAction = "que_men"
	v.Players[v.SeatIndex].Hand = []string{"m1", "m2", "p1", "p2", "s9"}
	c.SyncMode(v)
	require.Equal(t, CursorModeQueMen, c.Mode)
	require.Equal(t, 2, c.Index, "定缺切入应默认落到当前数量最少的花色")
	require.True(t, c.CanSubmit())
}

func TestHandCursorQueMenMovesAcrossThreeSuits(t *testing.T) {
	c := &HandCursor{Mode: CursorModeQueMen, Index: -1}
	c.Move(1, 0)
	require.Equal(t, 0, c.Index)
	c.Move(1, 0)
	require.Equal(t, 1, c.Index)
	c.Move(1, 0)
	require.Equal(t, 2, c.Index)
	c.Move(1, 0)
	require.Equal(t, 2, c.Index)
}

func TestHandCursorSyncModeClampsIndexWhenHandShrinks(t *testing.T) {
	// 同回合内手牌长度变化（如自杠后摸新牌再因后续回合直接缩短）时,
	// 旧 Index 不应越界停留,需 clamp 回末尾,避免 Enter 静默 no-op。
	c := &HandCursor{Mode: CursorModeSingle, Index: 12}
	v := makeMyTurnDiscardView()
	c.SyncMode(v)
	require.Equal(t, CursorModeSingle, c.Mode)
	require.Equal(t, len(v.Players[v.SeatIndex].Hand)-1, c.Index)
	require.True(t, c.CanSubmit())
}

func TestHandCursorSyncModeClearsIndexWhenHandEmpties(t *testing.T) {
	// 边界情形:Mode 仍然是 Single 但手牌瞬时为 0（理论极端态）,
	// Index 应回退到 -1,避免读取空切片 panic。
	c := &HandCursor{Mode: CursorModeSingle, Index: 5}
	v := makeMyTurnDiscardView()
	v.Players[v.SeatIndex].Hand = nil
	c.SyncMode(v)
	require.Equal(t, -1, c.Index)
	require.False(t, c.CanSubmit())
}

func TestHandCursorIsMarked(t *testing.T) {
	c := &HandCursor{Mode: CursorModeMulti3, Marked: []int{1, 3}}
	require.True(t, c.IsMarked(1))
	require.True(t, c.IsMarked(3))
	require.False(t, c.IsMarked(0))
	require.False(t, c.IsMarked(2))
}
