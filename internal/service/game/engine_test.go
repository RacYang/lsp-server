package game

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	_ "racoo.cn/lsp/internal/mahjong/sichuanxzdd"
)

func TestNewEngine(t *testing.T) {
	t.Parallel()
	e := NewEngine("")
	require.NotNil(t, e)
}

func TestPlayAutoRound(t *testing.T) {
	t.Parallel()

	e := NewEngine("sichuan_xzdd")
	notifies, err := e.PlayAutoRound(context.Background(), "room-auto-1", [4]string{"u0", "u1", "u2", "u3"})
	require.NoError(t, err)
	require.NotEmpty(t, notifies)
	require.Equal(t, KindExchangeThreeDone, notifies[0].Kind)
	require.Equal(t, KindQueMenDone, notifies[1].Kind)
	require.Equal(t, KindStartGame, notifies[2].Kind)
	require.Equal(t, KindSettlement, notifies[len(notifies)-1].Kind)
}
