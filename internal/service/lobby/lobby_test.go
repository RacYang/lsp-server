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
