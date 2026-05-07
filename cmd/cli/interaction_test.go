package main

import (
	"testing"

	"github.com/stretchr/testify/require"
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
	}{
		{"exchange_three", ActionExchangeThree},
		{"que_men", ActionQueMen},
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
		})
	}
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
