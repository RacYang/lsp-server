package handler

import (
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/session"
)

func TestSelfSeatInfoUsesUserDirectory(t *testing.T) {
	users := session.NewUserDirectory(nil)
	require.NoError(t, users.Set(t.Context(), "u1", session.UserProfile{Nickname: "alice"}))

	seats := selfSeatInfo(t.Context(), Deps{Users: users}, 2, "u1")
	require.Len(t, seats, 4)
	require.EqualValues(t, 2, seats[2].GetSeatIndex())
	require.Equal(t, "u1", seats[2].GetUserId())
	require.Equal(t, "alice", seats[2].GetNickname())
	require.Empty(t, seats[0].GetUserId())
}

func TestNicknameForUserMissingDirectory(t *testing.T) {
	require.Empty(t, nicknameForUser(t.Context(), Deps{}, "u1"))
	require.Empty(t, nicknameForUser(t.Context(), Deps{Users: session.NewUserDirectory(nil)}, "u1"))
}
