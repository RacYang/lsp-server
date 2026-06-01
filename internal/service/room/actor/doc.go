// Package actor 是房间命令队列与串行执行器，对应 ADR-0050 L4 层的并发编排职责。
//
// 职责边界：
//   - Actor 持有单房间的 mailbox goroutine，串行执行所有游戏命令。
//   - Scheduler 只读 RoundState.Deadline() 对齐 OS 定时器，不写状态。
//   - 所有对引擎状态的读取通过 engine.RoundState accessor 方法完成。
//
// 禁止事项：不得直接读写 engine.RoundState 未导出字段，不得持有对
// service/room.Service 的反向引用，不得 import adapter/handler/gateway。
package actor
