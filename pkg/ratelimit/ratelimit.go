package ratelimit

import (
	"container/list"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// TokenBucket 为单用户令牌桶状态。
type TokenBucket struct {
	tokens float64
	last   time.Time
}

// UserLimiter 基于令牌桶算法对每个用户独立限流。
type UserLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*TokenBucket
}

// NewUserLimiter 创建限流器；rate 为每秒令牌生成量，burst 为峰值容量。
func NewUserLimiter(rate, burst float64) *UserLimiter {
	return &UserLimiter{rate: rate, burst: burst, buckets: make(map[string]*TokenBucket)}
}

// Allow 判断 userID 是否在限流阈值内；空 userID 始终放行。
func (l *UserLimiter) Allow(userID string) bool {
	if l == nil || userID == "" {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.buckets[userID]
	if b == nil {
		l.buckets[userID] = &TokenBucket{tokens: l.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// IdemCache 为内存 LRU 幂等缓存；相同键的重复请求返回 true。
type IdemCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

type idemEntry struct {
	key string
}

// NewIdemCache 创建容量为 capacity 的幂等缓存。
func NewIdemCache(capacity int) *IdemCache {
	return &IdemCache{capacity: capacity, items: make(map[string]*list.Element), order: list.New()}
}

// SeenOrStore 检查幂等键是否已见过；首次见到时存入并返回 false，重复时返回 true。
func (c *IdemCache) SeenOrStore(scope string, msgID uint16, userID, key string) bool {
	if c == nil || key == "" {
		return false
	}
	fullKey := fmt.Sprintf("%s:%d:%s:%s", scope, msgID, userID, key)
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem := c.items[fullKey]; elem != nil {
		c.order.MoveToFront(elem)
		return true
	}
	elem := c.order.PushFront(idemEntry{key: fullKey})
	c.items[fullKey] = elem
	for c.order.Len() > c.capacity {
		back := c.order.Back()
		if back == nil {
			break
		}
		entry, ok2 := back.Value.(idemEntry)
		if !ok2 {
			c.order.Remove(back)
			continue
		}
		delete(c.items, entry.key)
		c.order.Remove(back)
	}
	return false
}

var (
	defaultLimiter atomic.Pointer[UserLimiter]
	defaultCache   atomic.Pointer[IdemCache]
)

func init() {
	defaultLimiter.Store(NewUserLimiter(20, 40))
	defaultCache.Store(NewIdemCache(4096))
}

// DefaultLimiter 返回全局默认限流器。
func DefaultLimiter() *UserLimiter { return defaultLimiter.Load() }

// DefaultCache 返回全局默认幂等缓存。
func DefaultCache() *IdemCache { return defaultCache.Load() }

// Configure 覆盖全局默认限流器与幂等缓存；非正值保留默认值。
func Configure(rate, burst float64, idemCacheSize int) {
	if rate <= 0 {
		rate = 20
	}
	if burst <= 0 {
		burst = 40
	}
	if idemCacheSize <= 0 {
		idemCacheSize = 4096
	}
	defaultLimiter.Store(NewUserLimiter(rate, burst))
	defaultCache.Store(NewIdemCache(idemCacheSize))
}
