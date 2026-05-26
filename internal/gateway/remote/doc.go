// Package remote 实现通过 gRPC 对接集群 lobby/room 服务的远程房间网关。
//
// 职责：
//   - 代理 handler.RoomGateway 接口至 LobbyService 与 RoomService gRPC 端点
//   - 通过 Redis BLPOP 订阅房间实时事件并转发至 session.Hub
//   - 维护 gRPC 连接池与 etcd 路由缓存，支持多 room 节点负载均衡
//
// 禁止：
//   - 不得直接依赖 internal/service/**, internal/mahjong/**, internal/bot/**
//   - 不得依赖 cmd/ 或 internal/app/
package remote
