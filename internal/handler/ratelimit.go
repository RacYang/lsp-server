package handler

import "racoo.cn/lsp/pkg/ratelimit"

// ConfigureRuntime 覆盖 WebSocket 入口限流与幂等缓存容量；非正值保留默认值。
func ConfigureRuntime(rate, burst float64, idemCacheSize int) {
	ratelimit.Configure(rate, burst, idemCacheSize)
}
