package redis_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/store/redis"
)

func TestHashSessionTokenStable(t *testing.T) {
	t.Parallel()
	a := redis.HashSessionToken("plain-token-xyz")
	b := redis.HashSessionToken("plain-token-xyz")
	require.Equal(t, a, b)
	require.NotEmpty(t, a)
	require.Empty(t, redis.HashSessionToken(""))
}
