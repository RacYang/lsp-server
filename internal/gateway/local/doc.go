// Package local 实现进程内 contract.RoomGateway，供 cmd/all 单体部署与本地冒烟测试使用。
//
// 职责：将 handler 的 RoomGateway 调用转换为对本进程内 service.Service 的直接调用，
// 并通过 session.Hub 广播引擎通知；不涉及 gRPC 或网络序列化。
// 禁止持有游戏状态；路由与 replay 逻辑必须委托给 service 层。
package local
