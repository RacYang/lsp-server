// Package cluster 合并节点 ID 生成、服务发现与房间路由三个子包，统一 etcd 控制面操作。
//
// 禁止：不得依赖 session、handler、store、service 或 app 等层。
package cluster
