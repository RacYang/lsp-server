// 补充房间聚合根与 FSM 边界用例，提升覆盖率。
package room

import "testing"

func TestNilRoomMethods(t *testing.T) {
	var r *Room
	if r.SeatOf(0) != "" {
		t.Fatal("nil SeatOf")
	}
	if _, ok := r.JoinAutoSeat("x"); ok {
		t.Fatal("nil JoinAutoSeat")
	}
	if err := r.SetReady(0, true); err != nil {
		t.Fatal(err)
	}
	if err := r.StartPlaying(); err != nil {
		t.Fatal(err)
	}
	if err := r.CloseToSettling(); err != nil {
		t.Fatal(err)
	}
	if err := r.CloseRoom(); err != nil {
		t.Fatal(err)
	}
}

func TestSeatOfBounds(t *testing.T) {
	r := NewRoom("x")
	if r.SeatOf(-1) != "" || r.SeatOf(4) != "" {
		t.Fatal("SeatOf bounds")
	}
	if r.SeatOf(0) != "" {
		t.Fatal("empty seat")
	}
	_, _ = r.JoinAutoSeat("u0")
	if r.SeatOf(0) != "u0" {
		t.Fatalf("got %q", r.SeatOf(0))
	}
}

func TestSetReadyBackToWaiting(t *testing.T) {
	r := NewRoom("x")
	for i := 0; i < 4; i++ {
		_, _ = r.JoinAutoSeat("u" + string(rune('0'+i)))
	}
	for i := 0; i < 4; i++ {
		if err := r.SetReady(i, true); err != nil {
			t.Fatal(err)
		}
	}
	if r.FSM.State() != StateReady {
		t.Fatalf("state %s", r.FSM.State())
	}
	if err := r.SetReady(0, false); err != nil {
		t.Fatal(err)
	}
	if r.FSM.State() != StateWaiting {
		t.Fatalf("want waiting got %s", r.FSM.State())
	}
}

func TestPlayingLifecycle(t *testing.T) {
	r := NewRoom("x")
	for i := 0; i < 4; i++ {
		_, _ = r.JoinAutoSeat("p" + string(rune('0'+i)))
	}
	for i := 0; i < 4; i++ {
		_ = r.SetReady(i, true)
	}
	if err := r.StartPlaying(); err != nil {
		t.Fatal(err)
	}
	if r.FSM.State() != StatePlaying {
		t.Fatalf("want playing got %s", r.FSM.State())
	}
	if err := r.CloseToSettling(); err != nil {
		t.Fatal(err)
	}
	if r.FSM.State() != StateSettling {
		t.Fatalf("want settling got %s", r.FSM.State())
	}
	if err := r.CloseRoom(); err != nil {
		t.Fatal(err)
	}
	if r.FSM.State() != StateClosed {
		t.Fatalf("want closed got %s", r.FSM.State())
	}
}

func TestStartPlayingWrongState(t *testing.T) {
	r := NewRoom("x")
	if err := r.StartPlaying(); err != nil {
		t.Fatal(err)
	}
	if r.FSM.State() != StateWaiting {
		t.Fatal("state changed")
	}
}

func TestCloseSettlingWrongState(t *testing.T) {
	r := NewRoom("x")
	if err := r.CloseToSettling(); err != nil {
		t.Fatal(err)
	}
	if err := r.CloseRoom(); err != nil {
		t.Fatal(err)
	}
}

func TestFSMNilReceiver(t *testing.T) {
	var f *FSM
	if f.State() != StateIdle {
		t.Fatal("nil FSM State")
	}
	if err := f.Transition(StateWaiting); err == nil {
		t.Fatal("nil Transition want error")
	}
}

func TestFSMMoreIllegal(t *testing.T) {
	f := NewFSM()
	_ = f.Transition(StateWaiting)
	if err := f.Transition(StateSettling); err == nil {
		t.Fatal("waiting->settling")
	}
	_ = f.Transition(StateReady)
	if err := f.Transition(StateClosed); err == nil {
		t.Fatal("ready->closed direct")
	}
	_ = f.Transition(StatePlaying)
	if err := f.Transition(StateReady); err == nil {
		t.Fatal("playing->ready")
	}
}

func TestPlayingToClosed(t *testing.T) {
	f := NewFSM()
	for _, s := range []State{StateWaiting, StateReady, StatePlaying} {
		_ = f.Transition(s)
	}
	if err := f.Transition(StateClosed); err != nil {
		t.Fatal(err)
	}
	if f.State() != StateClosed {
		t.Fatal(f.State())
	}
}

func TestWaitingToClosed(t *testing.T) {
	f := NewFSM()
	_ = f.Transition(StateWaiting)
	if err := f.Transition(StateClosed); err != nil {
		t.Fatal(err)
	}
}
