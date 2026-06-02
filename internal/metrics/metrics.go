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

// EtcdKeepaliveFailTotal 统计 etcd 续租失败次数；持续增长时说明 etcd 连接异常。
var EtcdKeepaliveFailTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "etcd_keepalive_fail_total",
	Help:      "etcd 租约续租失败计数。",
})

// RoomRecoverSkipTotal 统计冷启动房间恢复因超时被跳过的次数。
var RoomRecoverSkipTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "room_recover_skip_total",
	Help:      "冷启动超时导致房间恢复被跳过的计数。",
})

// GRPCApplyEventTotal 统计 room gRPC ApplyEvent 请求数。
var GRPCApplyEventTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "lsp",
	Name:      "grpc_apply_event_total",
	Help:      "room gRPC ApplyEvent 请求计数。",
}, []string{"result"})

// GRPCApplyEventSeconds 统计 room gRPC ApplyEvent 端到端耗时。
var GRPCApplyEventSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
	Namespace: "lsp",
	Name:      "grpc_apply_event_seconds",
	Help:      "room gRPC ApplyEvent 端到端耗时（含持久化）。",
	Buckets:   prometheus.DefBuckets,
})

func ObserveStorage(store, op string, started time.Time, err error) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	StorageOpSeconds.WithLabelValues(store, op, result).Observe(time.Since(started).Seconds())
}
