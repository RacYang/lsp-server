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
