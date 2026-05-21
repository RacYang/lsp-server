// Package app 组装各服务进程的依赖并启动监听。
//
// 职责：创建 gRPC 连接、Redis/Postgres 客户端、会话管理器与可观测组件，
// 将它们注入 handler 层，并管理服务的生命周期与优雅关闭。
//
// 禁止在本包内实现业务规则或直接操作牌局状态；业务逻辑属于 service 层。
package app
