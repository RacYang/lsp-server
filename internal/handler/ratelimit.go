package handler

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

type tokenBucket struct {
	tokens float64
	last   time.Time
}

type userRateLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*tokenBucket
}

func newUserRateLimiter(rate, burst float64) *userRateLimiter {
	return &userRateLimiter{rate: rate, burst: burst, buckets: make(map[string]*tokenBucket)}
}

func (l *userRateLimiter) Allow(userID string) bool {
	if l == nil || userID == "" {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.buckets[userID]
	if b == nil {
		l.buckets[userID] = &tokenBucket{tokens: l.burst - 1, last: now}
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

type idemCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

type idemEntry struct {
	key string
}

func newIdemCache(capacity int) *idemCache {
	return &idemCache{capacity: capacity, items: make(map[string]*list.Element), order: list.New()}
}

func (c *idemCache) SeenOrStore(scope string, msgID uint16, userID, key string) bool {
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
		entry := back.Value.(idemEntry)
		delete(c.items, entry.key)
		c.order.Remove(back)
	}
	return false
}

var (
	defaultWSRateLimiter = newUserRateLimiter(20, 40)
	defaultWSIdemCache   = newIdemCache(4096)
)
