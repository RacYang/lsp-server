// Package roomadapter 将 room 服务节点适配为 gRPC 传输层。
//
// 本包只承担协议适配职责：将 svcv1.RoomService RPC 请求翻译为
// roomsvc.Service 调用，并将通知事件持久化后推送到 Redis 队列或遗留流式通道。
// 不得包含麻将规则逻辑，不得依赖 app 或 cmd 包。
package roomadapter
