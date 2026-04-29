package logx

import (
	"context"
	"log/slog"
	"sync"
)

// Entry 表示 Observer 捕获的一条日志。
type Entry struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

// Observer 为测试提供并发安全的日志捕获器。
type Observer struct {
	mu      sync.Mutex
	entries []Entry
	events  chan Entry
}

// NewObserver 创建测试用 Logger 与捕获器。
func NewObserver() (*Observer, *Logger) {
	obs := &Observer{events: make(chan Entry, 1024)}
	return obs, &Logger{lg: slog.New(newContextHandler(obs))}
}

// Take 等待并返回最多 n 条日志。
func (o *Observer) Take(n int) []Entry {
	if n <= 0 {
		return nil
	}
	out := make([]Entry, 0, n)
	for len(out) < n {
		entry, ok := <-o.events
		if !ok {
			break
		}
		out = append(out, entry)
	}
	return out
}

// Drain 返回当前已捕获的全部日志快照。
func (o *Observer) Drain() []Entry {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Entry, len(o.entries))
	copy(out, o.entries)
	return out
}

func (o *Observer) Enabled(context.Context, slog.Level) bool {
	return true
}

func (o *Observer) Handle(_ context.Context, rec slog.Record) error {
	entry := Entry{Level: rec.Level, Message: rec.Message, Attrs: map[string]any{}}
	rec.Attrs(func(attr slog.Attr) bool {
		entry.Attrs[attr.Key] = attr.Value.Any()
		return true
	})
	o.mu.Lock()
	o.entries = append(o.entries, entry)
	o.mu.Unlock()
	select {
	case o.events <- entry:
	default:
	}
	return nil
}

func (o *Observer) WithAttrs(attrs []slog.Attr) slog.Handler {
	return observerWithAttrs{base: o, attrs: attrs}
}

func (o *Observer) WithGroup(name string) slog.Handler {
	return observerWithGroup{base: o, group: name}
}

type observerWithAttrs struct {
	base  *Observer
	attrs []slog.Attr
}

func (h observerWithAttrs) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h observerWithAttrs) Handle(ctx context.Context, rec slog.Record) error {
	rec = rec.Clone()
	rec.AddAttrs(h.attrs...)
	return h.base.Handle(ctx, rec)
}

func (h observerWithAttrs) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := append([]slog.Attr{}, h.attrs...)
	next = append(next, attrs...)
	return observerWithAttrs{base: h.base, attrs: next}
}

func (h observerWithAttrs) WithGroup(name string) slog.Handler {
	return observerWithGroup{base: h.base, group: name}
}

type observerWithGroup struct {
	base  *Observer
	group string
}

func (h observerWithGroup) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h observerWithGroup) Handle(ctx context.Context, rec slog.Record) error {
	return h.base.Handle(ctx, rec)
}

func (h observerWithGroup) WithAttrs(attrs []slog.Attr) slog.Handler {
	return observerWithAttrs{base: h.base, attrs: attrs}
}

func (h observerWithGroup) WithGroup(name string) slog.Handler {
	return observerWithGroup{base: h.base, group: h.group + "." + name}
}
