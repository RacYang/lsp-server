package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUserLimiterAllowsWithinBurst(t *testing.T) {
	t.Parallel()
	l := NewUserLimiter(10, 5)
	for i := 0; i < 5; i++ {
		require.True(t, l.Allow("u1"), "第 %d 次应放行", i+1)
	}
	require.False(t, l.Allow("u1"), "超出 burst 应限流")
}

func TestUserLimiterEmptyUserIDAlwaysAllowed(t *testing.T) {
	t.Parallel()
	l := NewUserLimiter(0.001, 0.001)
	require.True(t, l.Allow(""))
}

func TestUserLimiterNilAllowed(t *testing.T) {
	t.Parallel()
	var l *UserLimiter
	require.True(t, l.Allow("u"))
}

func TestUserLimiterRefillsOverTime(t *testing.T) {
	t.Parallel()
	// rate=1000 令牌/秒，先耗尽再等补充
	l := NewUserLimiter(1000, 2)
	require.True(t, l.Allow("u"))
	require.True(t, l.Allow("u"))
	require.False(t, l.Allow("u"))
	time.Sleep(3 * time.Millisecond)
	require.True(t, l.Allow("u"), "等待补充后应放行")
}

func TestUserLimiterSeparateUsers(t *testing.T) {
	t.Parallel()
	l := NewUserLimiter(10, 1)
	require.True(t, l.Allow("a"))
	require.False(t, l.Allow("a"))
	require.True(t, l.Allow("b"), "不同用户独立计数")
}

func TestIdemCacheSeenOrStore(t *testing.T) {
	t.Parallel()
	c := NewIdemCache(4)
	require.False(t, c.SeenOrStore("ws", 1, "u1", "key1"), "首次应返回 false")
	require.True(t, c.SeenOrStore("ws", 1, "u1", "key1"), "重复应返回 true")
	require.False(t, c.SeenOrStore("ws", 1, "u1", "key2"), "不同 key 应返回 false")
}

func TestIdemCacheLRUEviction(t *testing.T) {
	t.Parallel()
	c := NewIdemCache(2)
	c.SeenOrStore("ws", 1, "u", "k1")
	c.SeenOrStore("ws", 1, "u", "k2")
	c.SeenOrStore("ws", 1, "u", "k3") // 驱逐 k1
	require.False(t, c.SeenOrStore("ws", 1, "u", "k1"), "k1 已被驱逐，再次插入应返回 false")
}

func TestIdemCacheEmptyKeyIgnored(t *testing.T) {
	t.Parallel()
	c := NewIdemCache(10)
	require.False(t, c.SeenOrStore("ws", 1, "u", ""))
	require.False(t, c.SeenOrStore("ws", 1, "u", ""))
}

func TestIdemCacheNil(t *testing.T) {
	t.Parallel()
	var c *IdemCache
	require.False(t, c.SeenOrStore("ws", 1, "u", "k"))
}

func TestDefaultLimiterAndCache(t *testing.T) {
	t.Parallel()
	require.NotNil(t, DefaultLimiter())
	require.NotNil(t, DefaultCache())
}

func TestConfigureOverridesDefaults(t *testing.T) {
	prev1, prev2 := DefaultLimiter(), DefaultCache()
	t.Cleanup(func() {
		defaultLimiter.Store(prev1)
		defaultCache.Store(prev2)
	})

	Configure(5, 10, 32)
	require.NotNil(t, DefaultLimiter())
	require.NotNil(t, DefaultCache())

	Configure(0, 0, 0) // 非正值回退默认
	require.NotNil(t, DefaultLimiter())
}
