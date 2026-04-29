package logx

import (
	"log/slog"
	"reflect"
)

// Err 将 error 统一编码为结构化日志字段值。
func Err(err error) any {
	if err == nil {
		return nil
	}
	t := reflect.TypeOf(err)
	return slog.GroupValue(
		slog.String("message", err.Error()),
		slog.String("type", t.String()),
	)
}
