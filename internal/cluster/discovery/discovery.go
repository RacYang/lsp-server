package discovery

import (
	"context"

	"racoo.cn/lsp/internal/cluster/nodeid"
)

// NodeMeta 描述向控制面注册的节点元数据（地址、版本等）。
type NodeMeta struct {
	// AdvertiseAddr 为对端可连接的 gRPC 或 HTTP 地址（如 host:port）。
	AdvertiseAddr string
	// Version 为进程构建版本，便于灰度观察。
	Version string
}

// Registrar 抽象节点注册：租约续期失败时 Watch 侧应感知下线。
type Registrar interface {
	Register(ctx context.Context, kind nodeid.Kind, nodeID string, meta NodeMeta) (leaseID int64, err error)
	KeepAlive(ctx context.Context, leaseID int64) error
	Revoke(ctx context.Context, leaseID int64) error
}

// NodeInfo 为发现结果中的单条节点描述。
type NodeInfo struct {
	NodeID string
	Kind   nodeid.Kind
	Meta   NodeMeta
}

// Watcher 用于订阅节点集合变化（前缀 watch 由实现封装）。
type Watcher interface {
	WatchNodes(ctx context.Context, kind nodeid.Kind) (<-chan []NodeInfo, error)
}

// KindString 返回用于日志与指标的角色字符串（小写）。
func KindString(k nodeid.Kind) string {
	return string(k)
}
