package logx

import (
	"context"
	"io"
	"log/slog"
	"os"
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

var defaultLogger = New(os.Stdout, LevelInfo)

// New 创建 Logger，默认 JSON 输出。
func New(w io.Writer, min Level) *Logger {
	return &Logger{lg: newJSONSlogLogger(w, min)}
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
	defaultLogger.Debug(ctx, msg, args...)
}

// Info 使用默认 Logger。
func Info(ctx context.Context, msg string, args ...any) {
	defaultLogger.Info(ctx, msg, args...)
}

// Warn 使用默认 Logger。
func Warn(ctx context.Context, msg string, args ...any) {
	defaultLogger.Warn(ctx, msg, args...)
}

// Error 使用默认 Logger。
func Error(ctx context.Context, msg string, args ...any) {
	defaultLogger.Error(ctx, msg, args...)
}
