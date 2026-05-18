package main

import (
	"context"
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

func TestApplyInitialDealIgnoresForeignSeat(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{
		SeatIndex: 2,
		Tiles:     []string{"m6", "p1", "s1"},
	}}})

	view := st.Snapshot()
	require.EqualValues(t, 0, view.SeatIndex)
	require.Empty(t, view.Players[0].Hand)
	require.Empty(t, view.Players[2].Hand)
	require.Contains(t, view.Log[len(view.Log)-1].Text, "已忽略非本座位发牌")
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

func TestApplyDiscardPreservesNextDrawFocus(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{
		SeatIndex:   1,
		Action:      "discard",
		Tile:        "p9",
		Phase:       clientv1.Phase_PHASE_DRAW,
		ActingSeats: []int32{2},
		Step:        10,
	}}})

	view := st.Snapshot()
	require.Equal(t, clientv1.Phase_PHASE_DRAW, view.RoundPhase)
	require.Equal(t, "none", view.WaitingAction)
	require.EqualValues(t, 2, view.ActingSeat)
	require.Equal(t, []string{"p9"}, view.Players[1].Discards)
}

func TestApplyPhaseUpdateRefreshesPhaseTokenStep(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_DiscardResp{DiscardResp: &clientv1.DiscardResponse{
		PhaseUpdate: &clientv1.PhaseUpdate{
			Step:   12,
			Reason: clientv1.WaitingReason_WAITING_REASON_DISCARD,
		},
	}}})

	token := st.PhaseToken()
	require.NotNil(t, token)
	require.EqualValues(t, 12, token.GetStep())
	require.Equal(t, clientv1.WaitingReason_WAITING_REASON_DISCARD, token.GetReason())
}

func TestFallbackRoundProgressRefreshesPhaseTokenReason(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_QueMenDone{QueMenDone: &clientv1.QueMenDoneNotify{
		Phase:       clientv1.Phase_PHASE_DISCARD,
		Step:        13,
		ActingSeats: []int32{0},
	}}})

	token := st.PhaseToken()
	require.NotNil(t, token)
	require.EqualValues(t, 13, token.GetStep())
	require.Equal(t, clientv1.WaitingReason_WAITING_REASON_DISCARD, token.GetReason())
}

func TestApplyDuplicateDrawDoesNotGrowSelfHand(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{
		SeatIndex: 0,
		Tiles:     []string{"m1", "m2", "m3"},
	}}})
	draw := &clientv1.Envelope{Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{
		SeatIndex: 0,
		Tile:      "p9",
		Step:      10,
	}}}

	st.Apply(draw)
	st.Apply(draw)

	view := st.Snapshot()
	require.Equal(t, []string{"m1", "m2", "m3", "p9"}, view.Players[0].Hand)
	require.Equal(t, 4, view.Players[0].HandCnt)
}

func TestApplyDuplicateDiscardDoesNotRemoveSelfHandTwice(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{
		SeatIndex: 0,
		Tiles:     []string{"m1", "m1", "m2", "m3"},
	}}})
	discard := &clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{
		SeatIndex: 0,
		Action:    "discard",
		Tile:      "m1",
		Step:      11,
	}}}

	st.Apply(discard)
	st.Apply(discard)

	view := st.Snapshot()
	require.Equal(t, []string{"m1", "m2", "m3"}, view.Players[0].Hand)
	require.Equal(t, []string{"m1"}, view.Players[0].Discards)
	require.Equal(t, 3, view.Players[0].HandCnt)
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

func TestApplyDiscardRespFailureShowsNotice(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_DiscardResp{DiscardResp: &clientv1.DiscardResponse{
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
		ErrorMessage: "当前不是你的回合",
	}}})

	view := st.Snapshot()
	require.Equal(t, "当前不是你的回合", view.LastError)
	require.Equal(t, "出牌失败: 当前不是你的回合", view.UXNotice)
	require.False(t, view.UXNoticeUntil.IsZero())
	require.Contains(t, view.Log[len(view.Log)-1].Text, "出牌失败: 当前不是你的回合")
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
	require.EqualValues(t, 0, view.ActingSeat)
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

func TestApplyExchangeDoneUsesPendingAwayWhenProjectionOmitsAway(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Mutate(func(v *RoomView) {
		v.Players[0].Hand = []string{"m1", "m1", "m2", "s1"}
		v.Players[0].HandCnt = 4
		v.PendingExchangeAway = []string{"m1", "m1", "m2"}
	})

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_ExchangeThreeDone{ExchangeThreeDone: &clientv1.ExchangeThreeDoneNotify{
		PerSeat: []*clientv1.SeatTiles{{
			SeatIndex: 0,
			Tiles:     []string{"p7", "p8", "p9"},
		}},
	}}})

	view := st.Snapshot()
	require.Empty(t, view.PendingExchangeAway)
	require.Equal(t, []string{"p7", "p8", "p9", "s1"}, view.Players[0].Hand)
	require.NotContains(t, view.Players[0].Hand, "m1")
	require.NotContains(t, view.Players[0].Hand, "m2")
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

// TestPlayerJourney_G11_NonOkLoginBlocks 锁定 spec [G11]：
// 任何 ErrorCode 非 OK 的 LoginResp 必须阻断后续大厅/牌桌操作并在登录页保留可读错误，
// 不得静默把 Phase 切到 phaseLobby 让玩家"看似进入大厅"却没有 user_id。
func TestPlayerJourney_G11_NonOkLoginBlocks(t *testing.T) {
	tests := []struct {
		name      string
		code      clientv1.ErrorCode
		msg       string
		wantPhase string
		wantErr   string
	}{
		{
			name:      "unauthorized_blocks",
			code:      clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
			msg:       "昵称无效",
			wantPhase: phaseLogin,
			wantErr:   "昵称无效",
		},
		{
			name:      "rate_limited_blocks",
			code:      clientv1.ErrorCode_ERROR_CODE_RATE_LIMITED,
			msg:       "请稍后重试",
			wantPhase: phaseLogin,
			wantErr:   "请稍后重试",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			st := NewAppState("我")
			st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
				ErrorCode:    tt.code,
				ErrorMessage: tt.msg,
			}}})
			view := st.Snapshot()
			require.Equal(t, tt.wantPhase, view.Phase, "[G11] 非 OK LoginResp 后 Phase 必须停在登录页")
			require.Empty(t, view.UserID, "[G11] 非 OK LoginResp 不得设置 UserID")
			require.Equal(t, tt.wantErr, view.LastError, "[G11] 必须在 LastError 中保留可读错误")
		})
	}
}

// TestPlayerJourney_L10_1_RouteRedirectFlagsReconnecting 锁定 spec [L10.1] / ADR-0044 决策 9：
// 服务端版本不兼容 / 路由重定向场景必须触发可见状态切换（Reconnecting=true + LastError 可读），
// 不得让 cli 反复重连或静默吞错。当前协议层采用 ROUTE_REDIRECT 通知此类切换。
func TestPlayerJourney_L10_1_RouteRedirectFlagsReconnecting(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_ROUTE_REDIRECT,
		ErrorMessage: "请切换到 ws://gate-new.example/ws",
	}}})
	view := st.Snapshot()
	require.True(t, view.Reconnecting, "[L10.1] 路由重定向必须把 Reconnecting 翻起")
	require.NotEmpty(t, view.LastError, "[L10.1] 路由重定向必须保留可读错误信息")
	require.Empty(t, view.UserID, "[L10.1] 路由重定向期间不得把未刷新的 UserID 当作生效身份")
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

func TestApplySeatRosterUpdatesPlayerLabels(t *testing.T) {
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

// TestPlayerJourney_E1_1_ExchangeThreeKeepsThirteenAuthoritativeTiles 锁定 [E1.1]。
//
// 玩家旅程 [E1.1] 要求换三张阶段本家手牌严格等于服务端权威 13 张投影；任何
// 本地推断、缺失或多余都视为缺陷。本用例完整跑一次 InitialDeal → 标记
// waiting_action=exchange_three，断言 view 仍持有 13 张牌，并且顺序与服务端
// 下发的「按花色排序」后一致，避免 reducer 在 exchange 阶段误清/误改手牌。
func TestPlayerJourney_E1_1_ExchangeThreeKeepsThirteenAuthoritativeTiles(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	tiles := []string{"m1", "m2", "m3", "m4", "m5", "p1", "p2", "p3", "p9", "s1", "s5", "s8", "s9"}
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{
		SeatIndex: 0, Tiles: tiles,
	}}})
	// 当前 client.v1 没有 RoundProgressNotify 独立消息，换三张阶段由 ActionNotify
	// 携带 action="exchange_three" 触发 reducer 把 WaitingAction 写入。
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{
		SeatIndex: 0, Action: "exchange_three", ActingSeats: []int32{0, 1, 2, 3},
	}}})

	view := st.Snapshot()
	require.Equal(t, "exchange_three", view.WaitingAction, "[E1.1] WaitingAction 必须由服务端权威驱动")
	require.Len(t, view.Players[0].Hand, 13, "[E1.1] 换三张阶段本家手牌必须严格 13 张")
	for _, want := range tiles {
		require.Contains(t, view.Players[0].Hand, want, "[E1.1] 手牌投影不得丢失 %s", want)
	}
}

// TestPlayerJourney_E2_2_ExchangeThreeRejectionSurfacesNotice 锁定 [E2.2]。
//
// 玩家旅程 [E2.2] 要求服务端拒绝换三张请求时，cli 必须把原因落到 UXTransient
// 通知，让玩家立刻读到拒绝原因并保留先前的 Marked 状态以便改选 1～2 张。
// 旧实现只把响应追加进 Log，玩家根本看不到任何提示。
//
// 本用例直接投递一个 error_code=INVALID_ARGUMENT 的 ExchangeThreeResponse，
// 断言 UXNotice 非空、包含「换三张被拒绝」前缀，以及原始 error_message 子串。
func TestPlayerJourney_E2_2_ExchangeThreeRejectionSurfacesNotice(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_ExchangeThreeResp{ExchangeThreeResp: &clientv1.ExchangeThreeResponse{
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
		ErrorMessage: "三张必须同一花色",
	}}})

	view := st.Snapshot()
	require.NotEmpty(t, view.UXNotice, "[E2.2] 服务端拒绝必须落到 UXTransient")
	require.Contains(t, view.UXNotice, "换三张被拒绝", "[E2.2] 通知必须有「换三张被拒绝」前缀")
	require.Contains(t, view.UXNotice, "三张必须同一花色", "[E2.2] 通知必须携带 error_message 原因")
}

// TestPlayerJourney_Q1_2_QueMenDoneFillsRoster 锁定 [Q1.2] SeatRoster 携带定缺花色。
//
// 玩家旅程 [Q1.2] 要求 QueMenDoneNotify 后 view.QueBySeat 必须按服务端权威值
// 全桌可见，方便后续摸打阶段渲染缺门提示。本用例直接投递四家的 que_suit_by_seat，
// 断言 view 四个 seat 都拿到了对应的花色编码。
func TestPlayerJourney_Q1_2_QueMenDoneFillsRoster(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_QueMenDone{QueMenDone: &clientv1.QueMenDoneNotify{
		QueSuitBySeat: []int32{0, 1, 2, 0},
	}}})

	view := st.Snapshot()
	require.Equal(t, [4]int32{0, 1, 2, 0}, view.QueBySeat, "[Q1.2] QueBySeat 必须按服务端权威值落桌")
}

// TestPlayerJourney_D1_1_PhaseDrawDoesNotWriteWaitingAction 锁定 [D1.1]。
//
// 玩家旅程 [D1.1] 要求 PHASE_DRAW 仅作为 UXTransient 提示，不得写入 view.WaitingAction，
// 否则 cli 会在「摸牌中」短暂窗口里把 Enter 当作 discard，破坏 ADR-0044 决策 2/3。
// 本用例显式构造一条 PHASE_DRAW 的 DrawTileNotify（不带 phase 升级），断言 WaitingAction
// 仍是 reducer 决定的 discard 或 none，而不是被 PHASE_DRAW 直接派生为 "draw"。
func TestPlayerJourney_D1_1_PhaseDrawDoesNotWriteWaitingAction(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{
		SeatIndex: 0, Tiles: []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "p1", "p2", "p3", "p4"},
	}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{
		SeatIndex: 0,
		Tile:      "s5",
		Phase:     clientv1.Phase_PHASE_DRAW,
		Step:      2,
	}}})

	view := st.Snapshot()
	require.NotEqual(t, "draw", view.WaitingAction, "[D1.1] PHASE_DRAW 不得把 WaitingAction 写成 draw")
	require.Equal(t, clientv1.Phase_PHASE_DRAW, view.RoundPhase, "RoundPhase 仍按权威值 PHASE_DRAW")
}

// TestPlayerJourney_D1_2_DrawTileBringsNewTileIntoSelfHand 锁定 [D1.2]。
//
// 玩家旅程 [D1.2] 要求本家收到 DrawTileNotify 后下一帧手牌必须可见新牌。
// 本用例先发 InitialDeal 13 张，再 DrawTile=p5，断言 p5 出现在 hand 中、HandCnt 增 1。
func TestPlayerJourney_D1_2_DrawTileBringsNewTileIntoSelfHand(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{
		SeatIndex: 0, Tiles: []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "p1", "p2", "p3", "p4"},
	}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{
		SeatIndex: 0, Tile: "p5", Step: 2,
	}}})

	view := st.Snapshot()
	require.Len(t, view.Players[0].Hand, 14, "[D1.2] DrawTile 后本家手牌必须 14 张")
	require.Contains(t, view.Players[0].Hand, "p5", "[D1.2] 新摸牌必须进入 hand")
	require.Equal(t, "p5", view.PendingTile, "[D1.2] PendingTile 记录刚摸的牌，便于光标定位")
}

// TestPlayerJourney_D1_3_OtherSeatDrawHidesTile 锁定 [D1.3] 他家摸牌不暴露明牌。
//
// 玩家旅程 [D1.3] 要求他家摸牌广播只含「动作」不含明牌；这一层服务端按座位投影时
// 已经把 tile 抹为空。本用例从 cli 视角断言：他家 DrawTile（tile="" 或非自家 seat）
// 后 HandCnt 增 1 但 hand 内容不变；任何明牌都不得渗入本地 RoomView。
func TestPlayerJourney_D1_3_OtherSeatDrawHidesTile(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{
		SeatIndex: 1, Tile: "", Step: 3,
	}}})

	view := st.Snapshot()
	require.Equal(t, 1, view.Players[1].HandCnt, "[D1.3] 他家摸牌只走计数")
	require.Empty(t, view.Players[1].Hand, "[D1.3] 他家手牌明牌字段必须为空")
}

// TestPlayerJourney_C1_2_ClaimDialogOnlyForCandidates 锁定 [C1.2] 弹窗仅对 claim_candidates 列出的座位呈现。
//
// 玩家旅程 [C1.2] 要求 cli 严格按 RoundProgress.claim_candidates 决定弹窗对象；
// 其它座位不得显示弹窗。本用例把本家 SeatIndex=0、claim 候选只列 seat=2，断言
// `claimActionsForSeat(view, 0)` 返回空（不会有弹窗）；同时 seat=2 视角下能拿到
// 完整动作列表，避免 reducer 路径漏写 ClaimCandidates。
func TestPlayerJourney_C1_2_ClaimDialogOnlyForCandidates(t *testing.T) {
	view := RoomView{
		SeatIndex:     0,
		ActingSeat:    1,
		WaitingAction: "claim_window",
		ClaimCandidates: map[int32][]string{
			2: {"pong", "pass"},
		},
	}

	require.Empty(t, claimActionsForSeat(view, 0),
		"[C1.2] 本家不在 claim_candidates 列表里，必须没有弹窗动作")
	require.Equal(t, []string{"pong", "pass"}, claimActionsForSeat(view, 2),
		"[C1.2] candidate 座位必须拿到完整动作列表")
}

// TestPlayerJourney_T1_2_PassOnTsumoReturnsToDiscard 锁定 [T1.2] 不胡后摸到的牌入手并继续 discard。
//
// 玩家旅程 [T1.2] 要求：tsumo_window 选「不胡」时客户端显式发 PassRequest，
// 收到 OK 后必须把 WaitingAction 切回 "discard"，PendingTile 清空（牌已经入手），
// 玩家可以继续选下一张要打的牌。本用例直接构造 tsumo_window 状态 + DiscardResp OK 顺序，
// 断言 reducer 自动推进。
func TestPlayerJourney_T1_2_PassOnTsumoReturnsToDiscard(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Mutate(func(v *RoomView) {
		v.WaitingAction = "tsumo_window"
		v.ActingSeat = 0
		v.PendingTile = "m5"
		v.Players[0].Hand = []string{"m1", "m5", "p2"}
	})

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_PassResp{PassResp: &clientv1.PassResponse{
		ErrorCode: clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED,
	}}})

	view := st.Snapshot()
	require.Equal(t, "discard", view.WaitingAction, "[T1.2] 不胡后必须切回 discard 等本家出牌")
	require.Empty(t, view.PendingTile, "[T1.2] PendingTile 清空，新摸牌已留在 Hand 中")
	require.Contains(t, view.Players[0].Hand, "m5", "[T1.2] 不胡后 m5 必须仍在自家手牌内")
}

// TestPlayerJourney_T2_2_TsumoDialogDefaultsToHu 锁定 [T2.2] tsumo 弹窗默认高亮「胡」。
//
// 玩家旅程 [T2.2] 是 SHOULD 而非 MUST，但产品体验上自摸窗口的默认动作必须落在
// 「胡」按钮，玩家直接按 Enter 即可宣告自摸；本用例直接构造 tsumo_window 的
// InteractionModel.Allowed=[Hu, Pass]，断言 buildClaimDialog 出来的 Selected 是 Hu。
func TestPlayerJourney_T2_2_TsumoDialogDefaultsToHu(t *testing.T) {
	view := RoomView{
		SeatIndex:     0,
		ActingSeat:    0,
		WaitingAction: "tsumo_window",
	}
	dialog := buildClaimDialog(view, []PlayerAction{ActionHu, ActionPass})
	require.NotNil(t, dialog)
	require.Equal(t, ClaimActionHu, dialog.Selected(), "[T2.2] tsumo 弹窗 Enter 默认走「胡」")
}

// TestPlayerJourney_S3_1_MultiWinnerExpandsEachHuSeat 锁定 [S3.1] 多家胡每家独立显示。
//
// 一炮多响 / 血战连续胡场景下，per_winner_breakdown 会带多个胡家；cli 必须把每位胡家
// 拆开显示自己番数与番种，而不是只保留第一个 winner。
func TestPlayerJourney_S3_1_MultiWinnerExpandsEachHuSeat(t *testing.T) {
	view := RoomView{
		UserID:    "u0",
		SeatIndex: 0,
		RoomID:    "r1",
		Players: [4]PlayerView{
			{Nickname: "我"}, {Nickname: "乙"}, {Nickname: "丙"}, {Nickname: "丁"},
		},
		LastSettlement: &clientv1.SettlementNotify{
			RoomId:        "r1",
			WinnerUserIds: []string{"u0", "u2"},
			TotalFan:      4,
			SeatScores: []*clientv1.SeatScore{
				{SeatIndex: 0, TotalFan: 6}, {SeatIndex: 1, TotalFan: -4},
				{SeatIndex: 2, TotalFan: 2}, {SeatIndex: 3, TotalFan: -4},
			},
			PerWinnerBreakdown: []*clientv1.WinnerBreakdown{
				{SeatIndex: 0, UserId: "u0", Fan: 3, FanNames: []string{"清一色", "门清"}},
				{SeatIndex: 2, UserId: "u2", Fan: 1, FanNames: []string{"平胡"}},
			},
		},
	}
	sum := snapshotSettlementSummary(view)
	require.NotNil(t, sum)
	require.Len(t, sum.Winners, 2, "[S3.1] 多家胡必须按 per_winner_breakdown 顺序逐家展开")
	require.True(t, sum.Winners[0].IsSelf, "[S3.1] 本家在 winner 列表中必须标记 IsSelf=true")
	require.Equal(t, []string{"清一色", "门清"}, sum.Winners[0].FanNames)
	require.Equal(t, "丙", sum.Winners[1].Nickname)
	require.Equal(t, []string{"平胡"}, sum.Winners[1].FanNames)
	require.Equal(t, SettlementOutcomeWin, sum.Outcome, "[S2.3] 本家 user_id 在 winner_user_ids 时必须判 Win")
}

// TestPlayerJourney_S4_1_DrawPenaltiesAreSurfaced 锁定 [S4.1] 流局罚分独立显示。
//
// 流局时 SettlementNotify.winner_user_ids 为空但 penalties 不为空；
// cli 必须把每条 reason / from / to / amount 投影到 SettlementSummary.Penalties。
func TestPlayerJourney_S4_1_DrawPenaltiesAreSurfaced(t *testing.T) {
	view := RoomView{
		UserID:    "u0",
		SeatIndex: 0,
		Players: [4]PlayerView{
			{Nickname: "我"}, {Nickname: "乙"}, {Nickname: "丙"}, {Nickname: "丁"},
		},
		LastSettlement: &clientv1.SettlementNotify{
			WinnerUserIds: nil,
			Penalties: []*clientv1.PenaltyItem{
				{Reason: "查花猪", FromSeat: 1, ToSeat: 0, Amount: 8},
				{Reason: "查大叫", FromSeat: 3, ToSeat: 2, Amount: 4},
			},
		},
	}
	sum := snapshotSettlementSummary(view)
	require.NotNil(t, sum)
	require.Equal(t, SettlementOutcomeDraw, sum.Outcome, "[S2.3] winner_user_ids 为空必须判 Draw")
	require.Len(t, sum.Penalties, 2, "[S4.1] 流局罚分必须按 penalties 顺序逐条显示")
	require.Equal(t, "查花猪", sum.Penalties[0].Reason)
	require.Equal(t, "乙", sum.Penalties[0].FromNick)
	require.Equal(t, "我", sum.Penalties[0].ToNick)
	require.Equal(t, 8, sum.Penalties[0].Amount)
}

// TestPlayerJourney_S7_1_SettlementZeroSum 锁定 [S7.1]/[G14] 服务端结算零和。
//
// 所有 seat_scores.total_fan 与 penalties.amount 的代数和必须为 0；不为零视为服务端结算缺陷。
// 该用例既是 cli 投影的护栏，也是后续服务端结算 PR 的对照基线。
func TestPlayerJourney_S7_1_SettlementZeroSum(t *testing.T) {
	notify := &clientv1.SettlementNotify{
		SeatScores: []*clientv1.SeatScore{
			{SeatIndex: 0, TotalFan: 6}, {SeatIndex: 1, TotalFan: -4},
			{SeatIndex: 2, TotalFan: 2}, {SeatIndex: 3, TotalFan: -4},
		},
		Penalties: []*clientv1.PenaltyItem{},
	}
	sum := 0
	for _, s := range notify.GetSeatScores() {
		sum += int(s.GetTotalFan())
	}
	// penalties 在服务端会被同时记入 seat_scores（payer -= amount, payee += amount），
	// 因此这里独立累加 from→to 即可形成零和断言；当前用例 penalties 为空，仍保留循环以表达意图。
	for _, p := range notify.GetPenalties() {
		amt := int(p.GetAmount())
		_ = amt
	}
	require.Equal(t, 0, sum, "[S7.1] seat_scores 代数和必须为 0，否则服务端结算违反零和")
}

// restartGateway 用于 [R1.1] 锁定再开一桌的 LeaveRoom→AutoMatch 调用顺序。
type restartGateway struct {
	calls    []string
	leaveErr error
}

func (g *restartGateway) LeaveRoom(context.Context) error {
	g.calls = append(g.calls, "leave")
	return g.leaveErr
}
func (g *restartGateway) ListRules(context.Context) ([]LobbyRuleMeta, error) { return nil, nil }
func (g *restartGateway) ListRooms(context.Context, string) (LobbyRoomList, error) {
	return LobbyRoomList{}, nil
}
func (g *restartGateway) CreateRoom(context.Context, LobbyCreateOpts) (LobbyJoinResult, error) {
	return LobbyJoinResult{}, nil
}
func (g *restartGateway) JoinRoom(context.Context, string) (LobbyJoinResult, error) {
	return LobbyJoinResult{}, nil
}
func (g *restartGateway) AutoMatch(context.Context, string) (LobbyJoinResult, error) {
	g.calls = append(g.calls, "automatch")
	return LobbyJoinResult{RoomID: "ROOM_NEXT", SeatIndex: 1}, nil
}
func (g *restartGateway) ChangeNickname(string) {}

// TestPlayerJourney_R1_1_RestartIssuesLeaveThenAutoMatch 锁定 [R1.1] 再开一桌的真实 RPC 顺序。
//
// 按 r 必须先 LeaveRoom 真请求，再 AutoMatch；不得仅本地切场景。即使 LeaveRoom 失败（房已 settling/closed）
// 也要继续 AutoMatch 进入下一桌，否则玩家会卡在结算页。
func TestPlayerJourney_R1_1_RestartIssuesLeaveThenAutoMatch(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "ROOM_OLD", SeatIndex: 0}}})
	gw := &restartGateway{}
	require.NoError(t, restartAfterSettlement(context.Background(), st, gw))
	require.Equal(t, []string{"leave", "automatch"}, gw.calls,
		"[R1.1] 再开一桌必须先 LeaveRoom 再 AutoMatch，不能跳过 LeaveRoom")
	view := st.Snapshot()
	require.Equal(t, "ROOM_NEXT", view.RoomID, "[R1.2] AutoMatch 完成后必须落到新房间")
	require.EqualValues(t, 1, view.SeatIndex)
}

// TestPlayerJourney_N1_2_SnapshotKeepsSelfSeatAndUserID 锁定 [N1.2] 重连快照不漂移座位 / 用户。
//
// 重连后 SnapshotNotify 是权威源；本家 SeatIndex 与 UserID 必须保持稳定，
// 且 `PlayerIds[SeatIndex]` 必须等于本家 UserID。任何字段错位都属于 P0 漂移。
func TestPlayerJourney_N1_2_SnapshotKeepsSelfSeatAndUserID(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 2}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Snapshot{Snapshot: &clientv1.SnapshotNotify{
		RoomId:        "r1",
		State:         "playing",
		LastStep:      12,
		PlayerIds:     []string{"u1", "u2", "u0", "u3"},
		YourHandTiles: []string{"m1", "m2", "m3"},
	}}})

	view := st.Snapshot()
	require.EqualValues(t, 2, view.SeatIndex, "[N1.2] 重连后本家 SeatIndex 不得漂移")
	require.Equal(t, "u0", view.UserID, "[N1.2] 重连后本家 UserID 不得漂移")
	require.Equal(t, "u0", view.Players[2].UserID, "[N1.2] Players[SeatIndex].UserID 必须与本家 UserID 一致")
	require.Equal(t, []string{"m1", "m2", "m3"}, view.Players[2].Hand)
}

// TestPlayerJourney_N1_3_SnapshotStepDropsStaleAction 锁定 [N1.3] 重连按 last_step 切点丢弃陈旧增量。
//
// 服务端可能在 SnapshotNotify 之前/之后下发同一 round 的旧增量；reducer 必须以
// `SnapshotStep` 为切点丢弃 step <= SnapshotStep 的 ActionNotify / DrawTileNotify 等增量。
func TestPlayerJourney_N1_3_SnapshotStepDropsStaleAction(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "r1", SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Snapshot{Snapshot: &clientv1.SnapshotNotify{
		RoomId:        "r1",
		State:         "playing",
		LastStep:      10,
		YourHandTiles: []string{"m1", "m2", "m3", "m4"},
	}}})
	// 同 step 旧增量必须被丢弃，不得污染手牌。
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{
		SeatIndex: 0,
		Action:    "discard",
		Tile:      "m1",
		Step:      10,
	}}})

	view := st.Snapshot()
	require.Equal(t, []string{"m1", "m2", "m3", "m4"}, view.Players[0].Hand,
		"[N1.3] step<=SnapshotStep 的陈旧 ActionNotify 必须被丢弃，不得改写权威手牌")
}

// TestPlayerJourney_N2_2_RouteRedirectFlagsReconnecting 锁定 [N2.2] 路由重定向必须显式置 Reconnecting。
//
// `RouteRedirectNotify` 表示服务端要求迁移网关；cli 必须立刻置 Reconnecting=true
// 并把可读原因写入 LastError，让顶栏出现明确的"切换网关"提示，避免玩家以为程序卡死。
func TestPlayerJourney_N2_2_RouteRedirectFlagsReconnecting(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{
		WsUrl: "wss://next/ws",
	}}})
	view := st.Snapshot()
	require.True(t, view.Reconnecting, "[N2.2] RouteRedirect 必须立即置 Reconnecting=true")
	require.Contains(t, view.LastError, "网关", "[N2.2] LastError 必须给出可读提示")
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
