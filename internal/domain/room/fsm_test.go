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

func TestFSMRestoreAcceptsPersistableStates(t *testing.T) {
	for _, s := range []State{StateWaiting, StateReady, StatePlaying, StateSettling, StateClosed} {
		f := NewFSM()
		if err := f.Restore(s); err != nil {
			t.Fatalf("restore to %s: %v", s, err)
		}
		if f.State() != s {
			t.Fatalf("restore to %s, got state %s", s, f.State())
		}
	}
}

func TestFSMRestoreRejectsIdleAndUnknown(t *testing.T) {
	for _, bad := range []State{StateIdle, "", "garbage"} {
		f := NewFSM()
		if err := f.Restore(bad); err == nil {
			t.Fatalf("restore to %q expected error, got nil", bad)
		}
		if f.State() != StateIdle {
			t.Fatalf("rejected restore must not mutate state, got %s", f.State())
		}
	}
}

func TestFSMRestoreOverridesAnyCurrentState(t *testing.T) {
	f := NewFSM()
	if err := f.Transition(StateWaiting); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := f.Transition(StateReady); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := f.Restore(StateClosed); err != nil {
		t.Fatalf("restore overriding state: %v", err)
	}
	if f.State() != StateClosed {
		t.Fatalf("state mismatch after override, got %s", f.State())
	}
}

func TestFSMRestoreNilReceiver(t *testing.T) {
	var f *FSM
	if err := f.Restore(StatePlaying); err == nil {
		t.Fatal("nil receiver: expected error")
	}
}
