package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"

	"racoo.cn/lsp/internal/cluster/nodeid"
)

const defaultLeaseTTL int64 = 30

// Etcd 负责节点注册、续租与 watch，控制面真相源落在 etcd。
type Etcd struct {
	cli      *clientv3.Client
	prefix   string
	leaseTTL int64
}

// NewEtcd 创建 etcd 控制面客户端；prefix 为空时回退到 /lsp。
func NewEtcd(cli *clientv3.Client, prefix string, leaseTTL int64) *Etcd {
	if strings.TrimSpace(prefix) == "" {
		prefix = "/lsp"
	}
	if leaseTTL <= 0 {
		leaseTTL = defaultLeaseTTL
	}
	return &Etcd{cli: cli, prefix: strings.TrimRight(prefix, "/"), leaseTTL: leaseTTL}
}

func (e *Etcd) nodePrefix(kind nodeid.Kind) string {
	return fmt.Sprintf("%s/nodes/%s", e.prefix, kind)
}

func (e *Etcd) nodeKey(kind nodeid.Kind, nodeID string) string {
	return fmt.Sprintf("%s/%s", e.nodePrefix(kind), strings.TrimSpace(nodeID))
}

// Register 使用租约写入节点元数据；节点退出或失联后键会自动过期。
func (e *Etcd) Register(ctx context.Context, kind nodeid.Kind, nodeID string, meta NodeMeta) (int64, error) {
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
func (e *Etcd) KeepAlive(ctx context.Context, leaseID int64) error {
	if e == nil || e.cli == nil {
		return fmt.Errorf("nil etcd client")
	}
	_, err := e.cli.KeepAliveOnce(ctx, clientv3.LeaseID(leaseID))
	return err
}

// Revoke 主动撤销租约，等价于节点优雅下线。
func (e *Etcd) Revoke(ctx context.Context, leaseID int64) error {
	if e == nil || e.cli == nil {
		return fmt.Errorf("nil etcd client")
	}
	_, err := e.cli.Revoke(ctx, clientv3.LeaseID(leaseID))
	return err
}

// WatchNodes 先发一份快照，再在变更时重新拉取该 kind 的最新列表。
func (e *Etcd) WatchNodes(ctx context.Context, kind nodeid.Kind) (<-chan []NodeInfo, error) {
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

func (e *Etcd) listNodes(ctx context.Context, kind nodeid.Kind) ([]NodeInfo, error) {
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
func (e *Etcd) ResolveNode(ctx context.Context, kind nodeid.Kind, nodeID string) (NodeInfo, bool, error) {
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
