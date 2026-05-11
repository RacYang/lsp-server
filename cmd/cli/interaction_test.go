package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func TestDeriveInteractionModelPhases(t *testing.T) {
	tests := []struct {
		name string
		view RoomView
		want TablePhase
	}{
		{name: "login", view: RoomView{Phase: phaseLogin}, want: PhaseLogin},
		{name: "lobby", view: RoomView{Phase: phaseLobby}, want: PhaseLobby},
		{name: "exchange", view: RoomView{Phase: phaseTable, SeatIndex: 0, ActingSeat: 0, WaitingAction: "exchange_three"}, want: PhaseExchange},
		{name: "que", view: RoomView{Phase: phaseTable, SeatIndex: 0, ActingSeat: 0, WaitingAction: "que_men"}, want: PhaseQueMen},
		{name: "draw", view: RoomView{Phase: phaseTable, SeatIndex: 0, ActingSeat: 1, RoundPhase: clientv1.Phase_PHASE_DRAW, WaitingAction: "none"}, want: PhaseOtherTurn},
		{name: "discard", view: RoomView{Phase: phaseTable, SeatIndex: 0, ActingSeat: 0, WaitingAction: "discard"}, want: PhaseDiscard},
		{name: "claim", view: RoomView{Phase: phaseTable, SeatIndex: 1, ActingSeat: 1, WaitingAction: "claim_window", ClaimCandidates: map[int32][]string{1: {"hu", "pass"}}}, want: PhaseClaim},
		{name: "tsumo", view: RoomView{Phase: phaseTable, SeatIndex: 0, ActingSeat: 0, WaitingAction: "tsumo_window"}, want: PhaseTsumo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, DeriveInteractionModel(tt.view).Phase)
		})
	}
}

// TestExchangeAndQueMenAllowConcurrentSeats 防回归：川麻血战的换三张 / 定缺都是
// 4 家并发（不轮转），任何 SeatIndex 合法的玩家都应当能拿到对应 Allowed 动作；
// 之前要求 SeatIndex == ActingSeat 会导致非 dealer 玩家在这两阶段卡死。
func TestExchangeAndQueMenAllowConcurrentSeats(t *testing.T) {
	cases := []struct {
		action string
		want   PlayerAction
		phase  TablePhase
	}{
		{"exchange_three", ActionExchangeThree, PhaseExchange},
		{"que_men", ActionQueMen, PhaseQueMen},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			view := RoomView{
				Phase:         phaseTable,
				SeatIndex:     2,
				ActingSeat:    0,
				WaitingAction: tc.action,
			}
			model := DeriveInteractionModel(view)
			require.Contains(t, model.Allowed, tc.want)
			require.NotContains(t, model.Hint, "等待")
			require.Equal(t, tc.phase, DeriveTableUXModel(view, nil, nowForTest()).Phase)
		})
	}
}

func TestTableUXModelUsesRelativeSeatLabels(t *testing.T) {
	view := RoomView{
		Phase:         phaseTable,
		RoomState:     "playing",
		SeatIndex:     1,
		ActingSeat:    0,
		WaitingAction: "discard",
	}
	view.Players[0].Nickname = "BOT-2"

	ux := DeriveTableUXModel(view, nil, nowForTest())

	require.Equal(t, "上家", ux.Seats[0].RelativeLabel)
	require.Contains(t, ux.PrimaryPrompt, "上家")
	require.NotContains(t, ux.PrimaryPrompt, "BOT")
}

func TestDrawPhaseShowsNextSeatInsteadOfWaitingStart(t *testing.T) {
	view := RoomView{
		Phase:         phaseTable,
		RoomState:     "playing",
		SeatIndex:     1,
		ActingSeat:    2,
		RoundPhase:    clientv1.Phase_PHASE_DRAW,
		WaitingAction: "none",
	}

	ux := DeriveTableUXModel(view, nil, nowForTest())

	require.Equal(t, PhaseOtherTurn, ux.Phase)
	require.Contains(t, ux.PrimaryPrompt, "等待下家摸牌")
	require.NotContains(t, ux.PrimaryPrompt, "等待开")
}

func TestDeriveInteractionModelFiltersNonTargetClaim(t *testing.T) {
	view := RoomView{
		Phase:           phaseTable,
		SeatIndex:       0,
		ActingSeat:      2,
		WaitingAction:   "claim_window",
		ClaimCandidates: map[int32][]string{2: {"gang", "pass"}},
	}
	model := DeriveInteractionModel(view)
	require.Equal(t, PhaseClaim, model.Phase)
	require.Empty(t, model.Allowed)
	require.Nil(t, model.Claim)
	require.Contains(t, model.Hint, "等待")
}

func TestClaimUXShowsCorrectPassShortcut(t *testing.T) {
	view := RoomView{
		Phase:           phaseTable,
		RoomState:       "playing",
		SeatIndex:       0,
		ActingSeat:      2,
		WaitingAction:   "claim_window",
		PendingTile:     "p5",
		ClaimCandidates: map[int32][]string{0: {"pong", "pass"}},
	}

	ux := DeriveTableUXModel(view, nil, nowForTest())

	require.Contains(t, ux.PrimaryPrompt, "碰")
	require.Contains(t, ux.PrimaryPrompt, "过")
	require.Contains(t, ux.KeyHint, "p 碰")
	require.Contains(t, ux.KeyHint, "n 过")
	require.NotContains(t, ux.KeyHint, "P 跳过")
}

func TestDisabledReasonExplainsNotYourTurn(t *testing.T) {
	view := RoomView{
		Phase:         phaseTable,
		RoomState:     "playing",
		SeatIndex:     0,
		ActingSeat:    3,
		WaitingAction: "discard",
	}
	view.Players[3].Nickname = "机器人"

	ux := DeriveTableUXModel(view, nil, nowForTest())

	require.Contains(t, ux.DisabledReason, "还没轮到你")
	require.Contains(t, ux.DisabledReason, "上家")
	require.Contains(t, ux.KeyHint, ux.DisabledReason)
}

func nowForTest() time.Time {
	return time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)
}
