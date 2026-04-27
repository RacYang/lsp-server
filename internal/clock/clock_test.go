package clock

import (
	"testing"
	"time"
)

func TestFakeClockAdvanceAndStop(t *testing.T) {
	start := time.Unix(10, 0)
	fc := NewFake(start)
	if !fc.Now().Equal(start) {
		t.Fatalf("now mismatch: %v", fc.Now())
	}
	fired := 0
	stopped := fc.AfterFunc(time.Second, func() { fired++ })
	if !stopped.Stop() {
		t.Fatal("first stop should succeed")
	}
	if stopped.Stop() {
		t.Fatal("second stop should fail")
	}
	fc.Advance(time.Second)
	if fired != 0 {
		t.Fatalf("stopped timer fired: %d", fired)
	}

	fc.AfterFunc(time.Second, func() { fired++ })
	fc.Advance(500 * time.Millisecond)
	if fired != 0 {
		t.Fatalf("timer fired too early: %d", fired)
	}
	fc.Advance(500 * time.Millisecond)
	if fired != 1 {
		t.Fatalf("timer should fire once: %d", fired)
	}
}

func TestFakeClockZeroStartAndNilTimer(t *testing.T) {
	fc := NewFake(time.Time{})
	if !fc.Now().Equal(time.Unix(0, 0)) {
		t.Fatalf("zero start should normalize: %v", fc.Now())
	}
	var timer *fakeTimer
	if timer.Stop() {
		t.Fatal("nil timer stop should fail")
	}
}
