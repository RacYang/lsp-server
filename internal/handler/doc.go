// Package handler 将二进制帧路由到具体业务处理器，并调用应用服务层。
//
// 职责：WebSocket 升级、帧解析与分发、限流/幂等过滤、会话状态维护。
// 进程内网关适配由 internal/adapter/local 承担，远程网关由 internal/gateway/remote 承担。
//
// 禁止在本包内实现业务规则或直接访问存储层；所有业务调用须经由 RoomGateway 接口。
package handler
