package logx

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// SamplingConfig 定义日志采样窗口；默认关闭。
type SamplingConfig struct {
	Enabled           bool
	Initial           int
	Thereafter        int
	Tick              time.Duration
	ErrorNeverSampled bool
}

type samplingHandler struct {
	next slog.Handler
	cfg  SamplingConfig
	mu   sync.Mutex
	win  time.Time
	seen int
}

func newSamplingHandler(next slog.Handler, cfg SamplingConfig) slog.Handler {
	if cfg.Tick <= 0 {
		cfg.Tick = time.Second
	}
	return &samplingHandler{next: next, cfg: cfg}
}

func (h *samplingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *samplingHandler) Handle(ctx context.Context, rec slog.Record) error {
	if !h.shouldLog(rec) {
		return nil
	}
	return h.next.Handle(ctx, rec)
}

func (h *samplingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &samplingHandler{next: h.next.WithAttrs(attrs), cfg: h.cfg}
}

func (h *samplingHandler) WithGroup(name string) slog.Handler {
	return &samplingHandler{next: h.next.WithGroup(name), cfg: h.cfg}
}

func (h *samplingHandler) shouldLog(rec slog.Record) bool {
	if !h.cfg.Enabled {
		return true
	}
	if h.cfg.ErrorNeverSampled && rec.Level >= slog.LevelError {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := rec.Time
	if now.IsZero() {
		now = time.Now()
	}
	if h.win.IsZero() || now.Sub(h.win) >= h.cfg.Tick {
		h.win = now
		h.seen = 0
	}
	h.seen++
	if h.cfg.Initial <= 0 {
		return h.cfg.Thereafter <= 0 || h.seen%h.cfg.Thereafter == 0
	}
	if h.seen <= h.cfg.Initial {
		return true
	}
	return h.cfg.Thereafter > 0 && (h.seen-h.cfg.Initial)%h.cfg.Thereafter == 0
}
