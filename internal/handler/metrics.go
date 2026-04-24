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
