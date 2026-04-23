// 房间状态机单元测试，覆盖主路径与非法迁移。
package room

import "testing"

func TestFSMHappyPath(t *testing.T) {
	f := NewFSM()
	for _, s := range []State{StateWaiting, StateReady, StatePlaying, StateSettling, StateClosed} {
		if err := f.Transition(s); err != nil {
			t.Fatalf("to %s: %v", s, err)
		}
	}
}

func TestFSMIllegal(t *testing.T) {
	f := NewFSM()
	if err := f.Transition(StatePlaying); err == nil {
		t.Fatal("expected error")
	}
}
