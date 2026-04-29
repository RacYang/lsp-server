package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	clusterv1 "racoo.cn/lsp/api/gen/go/cluster/v1"
	lobbysvc "racoo.cn/lsp/internal/service/lobby"
)

func TestLobbyGRPCServerRoundTrip(t *testing.T) {
	t.Parallel()
	srv := newLobbyGRPCServer(lobbysvc.New(), nil, "")
	ctx := context.Background()

	created, err := srv.CreateRoom(ctx, &clusterv1.CreateRoomRequest{RoomId: "r1"})
	require.NoError(t, err)
	require.Equal(t, "room-local", created.GetRoomNodeId())

	joined, err := srv.JoinRoom(ctx, &clusterv1.JoinRoomRequest{RoomId: "r1", UserId: "u1"})
	require.NoError(t, err)
	require.EqualValues(t, 0, joined.GetSeatIndex())

	got, err := srv.GetRoom(ctx, &clusterv1.GetRoomRequest{RoomId: "r1"})
	require.NoError(t, err)
	require.Equal(t, "room-local", got.GetRoomNodeId())
}

func TestLobbyGRPCServerListCreateAndAutoMatch(t *testing.T) {
	t.Parallel()
	srv := newLobbyGRPCServer(lobbysvc.New(), nil, "room-test")
	ctx := context.Background()

	created, err := srv.CreateRoom(ctx, &clusterv1.CreateRoomRequest{
		RuleId:        "sichuan_xzdd",
		DisplayName:   "公开桌",
		CreatorUserId: "u1",
	})
	require.NoError(t, err)
	require.Empty(t, created.GetError())
	require.NotEmpty(t, created.GetRoomId())
	require.EqualValues(t, 0, created.GetSeatIndex())

	listed, err := srv.ListRooms(ctx, &clusterv1.ListRoomsRequest{PageSize: 20})
	require.NoError(t, err)
	require.Empty(t, listed.GetError())
	require.Len(t, listed.GetRooms(), 1)
	require.Equal(t, created.GetRoomId(), listed.GetRooms()[0].GetRoomId())

	matched, err := srv.AutoMatch(ctx, &clusterv1.AutoMatchRequest{RuleId: "sichuan_xzdd", UserId: "u2"})
	require.NoError(t, err)
	require.Empty(t, matched.GetError())
	require.Equal(t, created.GetRoomId(), matched.GetRoomId())
	require.EqualValues(t, 1, matched.GetSeatIndex())
}
