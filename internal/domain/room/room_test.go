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
