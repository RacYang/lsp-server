package localadapter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
)

// 验证玩家离线超时后自动触发投降（座位变空）。
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
	players, _, _, ok := rooms.RoomSnapshot("room-offline")
	require.True(t, ok)
	require.NotEmpty(t, players)
	require.Empty(t, players[0])
}

// 验证取消离线投降计时器的守卫分支：nil 网关与空 ID 均不 panic。
func TestLocalGatewayCancelOfflineSurrenderNilSafe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var g *LocalRoomGateway
	require.NoError(t, g.CancelOfflineSurrender(ctx, "room", "user"))

	rooms := roomsvc.NewService(roomsvc.NewRoomRegistry())
	hub := session.NewHub()
	g2 := NewLocalRoomGateway(rooms, hub, nil)
	require.NoError(t, g2.CancelOfflineSurrender(ctx, "", ""))
}

// 验证广播通知守卫分支：nil 网关、空房间 ID、空通知列表均不 panic。
func TestLocalGatewayBroadcastNotificationsNilSafe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var g *LocalRoomGateway
	// nil gateway 不 panic
	g.BroadcastNotifications(ctx, "room", nil)

	rooms := roomsvc.NewService(roomsvc.NewRoomRegistry())
	hub := session.NewHub()
	g2 := NewLocalRoomGateway(rooms, hub, nil)
	// 空房间 ID 不 panic
	g2.BroadcastNotifications(ctx, "", nil)
	// 空通知列表不 panic
	g2.BroadcastNotifications(ctx, "room", nil)
}

// 验证重连时显式取消投降计时器后玩家不被踢出；
// 新架构下计时器由 Actor 持有，取消必须显式发信号，与 hub 注册状态无关。
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

	// 重连：先取消 Actor 内的投降计时器，再注册到 hub。
	require.NoError(t, gateway.CancelOfflineSurrender(ctx, "room-reconnect", "user-1"))
	hub.Register("user-1", "room-reconnect", nil)

	time.Sleep(80 * time.Millisecond)
	players, _, _, ok := rooms.RoomSnapshot("room-reconnect")
	require.True(t, ok)
	require.NotEmpty(t, players)
	require.Equal(t, "user-1", players[0])
}
