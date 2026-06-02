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
	// 空 userID 在进入令牌桶逻辑前直接放行，与速率无关
	l := NewUserLimiter(10, 5)
	require.True(t, l.Allow(""))
}

func TestUserLimiterNilAllowed(t *testing.T) {
	t.Parallel()
	var l *UserLimiter
	require.True(t, l.Allow("u"))
}

func TestUserLimiterRefillsOverTime(t *testing.T) {
	t.Parallel()
	// rate=100 令牌/秒（每 10ms 补充一个），burst=2；
	// 耗尽后等待 50ms，远超 OS 定时器最粗粒度（Windows ~15ms），确保跨平台稳定。
	l := NewUserLimiter(100, 2)
	require.True(t, l.Allow("u"))
	require.True(t, l.Allow("u"))
	require.False(t, l.Allow("u"))
	time.Sleep(50 * time.Millisecond)
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
	c.SeenOrStore("ws", 1, "u", "k3") // 驱逐 k1（LRU）
	// 先验证保留项（k2/k3），再验证驱逐项（k1），避免检查 k1 时再次触发驱逐影响后续断言
	require.True(t, c.SeenOrStore("ws", 1, "u", "k2"), "k2 未被驱逐，应返回 true")
	require.True(t, c.SeenOrStore("ws", 1, "u", "k3"), "k3 未被驱逐，应返回 true")
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
	// 验证 Configure 确实替换了实例，而非只是非 nil
	require.NotSame(t, prev1, DefaultLimiter(), "Configure 应替换 limiter 实例")
	require.NotSame(t, prev2, DefaultCache(), "Configure 应替换 cache 实例")

	Configure(0, 0, 0) // 非正值回退默认
	require.NotNil(t, DefaultLimiter())
	require.NotNil(t, DefaultCache())
}
