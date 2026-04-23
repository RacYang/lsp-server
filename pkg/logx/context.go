// Package logx 提供统一日志门面，封装底层实现并支持从 Context 读取追踪字段。
package logx

import "context"

type ctxKey string

const (
	ctxKeyTraceID ctxKey = "logx_trace_id"
	ctxKeyUserID  ctxKey = "logx_user_id"
	ctxKeyRoomID  ctxKey = "logx_room_id"
)

// WithTraceID 将 trace_id 写入 Context，供日志门面在合并字段时读取。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ctxKeyTraceID, traceID)
}

// WithUserID 将 user_id 写入 Context。
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKeyUserID, userID)
}

// WithRoomID 将 room_id 写入 Context。
func WithRoomID(ctx context.Context, roomID string) context.Context {
	return context.WithValue(ctx, ctxKeyRoomID, roomID)
}

// TraceIDFromContext 从 Context 读取 trace_id；不存在则返回空字符串。
func TraceIDFromContext(ctx context.Context) string {
	v := ctx.Value(ctxKeyTraceID)
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// UserIDFromContext 从 Context 读取 user_id。
func UserIDFromContext(ctx context.Context) string {
	v := ctx.Value(ctxKeyUserID)
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// RoomIDFromContext 从 Context 读取 room_id。
func RoomIDFromContext(ctx context.Context) string {
	v := ctx.Value(ctxKeyRoomID)
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
