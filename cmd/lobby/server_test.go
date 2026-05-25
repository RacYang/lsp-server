package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	lobbysvc "racoo.cn/lsp/internal/service/lobby"
)

func TestLobbyGRPCServerRoundTrip(t *testing.T) {
	t.Parallel()
	srv := newLobbyGRPCServer(lobbysvc.New(), nil, "")
	ctx := context.Background()

	created, err := srv.CreateRoom(ctx, &svcv1.CreateRoomRequest{RoomId: "r1"})
	require.NoError(t, err)
	require.Equal(t, "room-local", created.GetRoomNodeId())

	joined, err := srv.JoinRoom(ctx, &svcv1.JoinRoomRequest{RoomId: "r1", UserId: "u1"})
	require.NoError(t, err)
	require.EqualValues(t, 0, joined.GetSeatIndex())

	got, err := srv.GetRoom(ctx, &svcv1.GetRoomRequest{RoomId: "r1"})
	require.NoError(t, err)
	require.Equal(t, "room-local", got.GetRoomNodeId())
}

func TestLobbyGRPCServerListCreateAndAutoMatch(t *testing.T) {
	t.Parallel()
	srv := newLobbyGRPCServer(lobbysvc.New(), nil, "room-test")
	ctx := context.Background()

	created, err := srv.CreateRoom(ctx, &svcv1.CreateRoomRequest{
		RuleId:        "sichuan_xuezhandaodi_huansanzhang",
		DisplayName:   "公开桌",
		CreatorUserId: "u1",
	})
	require.NoError(t, err)
	require.Empty(t, created.GetError())
	require.NotEmpty(t, created.GetRoomId())
	require.EqualValues(t, 0, created.GetSeatIndex())

	listed, err := srv.ListRooms(ctx, &svcv1.ListRoomsRequest{PageSize: 20})
	require.NoError(t, err)
	require.Empty(t, listed.GetError())
	require.Len(t, listed.GetRooms(), 1)
	require.Equal(t, created.GetRoomId(), listed.GetRooms()[0].GetRoomId())

	matched, err := srv.AutoMatch(ctx, &svcv1.AutoMatchRequest{RuleId: "sichuan_xuezhandaodi_huansanzhang", UserId: "u2"})
	require.NoError(t, err)
	require.Empty(t, matched.GetError())
	require.Equal(t, created.GetRoomId(), matched.GetRoomId())
	require.EqualValues(t, 1, matched.GetSeatIndex())
}
