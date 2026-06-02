// Package ratelimit 提供 WebSocket 入口令牌桶限流与内存幂等缓存。
//
// 职责：TokenBucket 单用户限流；IdempotencyCache LRU 幂等去重。
// 本包从 service/room 下沉至 pkg/，供 handler 直接引用（ADR-0050 L6）。
// 禁止在本包内引入业务规则或存储依赖。
package ratelimit
