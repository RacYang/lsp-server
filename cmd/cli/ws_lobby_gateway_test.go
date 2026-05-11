package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// TestApplyJoinResultToStateSetsRoomIDAndPhase 锁住「JoinRoomResp 不带 room_id 时
// 由客户端侧补齐 RoomID 并切 phaseTable」的契约：之前丢这一步导致玩家成功入房后
// main 主循环看不到 view.RoomID，又退回大厅，体感上像「加入失败」。
// 这里同时校验座位号、规则与显示名都被回填，保证大厅返回的元数据不会丢。
func TestApplyJoinResultToStateSetsRoomIDAndPhase(t *testing.T) {
	st := NewAppState("我")
	st.Mutate(func(v *RoomView) { v.Phase = phaseLobby })

	applyJoinResultToState(st, LobbyJoinResult{
		RoomID:      "ROOMX",
		SeatIndex:   2,
		DisplayName: "三缺一",
		RuleID:      "sichuan_xuezhandaodi_huansanzhang",
	})

	view := st.Snapshot()
	require.Equal(t, "ROOMX", view.RoomID)
	require.Equal(t, phaseTable, view.Phase)
	require.EqualValues(t, 2, view.SeatIndex)
	require.Equal(t, "sichuan_xuezhandaodi_huansanzhang", view.RuleID)
	require.Equal(t, "三缺一", view.DisplayName)
}

func TestApplyJoinResultToStateAppliesSeats(t *testing.T) {
	st := NewAppState("我")
	st.Mutate(func(v *RoomView) { v.Phase = phaseLobby })

	applyJoinResultToState(st, LobbyJoinResult{
		RoomID:    "ROOMB",
		SeatIndex: 0,
		Seats: []*clientv1.SeatInfo{
			{SeatIndex: 0, UserId: "u0", Nickname: "我", Online: true, Status: "online"},
			{SeatIndex: 1, UserId: "bot:ROOMB:1", Nickname: "机器人", IsBot: true, Status: "ready"},
			{SeatIndex: 2, UserId: "bot:ROOMB:2", Nickname: "机器人", IsBot: true, Status: "ready"},
			{SeatIndex: 3, UserId: "bot:ROOMB:3", Nickname: "机器人", IsBot: true, Status: "ready"},
		},
	})

	view := st.Snapshot()
	require.Equal(t, phaseTable, view.Phase)
	for seat := 1; seat <= 3; seat++ {
		require.True(t, view.Players[seat].IsBot)
		require.Equal(t, "机器人", view.Players[seat].Nickname)
		require.True(t, view.Players[seat].Ready)
	}
}

// TestApplyJoinResultToStateNoOpOnEmptyRoomID 服务端返回空 room_id 或 state 自身为 nil
// 都视为入房失败，本地 view 必须保持原状（仍在大厅），避免误把假状态推到玩家界面。
func TestApplyJoinResultToStateNoOpOnEmptyRoomID(t *testing.T) {
	st := NewAppState("我")
	st.Mutate(func(v *RoomView) { v.Phase = phaseLobby })

	applyJoinResultToState(st, LobbyJoinResult{})
	applyJoinResultToState(nil, LobbyJoinResult{RoomID: "X"})

	view := st.Snapshot()
	require.Empty(t, view.RoomID)
	require.Equal(t, phaseLobby, view.Phase)
}
