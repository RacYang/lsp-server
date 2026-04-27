// Package metrics 集中定义跨层业务指标，避免各层重复注册。
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var ClaimWindowTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "claim_window_total",
	Help:      "抢答窗口结果计数。",
}, []string{"result"})

var AutoTimeoutTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "auto_timeout_total",
	Help:      "服务端托管超时计数。",
}, []string{"kind"})

var ReconnectTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "reconnect_total",
	Help:      "重连结果计数。",
}, []string{"result"})

var ActorQueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "lsp",
	Name:      "actor_queue_depth",
	Help:      "房间 actor mailbox 当前队列深度。",
}, []string{"room"})

var StorageOpSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "lsp",
	Name:      "storage_op_seconds",
	Help:      "存储操作耗时。",
	Buckets:   prometheus.DefBuckets,
}, []string{"store", "op", "result"})

var StorageRetryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "storage_retry_total",
	Help:      "存储操作重试计数。",
}, []string{"store", "op", "result"})

var SettlementPenaltyTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "settlement_penalty_total",
	Help:      "局末罚分与退税条目计数。",
}, []string{"reason"})

func ObserveStorage(store, op string, started time.Time, err error) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	StorageOpSeconds.WithLabelValues(store, op, result).Observe(time.Since(started).Seconds())
}
