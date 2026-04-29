package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func TestApplyLobbyListMatchAndCreate(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0", SessionToken: "tok"}}})
	view := st.Snapshot()
	require.Equal(t, phaseLobby, view.Phase)

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_ListRoomsResp{ListRoomsResp: &clientv1.ListRoomsResponse{
		Rooms: []*clientv1.RoomMeta{{
			RoomId:      "ROOM01",
			RuleId:      "sichuan_xzdd",
			DisplayName: "公开桌",
			SeatCount:   2,
			MaxSeats:    4,
			Stage:       "waiting",
		}},
		NextPageToken: "next",
	}}})
	view = st.Snapshot()
	require.Equal(t, phaseLobby, view.Phase)
	require.Len(t, view.RoomList, 1)
	require.Equal(t, "ROOM01", view.RoomList[0].GetRoomId())
	require.Equal(t, "next", view.NextRoomPage)

	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "ROOM01", SeatIndex: 2}}})
	view = st.Snapshot()
	require.Equal(t, phaseTable, view.Phase)
	require.Equal(t, "ROOM01", view.RoomID)
	require.EqualValues(t, 2, view.SeatIndex)
	require.Equal(t, "u0", view.Players[2].UserID)

	st = NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u1"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_CreateRoomResp{CreateRoomResp: &clientv1.CreateRoomResponse{RoomId: "ROOM02", SeatIndex: 0}}})
	view = st.Snapshot()
	require.Equal(t, phaseTable, view.Phase)
	require.Equal(t, "ROOM02", view.RoomID)
	require.EqualValues(t, 0, view.SeatIndex)
}

func TestApplyLobbyErrorsStayInLobby(t *testing.T) {
	st := NewAppState("我")
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	st.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_ROOM_FULL,
		ErrorMessage: "房间已满",
	}}})
	view := st.Snapshot()
	require.Equal(t, phaseLobby, view.Phase)
	require.Equal(t, "房间已满", view.LastError)
}
