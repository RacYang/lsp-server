package cluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEtcdClaimAndResolveRoomOwner(t *testing.T) {
	t.Parallel()
	_, cli := startEmbeddedEtcd(t)
	r := NewEtcdRouter(cli, "/lsp-test")

	err := r.ClaimRoom(context.Background(), " room-1 ", "room-node-a", 0)
	require.NoError(t, err)

	nodeID, ok, err := r.ResolveRoomOwner(context.Background(), "room-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "room-node-a", nodeID)

	err = r.ClaimRoom(context.Background(), "room-1", "room-node-b", 0)
	require.Error(t, err)
}

func TestEtcdListRoomsByOwner(t *testing.T) {
	t.Parallel()
	_, cli := startEmbeddedEtcd(t)
	r := NewEtcdRouter(cli, "/lsp-test")

	require.NoError(t, r.ClaimRoom(context.Background(), "room-a", "room-node-a", 0))
	require.NoError(t, r.ClaimRoom(context.Background(), "room-b", "room-node-a", 0))
	require.NoError(t, r.ClaimRoom(context.Background(), "room-c", "room-node-b", 0))

	roomIDs, err := r.ListRoomsByOwner(context.Background(), "room-node-a")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"room-a", "room-b"}, roomIDs)
}
