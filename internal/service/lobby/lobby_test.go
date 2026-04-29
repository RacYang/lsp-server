package lobby

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()
	s := New()
	require.NotNil(t, s)
}

func TestCreateJoinGetRoom(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()

	created, err := s.CreateRoom(ctx, "r1")
	require.NoError(t, err)
	require.Equal(t, "room-local", created)

	joined, err := s.JoinRoom(ctx, "r1", "u1")
	require.NoError(t, err)
	require.EqualValues(t, 0, joined)

	got, err := s.GetRoom(ctx, "r1")
	require.NoError(t, err)
	require.Equal(t, "room-local", got)
}

func TestListRoomsFiltersPrivateAndFullRooms(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()

	publicID, _, err := s.CreateRoomWithMeta(ctx, "sichuan_xzdd", "公开桌", false, "u1")
	require.NoError(t, err)
	privateID, _, err := s.CreateRoomWithMeta(ctx, "sichuan_xzdd", "私密桌", true, "u2")
	require.NoError(t, err)
	for _, userID := range []string{"u3", "u4", "u5"} {
		_, err = s.JoinRoom(ctx, publicID, userID)
		require.NoError(t, err)
	}
	_, err = s.JoinRoom(ctx, publicID, "u6")
	require.ErrorIs(t, err, ErrRoomFull)

	rooms, next, err := s.ListRooms(ctx, 20, "")
	require.NoError(t, err)
	require.Empty(t, next)
	require.Empty(t, rooms)

	_, err = s.JoinRoom(ctx, privateID, "u7")
	require.NoError(t, err)
	rooms, _, err = s.ListRooms(ctx, 20, "")
	require.NoError(t, err)
	require.Empty(t, rooms)
}

func TestAutoMatchUsesOldestOpenRoomOrCreatesFallback(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	s.newRoomID = fixedRoomIDs("AAA111", "BBB222", "CCC333")

	first, _, err := s.CreateRoomWithMeta(ctx, "sichuan_xzdd", "一号桌", false, "u1")
	require.NoError(t, err)
	second, _, err := s.CreateRoomWithMeta(ctx, "other", "其它规则桌", false, "u2")
	require.NoError(t, err)

	roomID, seat, err := s.AutoMatch(ctx, "sichuan_xzdd", "u3")
	require.NoError(t, err)
	require.Equal(t, first, roomID)
	require.EqualValues(t, 1, seat)

	roomID, seat, err = s.AutoMatch(ctx, "other", "u4")
	require.NoError(t, err)
	require.Equal(t, second, roomID)
	require.EqualValues(t, 1, seat)

	roomID, seat, err = s.AutoMatch(ctx, "new-rule", "u5")
	require.NoError(t, err)
	require.NotEmpty(t, roomID)
	require.EqualValues(t, 0, seat)
	require.NotEqual(t, first, roomID)
	require.NotEqual(t, second, roomID)
}

func TestCreateRoomWithMetaRetriesRoomIDCollision(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	calls := 0
	s.newRoomID = func() (string, error) {
		calls++
		if calls <= 2 {
			return "ABC123", nil
		}
		return "DEF456", nil
	}

	first, _, err := s.CreateRoomWithMeta(ctx, "", "", false, "u1")
	require.NoError(t, err)
	require.Equal(t, "ABC123", first)
	second, _, err := s.CreateRoomWithMeta(ctx, "", "", false, "u2")
	require.NoError(t, err)
	require.Equal(t, "DEF456", second)
	require.Equal(t, 3, calls)
}

func TestListRoomsPagination(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	s.newRoomID = fixedRoomIDs("ROOM01", "ROOM02", "ROOM03")
	for _, userID := range []string{"u1", "u2", "u3"} {
		_, _, err := s.CreateRoomWithMeta(ctx, "", "", false, userID)
		require.NoError(t, err)
	}

	firstPage, next, err := s.ListRooms(ctx, 2, "")
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	require.NotEmpty(t, next)
	secondPage, next, err := s.ListRooms(ctx, 2, next)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	require.Empty(t, next)
}

func fixedRoomIDs(ids ...string) func() (string, error) {
	i := 0
	return func() (string, error) {
		id := ids[i]
		i++
		return id, nil
	}
}
