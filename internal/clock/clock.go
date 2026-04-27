// Package clock 提供可注入时间源，避免房间定时器测试依赖真实 sleep。
package clock

import (
	"sync"
	"time"
)

// Timer 表示一个可停止的定时任务。
type Timer interface {
	Stop() bool
}

// Clock 抽象当前时间与延迟回调。
type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, fn func()) Timer
}

type realClock struct{}

// NewReal 创建生产时间源。
func NewReal() Clock { return realClock{} }

func (realClock) Now() time.Time { return time.Now() }

func (realClock) AfterFunc(d time.Duration, fn func()) Timer {
	return time.AfterFunc(d, fn)
}

// Fake 是可手动推进的测试时间源。
type Fake struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	mu      sync.Mutex
	at      time.Time
	fn      func()
	stopped bool
}

// NewFake 创建测试时间源。
func NewFake(start time.Time) *Fake {
	if start.IsZero() {
		start = time.Unix(0, 0)
	}
	return &Fake{now: start}
}

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fake) AfterFunc(d time.Duration, fn func()) Timer {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := &fakeTimer{at: f.now.Add(d), fn: fn}
	f.timers = append(f.timers, t)
	return t
}

// Advance 推进测试时间，并同步触发到期回调。
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	var due []*fakeTimer
	kept := f.timers[:0]
	for _, t := range f.timers {
		t.mu.Lock()
		stopped := t.stopped
		at := t.at
		t.mu.Unlock()
		if stopped {
			continue
		}
		if !at.After(f.now) {
			t.mu.Lock()
			t.stopped = true
			t.mu.Unlock()
			due = append(due, t)
			continue
		}
		kept = append(kept, t)
	}
	f.timers = kept
	f.mu.Unlock()
	for _, t := range due {
		if t.fn != nil {
			t.fn()
		}
	}
}

func (t *fakeTimer) Stop() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}
