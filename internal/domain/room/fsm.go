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
