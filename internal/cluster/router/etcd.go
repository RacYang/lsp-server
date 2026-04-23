package router

import (
	"context"
	"fmt"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Etcd 负责房间 ownership 的 claim 与 resolve；它是 room affinity 的权威真相源。
type Etcd struct {
	cli    *clientv3.Client
	prefix string
}

// NewEtcd 创建房间路由客户端；prefix 为空时回退到 /lsp。
func NewEtcd(cli *clientv3.Client, prefix string) *Etcd {
	if strings.TrimSpace(prefix) == "" {
		prefix = "/lsp"
	}
	return &Etcd{cli: cli, prefix: strings.TrimRight(prefix, "/")}
}

func (e *Etcd) roomKey(roomID string) string {
	return fmt.Sprintf("%s/rooms/%s/owner", e.prefix, SanitizeRoomID(roomID))
}

// ClaimRoom 使用 compare-and-set 声明房间归属；已被其他节点占用时返回错误。
func (e *Etcd) ClaimRoom(ctx context.Context, roomID, roomNodeID string, leaseID int64) error {
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
func (e *Etcd) ResolveRoomOwner(ctx context.Context, roomID string) (string, bool, error) {
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
