package discovery

import (
	"context"
	"time"

	"racoo.cn/lsp/internal/cluster/nodeid"
)

// Registration 表示一个带租约的节点注册。
type Registration struct {
	NodeID  string
	LeaseID int64
	disco   *Etcd
}

// RegisterAndKeepAlive 注册节点并启动周期续租；nodeID 为空时自动生成。
func (e *Etcd) RegisterAndKeepAlive(ctx context.Context, kind nodeid.Kind, nodeID string, meta NodeMeta, interval time.Duration) (*Registration, error) {
	if nodeID == "" {
		nodeID = nodeid.New()
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	leaseID, err := e.Register(ctx, kind, nodeID, meta)
	if err != nil {
		return nil, err
	}
	reg := &Registration{NodeID: nodeID, LeaseID: leaseID, disco: e}
	go reg.keepAlive(ctx, interval)
	return reg, nil
}

func (r *Registration) keepAlive(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.disco.KeepAlive(ctx, r.LeaseID)
		}
	}
}

// Stop 主动撤销注册租约。
func (r *Registration) Stop(ctx context.Context) error {
	if r == nil || r.disco == nil || r.LeaseID == 0 {
		return nil
	}
	return r.disco.Revoke(ctx, r.LeaseID)
}
