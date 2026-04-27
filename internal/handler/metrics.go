package handler

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// wsFramesTotal 统计 WebSocket 入站帧数量，供 Prometheus 抓取。
var wsFramesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "lsp",
	Subsystem: "gate",
	Name:      "websocket_frames_total",
	Help:      "网关 WebSocket 入站帧计数，按 msg_id 分组。",
}, []string{"msg_id"})

var rateLimitedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "rate_limited_total",
	Help:      "服务端限流计数，按 gate 或 room 层分组。",
}, []string{"layer"})

var idempotentReplayTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "idempotent_replay_total",
	Help:      "幂等键重放请求计数。",
})

var unknownMsgTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "unknown_msg_total",
	Help:      "未知 WebSocket msg_id 计数。",
})
