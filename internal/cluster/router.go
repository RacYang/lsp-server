package cluster

import (
	"context"
	"fmt"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Resolver 根据房间 ID 解析应转发到的 room 节点 ID；未找到时 ok 为 false。
type Resolver interface {
	ResolveRoomOwner(ctx context.Context, roomID string) (nodeID string, ok bool, err error)
}

// Claimer 由 lobby 在创建或首次绑定时声明房间归属（与租约绑定）。
type Claimer interface {
	ClaimRoom(ctx context.Context, roomID, roomNodeID string, leaseID int64) error
}

// SanitizeRoomID 去除 roomID 两端空白，避免 etcd 键误拼空格。
func SanitizeRoomID(roomID string) string {
	return strings.TrimSpace(roomID)
}

// EtcdRouter 负责房间 ownership 的 claim 与 resolve；是 room affinity 的权威真相源。
type EtcdRouter struct {
	cli    *clientv3.Client
	prefix string
}

// NewEtcdRouter 创建房间路由客户端；prefix 为空时回退到 /lsp。
func NewEtcdRouter(cli *clientv3.Client, prefix string) *EtcdRouter {
	if strings.TrimSpace(prefix) == "" {
		prefix = "/lsp"
	}
	return &EtcdRouter{cli: cli, prefix: strings.TrimRight(prefix, "/")}
}

func (e *EtcdRouter) roomKey(roomID string) string {
	return fmt.Sprintf("%s/rooms/%s/owner", e.prefix, SanitizeRoomID(roomID))
}

func (e *EtcdRouter) roomPrefix() string {
	return fmt.Sprintf("%s/rooms/", e.prefix)
}

// ClaimRoom 使用 compare-and-set 声明房间归属；已被其他节点占用时返回错误。
func (e *EtcdRouter) ClaimRoom(ctx context.Context, roomID, roomNodeID string, leaseID int64) error {
	if e == nil || e.cli == nil {
		return fmt.Errorf("nil etcd client")
	}
	key := e.roomKey(roomID)
	getResp, err := e.cli.Get(ctx, key)
	if err != nil {
		return err
	}
	if len(getResp.Kvs) > 0 && string(getResp.Kvs[0].Value) == roomNodeID {
		return nil
	}
	op := clientv3.OpPut(key, roomNodeID)
	if leaseID > 0 {
		op = clientv3.OpPut(key, roomNodeID, clientv3.WithLease(clientv3.LeaseID(leaseID)))
	}
	txnResp, err := e.cli.Txn(ctx).
		If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
		Then(op).
		Commit()
	if err != nil {
		return err
	}
	if !txnResp.Succeeded {
		return fmt.Errorf("room already claimed: %s", roomID)
	}
	return nil
}

// ResolveRoomOwner 读取 room -> node 归属；键不存在时 ok=false。
func (e *EtcdRouter) ResolveRoomOwner(ctx context.Context, roomID string) (string, bool, error) {
	if e == nil || e.cli == nil {
		return "", false, fmt.Errorf("nil etcd client")
	}
	resp, err := e.cli.Get(ctx, e.roomKey(roomID))
	if err != nil {
		return "", false, err
	}
	if len(resp.Kvs) == 0 {
		return "", false, nil
	}
	return string(resp.Kvs[0].Value), true, nil
}

// ListRoomsByOwner 枚举当前归属于某 room 节点的房间，供进程冷启动恢复使用。
func (e *EtcdRouter) ListRoomsByOwner(ctx context.Context, roomNodeID string) ([]string, error) {
	if e == nil || e.cli == nil {
		return nil, fmt.Errorf("nil etcd client")
	}
	resp, err := e.cli.Get(ctx, e.roomPrefix(), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		if string(kv.Value) != roomNodeID {
			continue
		}
		key := string(kv.Key)
		if !strings.HasPrefix(key, e.roomPrefix()) || !strings.HasSuffix(key, "/owner") {
			continue
		}
		roomID := strings.TrimSuffix(strings.TrimPrefix(key, e.roomPrefix()), "/owner")
		if roomID != "" {
			out = append(out, roomID)
		}
	}
	return out, nil
}
