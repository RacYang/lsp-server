// Package room 表示房间聚合与状态机，约束房间生命周期与合法命令。
package room

// State 为房间 FSM 状态。
type State string

const (
	StateIdle     State = "idle"
	StateWaiting  State = "waiting"
	StateReady    State = "ready"
	StatePlaying  State = "playing"
	StateSettling State = "settling"
	StateClosed   State = "closed"
)
