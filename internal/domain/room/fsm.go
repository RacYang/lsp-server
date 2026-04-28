package room

import "fmt"

// FSM 为房间状态机，仅描述合法迁移，不包含网络或存储。
type FSM struct {
	state State
}

// NewFSM 创建从 Idle 开始的状态机。
func NewFSM() *FSM {
	return &FSM{state: StateIdle}
}

// State 返回当前状态。
func (f *FSM) State() State {
	if f == nil {
		return StateIdle
	}
	return f.state
}

// Transition 执行迁移；非法迁移返回错误。
func (f *FSM) Transition(next State) error {
	if f == nil {
		return fmt.Errorf("nil fsm")
	}
	ok := false
	switch f.state {
	case StateIdle:
		ok = next == StateWaiting
	case StateWaiting:
		ok = next == StateReady || next == StateClosed
	case StateReady:
		ok = next == StatePlaying || next == StateWaiting
	case StatePlaying:
		ok = next == StateSettling || next == StateClosed
	case StateSettling:
		ok = next == StateClosed
	case StateClosed:
		ok = false
	}
	if !ok {
		return fmt.Errorf("illegal transition %s -> %s", f.state, next)
	}
	f.state = next
	return nil
}

// Restore 把 FSM 直接置为给定状态；只用于从持久化数据恢复，不走普通 transition guard。
//
// 与 Transition 的差异：恢复路径需要跨越多步（如 idle→playing），但每步逐次 transition 在
// 中间出错时会留下"半恢复"状态，不可观测且难以回滚；Restore 一次性置位，要么完全成功要么不动。
//
// 仅接受可持久化的稳定状态，不接受 StateIdle（idle 由 NewFSM 隐式表达）。
func (f *FSM) Restore(target State) error {
	if f == nil {
		return fmt.Errorf("nil fsm")
	}
	switch target {
	case StateWaiting, StateReady, StatePlaying, StateSettling, StateClosed:
		f.state = target
		return nil
	default:
		return fmt.Errorf("illegal restore target: %q", target)
	}
}
