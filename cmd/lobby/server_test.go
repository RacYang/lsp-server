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
