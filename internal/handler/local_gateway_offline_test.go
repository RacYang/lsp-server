package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
)

func TestLocalGatewayOfflineThenSurrender(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rooms := roomsvc.NewService(roomsvc.NewRoomRegistry())
	hub := session.NewHub()
	gateway := NewLocalRoomGateway(rooms, hub, nil)
	gateway.SetOfflineSurrenderAfter(10 * time.Millisecond)

	_, err := gateway.Join(ctx, "room-offline", "user-1")
	require.NoError(t, err)

	require.NoError(t, gateway.MarkSeatOffline(ctx, "room-offline", "user-1"))
	time.Sleep(80 * time.Millisecond)
	players, _, ok := rooms.RoomSnapshot("room-offline")
	require.True(t, ok)
	require.NotEmpty(t, players)
	require.Empty(t, players[0])
}

func TestLocalGatewayReconnectBeforeSurrender(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rooms := roomsvc.NewService(roomsvc.NewRoomRegistry())
	hub := session.NewHub()
	gateway := NewLocalRoomGateway(rooms, hub, nil)
	gateway.SetOfflineSurrenderAfter(30 * time.Millisecond)

	_, err := gateway.Join(ctx, "room-reconnect", "user-1")
	require.NoError(t, err)
	require.NoError(t, gateway.MarkSeatOffline(ctx, "room-reconnect", "user-1"))

	hub.Register("user-1", "room-reconnect", nil)
	time.Sleep(80 * time.Millisecond)
	players, _, ok := rooms.RoomSnapshot("room-reconnect")
	require.True(t, ok)
	require.NotEmpty(t, players)
	require.Equal(t, "user-1", players[0])
}
