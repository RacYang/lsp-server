package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func TestApplyInitialDealDrawDiscardAndSnapshot(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0", SessionToken: "tok"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_JoinRoomResp{JoinRoomResp: &clientv1.JoinRoomResponse{SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{SeatIndex: 0, Tiles: []string{"s3", "m1", "p2"}}}})
	view := st.Snapshot()
	require.Equal(t, []string{"m1", "p2", "s3"}, view.Players[0].Hand)

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{SeatIndex: 0, Tile: "m9"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{SeatIndex: 0, Action: "discard", Tile: "p2"}}})
	view = st.Snapshot()
	require.NotContains(t, view.Players[0].Hand, "p2")
	require.Contains(t, view.Players[0].Discards, "p2")

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Snapshot{Snapshot: &clientv1.SnapshotNotify{
		RoomId:           "r1",
		PlayerIds:        []string{"u0", "u1", "u2", "u3"},
		QueSuitBySeat:    []int32{0, 1, 2, -1},
		State:            "playing",
		WaitingAction:    "discard",
		ActingSeat:       2,
		AvailableActions: []string{"discard"},
		YourHandTiles:    []string{"m2", "m1"},
		DiscardsBySeat:   []*clientv1.SeatTiles{{SeatIndex: 0, Tiles: []string{"p9"}}},
		MeldsBySeat:      []*clientv1.SeatTiles{{SeatIndex: 1, Tiles: []string{"pong:m3"}}},
	}}})
	view = st.Snapshot()
	require.Equal(t, "r1", view.RoomID)
	require.Equal(t, []string{"m1", "m2"}, view.Players[0].Hand)
	require.Equal(t, []string{"p9"}, view.Players[0].Discards)
	require.Equal(t, []string{"pong:m3"}, view.Players[1].Melds)
	require.Equal(t, int32(2), view.ActingSeat)
}

func TestApplyDiscardDoesNotKeepDiscarderAsActingSeat(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{SeatIndex: 1, Tile: "p9"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{SeatIndex: 1, Action: "discard", Tile: "p9"}}})

	view := st.Snapshot()
	require.Equal(t, int32(-1), view.ActingSeat, "discard.seat 是刚出牌的人,不是下一位行动者")
	require.Equal(t, []string{"p9"}, view.Players[1].Discards)
	require.Empty(t, view.Players[0].Discards)
}

func TestApplyPongRemovesClaimedDiscardFromRiver(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{SeatIndex: 1, Action: "discard", Tile: "p9"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{SeatIndex: 2, Action: "pong", Tile: "p9"}}})

	view := st.Snapshot()
	require.Empty(t, view.Players[1].Discards)
	require.Equal(t, []string{"pong:p9"}, view.Players[2].Melds)
	require.Equal(t, int32(2), view.ActingSeat)
}

func TestApplyResponsesAndSettlement(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_ReadyResp{ReadyResp: &clientv1.ReadyResponse{}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_PongResp{PongResp: &clientv1.PongResponse{ErrorCode: clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, ErrorMessage: "不能碰"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Settlement{Settlement: &clientv1.SettlementNotify{RoomId: "r", DetailText: "结算"}}})
	view := st.Snapshot()
	require.Equal(t, PhaseSettlement, DeriveInteractionModel(view).Phase)
	require.Equal(t, "不能碰", view.LastError)
	require.NotNil(t, view.LastSettlement)
}

func TestQueMenStartGameDrawKeepsAuthoritativePhase(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{SeatIndex: 0, Tiles: []string{"m1", "m2", "m3"}}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_QueMenDone{QueMenDone: &clientv1.QueMenDoneNotify{
		QueSuitBySeat: []int32{0, 1, 2, 0},
		Phase:         clientv1.Phase_PHASE_DRAW,
		Step:          1,
		ActingSeats:   []int32{0},
	}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_StartGame{StartGame: &clientv1.StartGameNotify{
		RoomId:      "r1",
		DealerSeat:  0,
		Phase:       clientv1.Phase_PHASE_DRAW,
		Step:        1,
		ActingSeats: []int32{0},
	}}})

	view := st.Snapshot()
	require.Equal(t, clientv1.Phase_PHASE_DRAW, view.RoundPhase)
	require.Equal(t, "none", view.WaitingAction)
	require.Equal(t, 3, view.Players[0].HandCnt)

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{
		SeatIndex:   0,
		Tile:        "p9",
		Phase:       clientv1.Phase_PHASE_DISCARD,
		Step:        2,
		ActingSeats: []int32{0},
	}}})
	view = st.Snapshot()
	require.Equal(t, clientv1.Phase_PHASE_DISCARD, view.RoundPhase)
	require.Equal(t, "discard", view.WaitingAction)
	require.Equal(t, []string{"m1", "m2", "m3", "p9"}, view.Players[0].Hand)
}

func TestApplyExchangeDoneReplacesExchangedTiles(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{
		SeatIndex: 0,
		Tiles:     []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "s1", "s2", "s3", "s4"},
	}}})

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_ExchangeThreeDone{ExchangeThreeDone: &clientv1.ExchangeThreeDoneNotify{
		PerSeat: []*clientv1.SeatTiles{{
			SeatIndex: 0,
			Tiles:     []string{"p7", "p8", "p9"},
		}},
		YourExchangedAway: []string{"m1", "m2", "m3"},
	}}})

	view := st.Snapshot()
	require.Len(t, view.Players[0].Hand, 13)
	require.NotContains(t, view.Players[0].Hand, "m1")
	require.NotContains(t, view.Players[0].Hand, "m2")
	require.NotContains(t, view.Players[0].Hand, "m3")
	require.Contains(t, view.Players[0].Hand, "p7")
	require.Contains(t, view.Players[0].Hand, "p8")
	require.Contains(t, view.Players[0].Hand, "p9")
}

func TestApplyExchangeDoneDoesNotRemoveOptimisticExchangeTwice(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Mutate(func(v *RoomView) {
		v.Players[0].Hand = []string{"m1", "s1"}
		v.Players[0].HandCnt = 2
		v.PendingExchangeAway = []string{"m1", "m1", "m2"}
	})

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_ExchangeThreeDone{ExchangeThreeDone: &clientv1.ExchangeThreeDoneNotify{
		PerSeat: []*clientv1.SeatTiles{{
			SeatIndex: 0,
			Tiles:     []string{"p7", "p8", "p9"},
		}},
		YourExchangedAway: []string{"m1", "m1", "m2"},
	}}})

	view := st.Snapshot()
	require.Empty(t, view.PendingExchangeAway)
	require.Equal(t, []string{"m1", "p7", "p8", "p9", "s1"}, view.Players[0].Hand)
}

func TestSnapshotStepDropsStaleDrawTile(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Snapshot{Snapshot: &clientv1.SnapshotNotify{
		RoomId:        "r1",
		State:         "playing",
		LastStep:      8,
		YourHandTiles: []string{"m1", "m2", "m3", "m4"},
	}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{
		SeatIndex: 0,
		Tile:      "p9",
		Step:      8,
	}}})

	view := st.Snapshot()
	require.Equal(t, []string{"m1", "m2", "m3", "m4"}, view.Players[0].Hand)
}

func TestApplyLoginResetsLobbyStateWhenNotResumed(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{
		RoomId:      "r1",
		SeatIndex:   1,
		RuleId:      "sichuan_xuezhandaodi_huansanzhang",
		DisplayName: "旧房间",
	}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Settlement{Settlement: &clientv1.SettlementNotify{RoomId: "r1"}}})

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		UserId:  "u0",
		Resumed: false,
	}}})

	view := st.Snapshot()
	require.Equal(t, phaseLobby, view.Phase)
	require.Empty(t, view.RoomID)
	require.Equal(t, int32(-1), view.SeatIndex)
	require.Empty(t, view.DisplayName)
	require.Empty(t, view.RoomState)
	require.Empty(t, view.WaitingAction)
	require.Nil(t, view.AvailableActions)
	require.Nil(t, view.LastSettlement)
}

func TestApplyLoginResumedDoesNotForceTable(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		UserId:  "u0",
		Resumed: true,
	}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Snapshot{Snapshot: &clientv1.SnapshotNotify{RoomId: "r1", State: "playing"}}})

	view := st.Snapshot()
	require.Equal(t, phaseLobby, view.Phase)
	require.Empty(t, view.RoomID)
	require.Equal(t, "r1", view.ResumeRoomID)
	require.True(t, view.SuppressAutoResume)
}

func TestLeaveRoomLocallyDropsStaleSnapshot(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	roomID := st.LeaveRoomLocally("")
	require.Equal(t, "r1", roomID)

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Snapshot{Snapshot: &clientv1.SnapshotNotify{RoomId: "r1", State: "playing"}}})
	view := st.Snapshot()
	require.Equal(t, phaseLobby, view.Phase)
	require.Empty(t, view.RoomID)
	require.Equal(t, "r1", view.PendingLeaveRoomID)
}

func TestApplySeatInfosUpdatesPlayerLabels(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Snapshot{Snapshot: &clientv1.SnapshotNotify{
		RoomId: "r1",
		Seats: []*clientv1.SeatInfo{
			{SeatIndex: 0, UserId: "u0", Nickname: "alice"},
			{SeatIndex: 1, UserId: "bot:r1:1", Nickname: "机器人", IsBot: true, Surrendered: true},
		},
	}}})
	view := st.Snapshot()
	require.Equal(t, "alice", view.Players[0].Nickname)
	require.True(t, view.Players[1].IsBot)
	require.True(t, view.Players[1].Surrendered)
}

func TestApplyRouteRedirectMarksReconnecting(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{
		WsUrl: "wss://next/ws",
	}}})
	view := st.Snapshot()
	require.True(t, view.Reconnecting)
	require.Equal(t, "服务端要求切换网关", view.LastError)
}
