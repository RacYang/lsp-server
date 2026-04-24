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

func TestNilRedisClientSessionErrors(t *testing.T) {
	t.Parallel()
	var cli *Client
	ctx := context.Background()
	require.Error(t, cli.PutSession(ctx, "u", SessionRecord{}, time.Minute))
	require.Error(t, cli.SaveSessionWithPlainToken(ctx, "u", "tok", SessionRecord{}, time.Minute))
	require.Error(t, cli.SaveSessionWithPlainToken(ctx, "u", "", SessionRecord{}, time.Minute))
	_, _, err := cli.GetSession(ctx, "u")
	require.Error(t, err)
	_, _, err = cli.ResolveUserIDByPlainToken(ctx, "x")
	require.Error(t, err)
	require.Error(t, cli.DeleteSession(ctx, "u"))
}

func TestSaveSessionResolveAndSnapMeta(t *testing.T) {
	t.Parallel()
	cli, srv := newTestClient(t)
	ctx := context.Background()

	require.NoError(t, cli.SaveSessionWithPlainToken(ctx, "user-1", "plain-tok", SessionRecord{GateNodeID: "g1"}, time.Minute))
	uid, ok, err := cli.ResolveUserIDByPlainToken(ctx, "plain-tok")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "user-1", uid)

	uid, ok, err = cli.ResolveUserIDByPlainToken(ctx, "")
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, uid)

	meta := RoomSnapMeta{Seq: 4, State: "playing", PlayerIDs: []string{"a", "b"}}
	require.NoError(t, cli.PutRoomSnapMeta(ctx, "snap-room", meta, time.Minute))
	got, ok, err := cli.GetRoomSnapMeta(ctx, "snap-room")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(4), got.Seq)
	require.Equal(t, "playing", got.State)

	require.Error(t, cli.PutRoomSnapMeta(ctx, "", RoomSnapMeta{Seq: 1}, time.Minute))

	require.NoError(t, srv.Set(RoomSnapshotMetaKey("bad-json"), "not-json"))
	_, _, err = cli.GetRoomSnapMeta(ctx, "bad-json")
	require.Error(t, err)
}

func TestSessionTokenVersionHelpers(t *testing.T) {
	t.Parallel()
	token := FormatSessionToken(3, "entropy")
	require.Equal(t, "v3.entropy", token)

	ver, ok := ParseSessionTokenVersion(token)
	require.True(t, ok)
	require.EqualValues(t, 3, ver)

	_, ok = ParseSessionTokenVersion("bad-token")
	require.False(t, ok)
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
