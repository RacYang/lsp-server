package logx

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

// Level 表示日志级别，供门面配置使用。
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger 为统一日志门面；业务代码禁止直接依赖 slog.Logger。
type Logger struct {
	lg *slog.Logger
}

var (
	defaultLoggerMu sync.RWMutex
	defaultLogger   = New(os.Stdout, LevelInfo)
)

// Options 定义日志门面的构造选项。
type Options struct {
	Format        string
	Sampling      SamplingConfig
	RedactKeys    []string
	FieldSchema   FieldSchema
	AtomicLevel   *AtomicLevel
	IncludeSource bool
}

// New 创建 Logger，默认 JSON 输出。
func New(w io.Writer, min Level) *Logger {
	return NewWithOptions(w, min, Options{Format: "json", IncludeSource: true})
}

// NewWithOptions 按选项创建 Logger。
func NewWithOptions(w io.Writer, min Level, opts Options) *Logger {
	if w == nil {
		w = os.Stdout
	}
	if opts.Format == "" {
		opts.Format = "json"
	}
	var h slog.Handler
	switch opts.Format {
	case "console":
		h = newConsoleHandler(w, min, opts.IncludeSource)
	default:
		h = newJSONSlogHandler(w, min, opts.IncludeSource)
	}
	if opts.AtomicLevel != nil {
		h = newAtomicLevelHandler(h, opts.AtomicLevel)
	}
	if len(opts.RedactKeys) > 0 {
		h = newRedactHandler(h, opts.RedactKeys)
	}
	if opts.FieldSchema.Pattern != "" || len(opts.FieldSchema.CoreKeys) > 0 {
		h = newSchemaHandler(h, opts.FieldSchema)
	}
	h = newContextHandler(h)
	if opts.Sampling.Enabled {
		h = newSamplingHandler(h, opts.Sampling)
	}
	return &Logger{lg: slog.New(h)}
}

// Default 返回当前包级默认 Logger。
func Default() *Logger {
	defaultLoggerMu.RLock()
	defer defaultLoggerMu.RUnlock()
	return defaultLogger
}

// SetDefault 替换包级默认 Logger，主要用于进程启动配置与测试。
func SetDefault(log *Logger) {
	if log == nil {
		log = New(os.Stdout, LevelInfo)
	}
	defaultLoggerMu.Lock()
	defaultLogger = log
	defaultLoggerMu.Unlock()
}

// With 创建携带固定结构化字段的子 Logger。
func (l *Logger) With(args ...any) *Logger {
	return &Logger{lg: l.lg.With(args...)}
}

// Named 创建带组件名的子 Logger。
func (l *Logger) Named(name string) *Logger {
	return l.With("logger", name)
}

// Debug 输出 Debug 级别日志。
func (l *Logger) Debug(ctx context.Context, msg string, args ...any) {
	l.lg.Log(ctx, slog.LevelDebug, msg, args...)
}

// Info 输出 Info 级别日志。
func (l *Logger) Info(ctx context.Context, msg string, args ...any) {
	l.lg.Log(ctx, slog.LevelInfo, msg, args...)
}

// Warn 输出 Warn 级别日志。
func (l *Logger) Warn(ctx context.Context, msg string, args ...any) {
	l.lg.Log(ctx, slog.LevelWarn, msg, args...)
}

// Error 输出 Error 级别日志。
func (l *Logger) Error(ctx context.Context, msg string, args ...any) {
	l.lg.Log(ctx, slog.LevelError, msg, args...)
}

// Debug 使用默认 Logger。
func Debug(ctx context.Context, msg string, args ...any) {
	Default().Debug(ctx, msg, args...)
}

// Info 使用默认 Logger。
func Info(ctx context.Context, msg string, args ...any) {
	Default().Info(ctx, msg, args...)
}

// Warn 使用默认 Logger。
func Warn(ctx context.Context, msg string, args ...any) {
	Default().Warn(ctx, msg, args...)
}

// Error 使用默认 Logger。
func Error(ctx context.Context, msg string, args ...any) {
	Default().Error(ctx, msg, args...)
}
