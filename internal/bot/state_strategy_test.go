package bot

import (
	"context"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func TestBotStateAppliesVisibleEventsAndHidesOpponentHands(t *testing.T) {
	st := NewState()
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_JoinRoomResp{JoinRoomResp: &clientv1.JoinRoomResponse{SeatIndex: 0}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{
		SeatIndex: 0,
		Tiles:     []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "m5", "m5", "p7", "p8"},
	}}})
	st.RememberExchange([]string{"m1", "m2", "m3"})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_OpeningDone{OpeningDone: &clientv1.OpeningDoneNotify{
		Action:    "exchange_three",
		Kind:      "exchange_done",
		SeatTiles: []*clientv1.OpeningSeatTiles{{Key: "received", Seats: []*clientv1.SeatTiles{{SeatIndex: 0, Tiles: []string{"s7", "s8", "s9"}}}}},
	}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_OpeningDone{OpeningDone: &clientv1.OpeningDoneNotify{
		Action:   "que_men",
		Kind:     "missing_suit_done",
		SeatInts: []*clientv1.OpeningSeatInts{{Key: "que_suit", Values: []int32{0, 1, 2, 0}}},
	}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{SeatIndex: 1, Tile: "p9"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{SeatIndex: 1, Action: "discard", Tile: "p9"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{SeatIndex: 0, Action: "hu_choice", Tile: "p9"}}})

	view := st.Snapshot()
	require.Equal(t, "u0", view.UserID)
	require.Equal(t, int32(0), view.SeatIndex)
	require.NotContains(t, view.HandTiles, "m1")
	require.Contains(t, view.HandTiles, "s7")
	require.Equal(t, []string{"p9"}, view.DiscardsBySeat[1])
	require.Equal(t, []string{"p9"}, view.DrawnBySeat[1])
	require.Equal(t, "claim_window", view.WaitingAction)
	require.Equal(t, []string{"hu", "pass"}, view.AvailableAction)

	for seat := 1; seat < 4; seat++ {
		require.Empty(t, view.MeldsBySeat[seat])
	}
}

func TestBotStateSnapshotClosed(t *testing.T) {
	st := NewState()
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Snapshot{Snapshot: &clientv1.SnapshotNotify{
		RoomId:        "r1",
		State:         "closed",
		ActingSeat:    2,
		WaitingAction: "discard",
		YourHandTiles: []string{"m1", "m2"},
	}}})
	view := st.Snapshot()
	require.True(t, view.Closed)
	require.Equal(t, []string{"m1", "m2"}, view.HandTiles)
}

func TestStaleLoginSession(t *testing.T) {
	require.True(t, staleLoginSession(&clientv1.LoginResponse{
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_RECONNECTING,
		ErrorMessage: "快照房间失败: room not found",
	}))
	require.True(t, staleLoginSession(&clientv1.LoginResponse{
		ErrorCode: clientv1.ErrorCode_ERROR_CODE_ROOM_NOT_FOUND,
	}))
	require.False(t, staleLoginSession(&clientv1.LoginResponse{
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_RECONNECTING,
		ErrorMessage: "temporary reconnecting",
	}))
}

func TestShouldDecideWaitsForExchangeHand(t *testing.T) {
	view := BotView{
		SeatIndex:       1,
		WaitingAction:   "exchange_three",
		AvailableAction: []string{"exchange_three"},
	}
	require.False(t, shouldDecide(view))

	view.HandTiles = []string{"m1", "m2", "m3"}
	require.True(t, shouldDecide(view))
}

func TestRuleStrategyProducesAllowedActions(t *testing.T) {
	strategy := NewRuleStrategy(RuleStrategyConfig{
		Difficulty: DifficultyNormal,
		Rand:       rand.New(rand.NewSource(1)), //nolint:gosec // 固定种子用于可复现策略测试。
	})
	view := BotView{
		SeatIndex:       0,
		WaitingAction:   "discard",
		ActingSeat:      0,
		AvailableAction: []string{"discard"},
		HandTiles:       []string{"m1", "m2", "m3", "p1", "p2", "p3", "s1", "s2", "s3", "m5", "m5", "p7", "p8", "p9"},
		DiscardsBySeat:  make([][]string, 4),
		MeldsBySeat:     make([][]string, 4),
		DrawnBySeat:     make([][]string, 4),
		ClaimCandidates: map[int32][]string{},
		QueBySeat:       [4]int32{0, 1, 2, 0},
	}
	action, err := strategy.Decide(context.Background(), view)
	require.NoError(t, err)
	require.Equal(t, ActionDiscard, action.Kind)
	require.Contains(t, view.HandTiles, action.Tile)

	view.WaitingAction = "que_men"
	action, err = strategy.Decide(context.Background(), view)
	require.NoError(t, err)
	require.Equal(t, ActionQueMen, action.Kind)
	require.GreaterOrEqual(t, action.Suit, int32(0))
	require.LessOrEqual(t, action.Suit, int32(2))

	view.WaitingAction = "claim_window"
	view.AvailableAction = []string{"pong", "pass"}
	view.PendingTile = "p7"
	action, err = strategy.Decide(context.Background(), view)
	require.NoError(t, err)
	require.Contains(t, []ActionKind{ActionPong, ActionPass}, action.Kind)
}
