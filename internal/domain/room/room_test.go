// 房间聚合根测试：占座、准备与状态机联动。
package room

import (
	"fmt"
	"testing"
)

func TestJoinAndReadyFlow(t *testing.T) {
	r := NewRoom("r1")
	if r.FSM.State() != StateWaiting {
		t.Fatalf("state=%s", r.FSM.State())
	}
	for i := 0; i < 4; i++ {
		if _, ok := r.JoinAutoSeat(fmt.Sprintf("u%d", i)); !ok {
			t.Fatalf("join seat %d", i)
		}
	}
	if _, ok := r.JoinAutoSeat("overflow"); ok {
		t.Fatal("expected full")
	}
	for i := 0; i < 4; i++ {
		if err := r.SetReady(i, true); err != nil {
			t.Fatalf("ready %d: %v", i, err)
		}
	}
	if r.FSM.State() != StateReady {
		t.Fatalf("want ready got %s", r.FSM.State())
	}
}

func TestJoinAutoSeatSameUserKeepsSeat(t *testing.T) {
	r := NewRoom("r2")
	seat0, ok := r.JoinAutoSeat("u1")
	if !ok {
		t.Fatal("first join failed")
	}
	seat1, ok := r.JoinAutoSeat("u1")
	if !ok {
		t.Fatal("second join failed")
	}
	if seat0 != seat1 {
		t.Fatalf("want same seat got %d and %d", seat0, seat1)
	}
	if r.PlayerIDs[0] != "u1" || r.PlayerIDs[1] != "" {
		t.Fatalf("unexpected seats: %#v", r.PlayerIDs)
	}
}

func TestLeaveMovesReadyRoomBackToWaiting(t *testing.T) {
	r := NewRoom("r3")
	for i := 0; i < 4; i++ {
		if _, ok := r.JoinAutoSeat(fmt.Sprintf("u%d", i)); !ok {
			t.Fatalf("join seat %d", i)
		}
		if err := r.SetReady(i, true); err != nil {
			t.Fatalf("ready %d: %v", i, err)
		}
	}
	if r.FSM.State() != StateReady {
		t.Fatalf("want ready got %s", r.FSM.State())
	}
	if err := r.Leave("u1"); err != nil {
		t.Fatal(err)
	}
	if r.FSM.State() != StateWaiting {
		t.Fatalf("want waiting got %s", r.FSM.State())
	}
	if r.PlayerIDs[1] != "" || r.Ready[1] {
		t.Fatalf("seat should be cleared: %#v %#v", r.PlayerIDs, r.Ready)
	}
}
