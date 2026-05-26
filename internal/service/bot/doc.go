// Package botsvc 实现占座机器人调度器（BotSupervisor）。
//
// 调度器在 room 服务进程内代占位 bot 出牌：每当 actor 处理完一条命令，
// supervisor 异步检查是否有 bot 座位需要响应，依次走策略决策并提交回 room.Service。
//
// 仅依赖 internal/service/room 的公共入口与 internal/bot 的策略接口，
// 不得引入传输、会话、存储或集群包。
package botsvc
