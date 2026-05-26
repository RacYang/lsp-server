package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// NodeMeta 描述向控制面注册的节点元数据（地址、版本等）。
type NodeMeta struct {
	// AdvertiseAddr 为对端可连接的 gRPC 或 HTTP 地址（如 host:port）。
	AdvertiseAddr string
	// ExternalWSURL 为可选的客户端可达 WebSocket 完整 URL，仅用于观测或跨入口重定向。
	ExternalWSURL string
	// Version 为进程构建版本，便于灰度观察。
	Version string
}

// Registrar 抽象节点注册：租约续期失败时 Watch 侧应感知下线。
type Registrar interface {
	Register(ctx context.Context, kind Kind, nodeID string, meta NodeMeta) (leaseID int64, err error)
	KeepAlive(ctx context.Context, leaseID int64) error
	Revoke(ctx context.Context, leaseID int64) error
}

// NodeInfo 为发现结果中的单条节点描述。
type NodeInfo struct {
	NodeID string
	Kind   Kind
	Meta   NodeMeta
}

// Watcher 用于订阅节点集合变化（前缀 watch 由实现封装）。
type Watcher interface {
	WatchNodes(ctx context.Context, kind Kind) (<-chan []NodeInfo, error)
}

// NodeDiscovery 为服务发现接口，统一节点注册、续租、撤销、Watch 与单点查询。
type NodeDiscovery interface {
	Registrar
	Watcher
	ResolveNode(ctx context.Context, kind Kind, nodeID string) (NodeInfo, bool, error)
}

const defaultLeaseTTL int64 = 30

// EtcdDiscovery 负责节点注册、续租与 watch，控制面真相源落在 etcd。
type EtcdDiscovery struct {
	cli      *clientv3.Client
	prefix   string
	leaseTTL int64
}

// NewEtcdDiscovery 创建 etcd 控制面客户端；prefix 为空时回退到 /lsp。
func NewEtcdDiscovery(cli *clientv3.Client, prefix string, leaseTTL int64) *EtcdDiscovery {
	if strings.TrimSpace(prefix) == "" {
		prefix = "/lsp"
	}
	if leaseTTL <= 0 {
		leaseTTL = defaultLeaseTTL
	}
	return &EtcdDiscovery{cli: cli, prefix: strings.TrimRight(prefix, "/"), leaseTTL: leaseTTL}
}

func (e *EtcdDiscovery) nodePrefix(kind Kind) string {
	return fmt.Sprintf("%s/nodes/%s", e.prefix, kind)
}

func (e *EtcdDiscovery) nodeKey(kind Kind, nodeID string) string {
	return fmt.Sprintf("%s/%s", e.nodePrefix(kind), strings.TrimSpace(nodeID))
}

// Register 使用租约写入节点元数据；节点退出或失联后键会自动过期。
func (e *EtcdDiscovery) Register(ctx context.Context, kind Kind, nodeID string, meta NodeMeta) (int64, error) {
	if e == nil || e.cli == nil {
		return 0, fmt.Errorf("nil etcd client")
	}
	lease, err := e.cli.Grant(ctx, e.leaseTTL)
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return 0, err
	}
	if _, err := e.cli.Put(ctx, e.nodeKey(kind, nodeID), string(payload), clientv3.WithLease(lease.ID)); err != nil {
		return 0, err
	}
	return int64(lease.ID), nil
}

// KeepAlive 续租一次；调用方可在 ticker 中周期调用。
func (e *EtcdDiscovery) KeepAlive(ctx context.Context, leaseID int64) error {
	if e == nil || e.cli == nil {
		return fmt.Errorf("nil etcd client")
	}
	_, err := e.cli.KeepAliveOnce(ctx, clientv3.LeaseID(leaseID))
	return err
}

// Revoke 主动撤销租约，等价于节点优雅下线。
func (e *EtcdDiscovery) Revoke(ctx context.Context, leaseID int64) error {
	if e == nil || e.cli == nil {
		return fmt.Errorf("nil etcd client")
	}
	_, err := e.cli.Revoke(ctx, clientv3.LeaseID(leaseID))
	return err
}

// WatchNodes 先发一份快照，再在变更时重新拉取该 kind 的最新列表。
func (e *EtcdDiscovery) WatchNodes(ctx context.Context, kind Kind) (<-chan []NodeInfo, error) {
	if e == nil || e.cli == nil {
		return nil, fmt.Errorf("nil etcd client")
	}
	out := make(chan []NodeInfo, 8)
	prefix := e.nodePrefix(kind)
	snapshot, err := e.listNodes(ctx, kind)
	if err != nil {
		return nil, err
	}
	out <- snapshot
	watchCh := e.cli.Watch(ctx, prefix, clientv3.WithPrefix())
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case wr, ok := <-watchCh:
				if !ok {
					return
				}
				if wr.Err() != nil {
					return
				}
				snapshot, err := e.listNodes(ctx, kind)
				if err != nil {
					return
				}
				select {
				case out <- snapshot:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (e *EtcdDiscovery) listNodes(ctx context.Context, kind Kind) ([]NodeInfo, error) {
	resp, err := e.cli.Get(ctx, e.nodePrefix(kind), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	out := make([]NodeInfo, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var meta NodeMeta
		if err := json.Unmarshal(kv.Value, &meta); err != nil {
			return nil, err
		}
		out = append(out, NodeInfo{NodeID: path.Base(string(kv.Key)), Kind: kind, Meta: meta})
	}
	return out, nil
}

// ResolveNode 读取单个节点元数据；键不存在时 ok=false。
func (e *EtcdDiscovery) ResolveNode(ctx context.Context, kind Kind, nodeID string) (NodeInfo, bool, error) {
	if e == nil || e.cli == nil {
		return NodeInfo{}, false, fmt.Errorf("nil etcd client")
	}
	resp, err := e.cli.Get(ctx, e.nodeKey(kind, nodeID))
	if err != nil {
		return NodeInfo{}, false, err
	}
	if len(resp.Kvs) == 0 {
		return NodeInfo{}, false, nil
	}
	var meta NodeMeta
	if err := json.Unmarshal(resp.Kvs[0].Value, &meta); err != nil {
		return NodeInfo{}, false, err
	}
	return NodeInfo{NodeID: nodeID, Kind: kind, Meta: meta}, true, nil
}

// Registration 表示一个带租约的节点注册。
type Registration struct {
	NodeID  string
	LeaseID int64
	disco   *EtcdDiscovery
}

// RegisterAndKeepAlive 注册节点并启动周期续租；nodeID 为空时自动生成。
func (e *EtcdDiscovery) RegisterAndKeepAlive(ctx context.Context, kind Kind, nodeID string, meta NodeMeta, interval time.Duration) (*Registration, error) {
	if nodeID == "" {
		nodeID = NewNodeID()
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
