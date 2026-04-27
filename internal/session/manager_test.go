package session_test

import (
	"context"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/internal/store/redis"
)

func TestManagerIssueResumeBind(t *testing.T) {
	t.Parallel()
	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)
	cli := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = cli.Close() })
	store := redis.NewClientFromUniversal(cli)
	mgr := session.NewManager(store)
	ctx := context.Background()

	const uid = "user-aaa"
	tok, err := mgr.Issue(ctx, uid, ":18080")
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	require.NoError(t, mgr.BindRoom(ctx, uid, "room-xyz"))
	require.NoError(t, mgr.UpdateCursor(ctx, uid, "room-xyz:7"))

	got, rec, err := mgr.Resume(ctx, tok)
	require.NoError(t, err)
	require.Equal(t, uid, got)
	require.Equal(t, "room-xyz", rec.RoomID)
	require.Equal(t, "room-xyz:7", rec.LastCursor)
	require.EqualValues(t, 1, rec.SessionVer)

	require.NoError(t, mgr.UnbindRoom(ctx, uid))
	_, rec, err = mgr.Resume(ctx, tok)
	require.NoError(t, err)
	require.Empty(t, rec.RoomID)
}

func TestManagerResumeRejectsSessionVersionMismatch(t *testing.T) {
	t.Parallel()
	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)
	cli := goredis.NewClient(&goredis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = cli.Close() })
	store := redis.NewClientFromUniversal(cli)
	mgr := session.NewManager(store)
	ctx := context.Background()

	const uid = "user-bbb"
	tok, err := mgr.Issue(ctx, uid, ":18080")
	require.NoError(t, err)

	srec, ok, err := store.GetSession(ctx, uid)
	require.NoError(t, err)
	require.True(t, ok)
	srec.SessionVer = 2
	require.NoError(t, store.PutSession(ctx, uid, srec, 0))

	_, _, err = mgr.Resume(ctx, tok)
	require.Error(t, err)
	require.Contains(t, err.Error(), "会话版本校验失败")
}
