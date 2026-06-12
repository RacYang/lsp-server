package cluster

import (
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// etcdDialTimeout 是控制面 etcd 客户端的统一拨号超时。
const etcdDialTimeout = 5 * time.Second

// NewEtcdClient 是控制面 etcd 客户端的唯一构造点：端点解析、拨号超时与 TLS
// 材料三态语义（齐备启用、全空明文、半配置拒绝）统一在此收口，调用点不得
// 内联 clientv3.Config 自行决定连接形态。
func NewEtcdClient(endpoints, certFile, keyFile, caFile, serverName string) (*clientv3.Client, error) {
	tlsCfg, err := NewClientTLSConfig(certFile, keyFile, caFile, serverName)
	if err != nil {
		return nil, fmt.Errorf("构造 etcd 客户端 TLS: %w", err)
	}
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   ParseEndpoints(endpoints),
		DialTimeout: etcdDialTimeout,
		TLS:         tlsCfg,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 etcd 客户端: %w", err)
	}
	return cli, nil
}
