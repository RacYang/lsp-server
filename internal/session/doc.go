// Package session 管理 WebSocket 连接注册、用户目录与房间级广播（Phase 1 内存实现）。
//
// 职责：Hub 连接广播、Manager 会话生命周期、token 生成与 UserDirectory 在线状态。
// 禁止在本包内实现业务规则；不得直接访问存储层（通过接口注入）。
package session
