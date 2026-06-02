package reconnect

import (
	"context"
	"fmt"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
	redisstore "racoo.cn/lsp/internal/store/redis"
)

// stubRoomQueries 是测试桩，替代真实房间查询服务，避免依赖完整房间运行时。
type stubRoomQueries struct {
	snapshot   []string
	fsmState   string
	snapshotOK bool
	viewErr    error
}

func (s *stubRoomQueries) RoomSnapshot(_ string) ([]string, string, [4]bool, bool) {
	return s.snapshot, s.fsmState, [4]bool{}, s.snapshotOK
}
func (s *stubRoomQueries) RoundView(_ context.Context, _ string) (roomsvc.RoundView, bool, error) {
	return roomsvc.RoundView{}, false, s.viewErr
}
func (s *stubRoomQueries) RoundPersistSnapshot(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (s *stubRoomQueries) PlayerIDs(_ string) ([4]string, bool) { return [4]string{}, false }
func (s *stubRoomQueries) ActiveRoomCount() int                 { return 0 }
func (s *stubRoomQueries) RuleID() string                       { return "" }

func newTestManager(t *testing.T) *session.Manager {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	return session.NewManager(redisstore.NewClientFromUniversal(rcli))
}

func TestResumeNilService(t *testing.T) {
	t.Parallel()
	var s *Service
	_, err := s.Resume(context.Background(), "tok")
	require.Error(t, err)
}

func TestResumeNilSession(t *testing.T) {
	t.Parallel()
	s := New(&stubRoomQueries{snapshotOK: true}, nil)
	_, err := s.Resume(context.Background(), "tok")
	require.Error(t, err)
}

func TestResumeLobbySession(t *testing.T) {
	t.Parallel()
	mgr := newTestManager(t)
	ctx := context.Background()
	tok, err := mgr.Issue(ctx, "user-lobby")
	require.NoError(t, err)

	s := New(&stubRoomQueries{snapshotOK: true}, mgr)
	r, err := s.Resume(ctx, tok)
	require.NoError(t, err)
	require.Equal(t, "user-lobby", r.UserID)
	require.False(t, r.Resumed)
	require.Empty(t, r.RoomID)
}

func TestResumeRoomNotFound(t *testing.T) {
	t.Parallel()
	mgr := newTestManager(t)
	ctx := context.Background()
	tok, err := mgr.Issue(ctx, "orphan")
	require.NoError(t, err)
	require.NoError(t, mgr.BindRoom(ctx, "orphan", "ghost-room"))

	s := New(&stubRoomQueries{snapshotOK: false}, mgr)
	_, err = s.Resume(ctx, tok)
	require.Error(t, err)
}

func TestResumeRoundViewError(t *testing.T) {
	t.Parallel()
	mgr := newTestManager(t)
	ctx := context.Background()
	tok, err := mgr.Issue(ctx, "u1")
	require.NoError(t, err)
	require.NoError(t, mgr.BindRoom(ctx, "u1", "room-1"))

	stub := &stubRoomQueries{
		snapshotOK: true,
		fsmState:   "playing",
		snapshot:   []string{"u1", "u2", "u3", "u4"},
		viewErr:    fmt.Errorf("view error"),
	}
	s := New(stub, mgr)
	_, err = s.Resume(ctx, tok)
	require.Error(t, err)
}

func TestResumeSuccess(t *testing.T) {
	t.Parallel()
	mgr := newTestManager(t)
	ctx := context.Background()
	tok, err := mgr.Issue(ctx, "u1")
	require.NoError(t, err)
	require.NoError(t, mgr.BindRoom(ctx, "u1", "room-1"))

	stub := &stubRoomQueries{
		snapshotOK: true,
		fsmState:   "playing",
		snapshot:   []string{"u1", "u2", "u3", "u4"},
	}
	s := New(stub, mgr)
	r, err := s.Resume(ctx, tok)
	require.NoError(t, err)
	require.True(t, r.Resumed)
	require.Equal(t, "u1", r.UserID)
	require.Equal(t, "room-1", r.RoomID)
	require.Equal(t, "playing", r.State)
	require.Equal(t, stub.snapshot, r.Players, "Players 应由 RoomSnapshot 快照组装")
}

func TestResumeInvalidToken(t *testing.T) {
	t.Parallel()
	mgr := newTestManager(t)
	s := New(&stubRoomQueries{snapshotOK: true}, mgr)
	_, err := s.Resume(context.Background(), "bad-token")
	require.Error(t, err)
}
