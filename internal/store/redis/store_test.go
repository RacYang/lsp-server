package redis

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	cli := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = cli.Close() })
	return NewClientFromUniversal(cli), srv
}

func TestKeys(t *testing.T) {
	t.Parallel()
	require.Equal(t, "lsp:session:u1", SessionKey("u1"))
	require.Equal(t, "lsp:idem:join:x", IdempotencyKey("join", "x"))
	require.Equal(t, "lsp:route:room:r1", RoomRouteCacheKey("r1"))
	require.Equal(t, "lsp:room:snapmeta:r1", RoomSnapshotMetaKey("r1"))
}

func TestSessionCRUD(t *testing.T) {
	t.Parallel()
	cli, _ := newTestClient(t)
	ctx := context.Background()

	err := cli.PutSession(ctx, "u1", SessionRecord{GateNodeID: "gate-a", AdvertiseAddr: "127.0.0.1:1"}, time.Minute)
	require.NoError(t, err)

	rec, ok, err := cli.GetSession(ctx, "u1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "gate-a", rec.GateNodeID)

	require.NoError(t, cli.DeleteSession(ctx, "u1"))
	_, ok, err = cli.GetSession(ctx, "u1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestRouteCacheCRUD(t *testing.T) {
	t.Parallel()
	cli, _ := newTestClient(t)
	ctx := context.Background()

	err := cli.PutRoomRouteCache(ctx, "room-1", RouteRecord{RoomNodeID: "room-node-a", Version: 7}, time.Minute)
	require.NoError(t, err)

	rec, ok, err := cli.GetRoomRouteCache(ctx, "room-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "room-node-a", rec.RoomNodeID)
	require.EqualValues(t, 7, rec.Version)

	require.NoError(t, cli.DeleteRoomRouteCache(ctx, "room-1"))
	_, ok, err = cli.GetRoomRouteCache(ctx, "room-1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestIdempotencyAbsent(t *testing.T) {
	t.Parallel()
	cli, _ := newTestClient(t)
	ctx := context.Background()

	created, err := cli.PutIdempotencyAbsent(ctx, "join_room", "k1", IdempotencyRecord{Result: "ok"}, time.Minute)
	require.NoError(t, err)
	require.True(t, created)

	created, err = cli.PutIdempotencyAbsent(ctx, "join_room", "k1", IdempotencyRecord{Result: "dup"}, time.Minute)
	require.NoError(t, err)
	require.False(t, created)

	rec, ok, err := cli.GetIdempotency(ctx, "join_room", "k1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "ok", rec.Result)
}

func TestTTLApplied(t *testing.T) {
	t.Parallel()
	cli, srv := newTestClient(t)
	ctx := context.Background()

	require.NoError(t, cli.PutSession(ctx, "u-expire", SessionRecord{GateNodeID: "g"}, time.Second))
	srv.FastForward(2 * time.Second)
	_, ok, err := cli.GetSession(ctx, "u-expire")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestNewClientEmptyAddr(t *testing.T) {
	t.Parallel()
	cli, err := NewClient("")
	require.Nil(t, cli)
	require.Error(t, err)
}

func TestPingAndClose(t *testing.T) {
	t.Parallel()
	cli, _ := newTestClient(t)
	require.NoError(t, cli.Ping(context.Background()))
	require.NoError(t, cli.Close())
}

func TestMissesAndDefaultTTL(t *testing.T) {
	t.Parallel()
	cli, srv := newTestClient(t)
	ctx := context.Background()

	_, ok, err := cli.GetRoomRouteCache(ctx, "missing-room")
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = cli.GetIdempotency(ctx, "join", "missing")
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, cli.PutRoomRouteCache(ctx, "room-default", RouteRecord{RoomNodeID: "room-a"}, 0))
	ttl := srv.TTL(RoomRouteCacheKey("room-default"))
	require.Positive(t, ttl)
}
