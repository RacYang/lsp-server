// Package logx 是日志门面，统一处理字段注入、采样与 PII 脱敏。
//
// 职责：封装 slog 后端，提供 Info/Warn/Error/Debug 入口；通过 Context 自动注入
// trace_id、user_id、room_id；向下屏蔽底层日志库细节。
// 禁止业务代码绕过本包直接 import log/slog 或 go.uber.org/zap。
package logx
