// Package engine 是确定性游戏状态机，对应 ADR-0050 L3 层。
//
// 职责边界：
//   - 持有 RoundState（单局运行态）与 Engine（无状态驱动器）。
//   - Apply*(ctx, *RoundState, ...) 系列方法推进局面，返回 []Notification。
//   - phase.go 是 RoundState 阶段字段的唯一写入入口（ADR-0045）。
//
// 禁止事项：不得启动 goroutine / timer，不得访问 L4/L6/L7 任何包，
// 不得持久化数据，不得 import internal/service/room 或更上层包。
package engine
