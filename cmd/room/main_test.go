package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/store/postgres"
)

func TestSplitEndpointsTrimsEmptyParts(t *testing.T) {
	t.Parallel()

	got := splitEndpoints(" http://etcd-0:2379, ,http://etcd-1:2379 ")

	require.Equal(t, []string{"http://etcd-0:2379", "http://etcd-1:2379"}, got)
}

func TestDeriveRecoveredState(t *testing.T) {
	t.Parallel()

	rows := []postgres.RoomEventRow{
		{Kind: "start_game"},
		{Kind: "settlement"},
	}

	require.Equal(t, "closed", deriveRecoveredState("ready", rows))
	require.Equal(t, "waiting", deriveRecoveredState("waiting", nil))
}
