package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func TestBuildTableFrontendModelUsesHandCountsForOpponents(t *testing.T) {
	for self := int32(0); self < 4; self++ {
		view := RoomView{
			Phase:       phaseTable,
			SeatIndex:   self,
			RoomState:   "playing",
			ActingSeat:  self,
			ActingSeats: []int32{self},
		}
		for seat := int32(0); seat < 4; seat++ {
			view.Players[seat] = PlayerView{
				UserID:   "u",
				Nickname: "p",
				HandCnt:  13 + int(seat%2),
			}
		}
		view.Players[self].Hand = []string{"m1", "m2", "m3"}
		view.Players[self].HandCnt = len(view.Players[self].Hand)

		model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
		require.Equal(t, self, model.Seats[0].AbsSeat)
		require.Equal(t, view.Players[(self+1)%4].HandCnt, model.Seats[1].HandCount)
		require.Equal(t, view.Players[(self+2)%4].HandCnt, model.Seats[2].HandCount)
		require.Equal(t, view.Players[(self+3)%4].HandCnt, model.Seats[3].HandCount)
	}
}

func TestBuildTableFrontendModelClaimWindowOnlyForCandidate(t *testing.T) {
	view := RoomView{
		Phase:         phaseTable,
		SeatIndex:     0,
		RoomState:     "playing",
		ActingSeat:    2,
		ActingSeats:   []int32{2},
		LastStep:      8,
		RoundPhase:    clientv1.Phase_PHASE_CLAIM,
		WaitingAction: "claim_window",
		PendingTile:   "p3",
		ClaimCandidates: map[int32][]string{
			2: {"pong", "pass"},
		},
	}
	require.Nil(t, BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0)).ActionWindow)

	view.SeatIndex = 2
	model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
	require.NotNil(t, model.ActionWindow)
	require.Equal(t, []PlayerAction{ActionPong, ActionPass}, model.AllowedActions)
	require.Equal(t, ActionWindowClaim, model.ActionWindow.Kind)
}

func TestBuildTableFrontendModelClaimWindowSupportsChi(t *testing.T) {
	view := RoomView{
		Phase:         phaseTable,
		SeatIndex:     1,
		RoomState:     "playing",
		ActingSeat:    1,
		ActingSeats:   []int32{1},
		LastStep:      9,
		RoundPhase:    clientv1.Phase_PHASE_CLAIM,
		WaitingAction: "claim_window",
		PendingTile:   "m2",
		ClaimCandidates: map[int32][]string{
			1: {"chi", "pass"},
		},
	}
	model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
	require.NotNil(t, model.ActionWindow)
	require.Equal(t, []PlayerAction{ActionChi, ActionPass}, model.AllowedActions)
	require.Equal(t, []ClaimAction{ClaimActionChow, ClaimActionPass}, model.ActionWindow.Actions)
	require.Contains(t, model.KeyHint, "c 吃")
}

func TestBuildTableFrontendModelTsumoWindow(t *testing.T) {
	view := RoomView{
		Phase:            phaseTable,
		SeatIndex:        1,
		RoomState:        "playing",
		ActingSeat:       1,
		ActingSeats:      []int32{1},
		LastStep:         12,
		RoundPhase:       clientv1.Phase_PHASE_TSUMO,
		WaitingAction:    "tsumo_window",
		PendingTile:      "m9",
		AvailableActions: []string{"hu", "pass"},
	}
	model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
	require.NotNil(t, model.ActionWindow)
	require.Equal(t, ActionWindowTsumo, model.ActionWindow.Kind)
	require.Equal(t, []PlayerAction{ActionHu, ActionPass}, model.AllowedActions)
	require.Equal(t, ClaimActionHu, model.ActionWindow.Actions[0])
}

func TestBuildTableFrontendModelHuedSelfCannotDiscard(t *testing.T) {
	view := RoomView{
		Phase:            phaseTable,
		SeatIndex:        0,
		RoomState:        "playing",
		ActingSeat:       0,
		ActingSeats:      []int32{0},
		RoundPhase:       clientv1.Phase_PHASE_DISCARD,
		WaitingAction:    "discard",
		AvailableActions: []string{"discard"},
	}
	view.Players[0] = PlayerView{
		Nickname: "self",
		Hand:     []string{"m1", "m2"},
		HandCnt:  2,
		Hued:     true,
	}

	model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
	require.Empty(t, model.AllowedActions)
	require.Equal(t, "你已胡牌，等待本局结束", model.DisabledReason)
	require.Equal(t, "你已胡牌，等待本局结束", model.Prompt)
	require.Equal(t, CursorModeNone, DeriveCursorMode(view))
}
