package handler

import (
	"context"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/internal/store/redis"
)

func TestLocalRoomGatewayNilErrors(t *testing.T) {
	t.Parallel()
	var g *LocalRoomGateway
	ctx := context.Background()
	_, err := g.Join(ctx, "r", "u")
	require.Error(t, err)
	_, err = g.Ready(ctx, "r", "u")
	require.Error(t, err)
	_, err = g.Leave(ctx, "r", "u")
	require.Error(t, err)
	_, err = g.Discard(ctx, "r", "u", "1m")
	require.Error(t, err)
	_, err = g.Pong(ctx, "r", "u")
	require.Error(t, err)
	_, err = g.Gang(ctx, "r", "u", "1m")
	require.Error(t, err)
	_, err = g.Hu(ctx, "r", "u")
	require.Error(t, err)
	_, err = g.ExchangeThree(ctx, "r", "u", nil, 0)
	require.Error(t, err)
	_, err = g.QueMen(ctx, "r", "u", 0)
	require.Error(t, err)
	_, err = g.Resume(ctx, "tok")
	require.Error(t, err)
}

func TestLocalRoomGatewayJoinSmoke(t *testing.T) {
	t.Parallel()
	svc := roomsvc.NewService(roomsvc.NewLobby())
	g := NewLocalRoomGateway(svc, session.NewHub(), nil)
	seat, err := g.Join(context.Background(), "local-room", "u-local")
	require.NoError(t, err)
	require.GreaterOrEqual(t, seat, 0)
}

func TestLocalRoomGatewayResumeRequiresSession(t *testing.T) {
	t.Parallel()
	svc := roomsvc.NewService(roomsvc.NewLobby())
	g := NewLocalRoomGateway(svc, session.NewHub(), nil)
	_, err := g.Resume(context.Background(), "any-token")
	require.Error(t, err)
}

func TestLocalRoomGatewayEnsureSubscriptionNoOp(t *testing.T) {
	t.Parallel()
	svc := roomsvc.NewService(roomsvc.NewLobby())
	g := NewLocalRoomGateway(svc, session.NewHub(), nil)
	require.NoError(t, g.EnsureRoomEventSubscription(context.Background(), "r", "c"))
}

func TestLocalRoomGatewayResumeWithRedisSession(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	cli := redis.NewClientFromUniversal(rcli)
	mgr := session.NewManager(cli)

	lobby := roomsvc.NewLobby()
	svc := roomsvc.NewService(lobby)
	hub := session.NewHub()
	gw := NewLocalRoomGateway(svc, hub, mgr)

	ctx := context.Background()
	tok, err := mgr.Issue(ctx, "resume-user", "127.0.0.1:9")
	require.NoError(t, err)

	_, err = gw.Join(ctx, "resume-room", "resume-user")
	require.NoError(t, err)
	for _, uid := range []string{"u2", "u3", "u4"} {
		_, err = gw.Join(ctx, "resume-room", uid)
		require.NoError(t, err)
	}
	for _, uid := range []string{"resume-user", "u2", "u3", "u4"} {
		_, err = gw.Ready(ctx, "resume-room", uid)
		require.NoError(t, err)
	}
	for _, uid := range []string{"resume-user", "u2", "u3", "u4"} {
		_, err = gw.ExchangeThree(ctx, "resume-room", uid, nil, 0)
		require.NoError(t, err)
	}
	for _, uid := range []string{"resume-user", "u2", "u3", "u4"} {
		_, err = gw.QueMen(ctx, "resume-room", uid, 0)
		require.NoError(t, err)
	}
	require.NoError(t, mgr.BindRoom(ctx, "resume-user", "resume-room"))

	res, err := gw.Resume(ctx, tok)
	require.NoError(t, err)
	require.Equal(t, "resume-user", res.UserID)
	require.Equal(t, "resume-room", res.RoomID)
	require.NotNil(t, res.Snapshot)
	require.Equal(t, "resume-room", res.Snapshot.GetRoomId())
	require.Len(t, res.Snapshot.GetQueSuitBySeat(), 4)
	require.NotEmpty(t, res.Snapshot.GetYourHandTiles())
	require.Len(t, res.Snapshot.GetDiscardsBySeat(), 4)
	require.Len(t, res.Snapshot.GetMeldsBySeat(), 4)
}

func TestLocalRoomGatewayReadyBroadcastSkippedWhenHubNil(t *testing.T) {
	t.Parallel()
	svc := roomsvc.NewService(roomsvc.NewLobby())
	gw := NewLocalRoomGateway(svc, nil, nil)
	ctx := context.Background()

	_, err := gw.Join(ctx, "ready-room", "p0")
	require.NoError(t, err)
	_, err = gw.Join(ctx, "ready-room", "p1")
	require.NoError(t, err)
	_, err = gw.Join(ctx, "ready-room", "p2")
	require.NoError(t, err)
	_, err = gw.Join(ctx, "ready-room", "p3")
	require.NoError(t, err)

	cb, err := gw.Ready(ctx, "ready-room", "p0")
	require.NoError(t, err)
	require.NotNil(t, cb)
	cb()
}

func TestLocalRoomGatewayResumeRoomMissing(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	cli := redis.NewClientFromUniversal(rcli)
	mgr := session.NewManager(cli)

	svc := roomsvc.NewService(roomsvc.NewLobby())
	gw := NewLocalRoomGateway(svc, session.NewHub(), mgr)
	ctx := context.Background()

	tok, err := mgr.Issue(ctx, "orphan", "127.0.0.1:1")
	require.NoError(t, err)
	require.NoError(t, mgr.BindRoom(ctx, "orphan", "never-created-room"))

	_, err = gw.Resume(ctx, tok)
	require.Error(t, err)
}
