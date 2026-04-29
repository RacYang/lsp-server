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
		State:            "discard",
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

func TestApplyResponsesAndSettlement(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_ReadyResp{ReadyResp: &clientv1.ReadyResponse{}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_PongResp{PongResp: &clientv1.PongResponse{ErrorCode: clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, ErrorMessage: "不能碰"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Settlement{Settlement: &clientv1.SettlementNotify{RoomId: "r", DetailText: "结算"}}})
	view := st.Snapshot()
	require.Equal(t, stageSettlement, view.Stage)
	require.Equal(t, "不能碰", view.LastError)
	require.NotNil(t, view.LastSettlement)
}
