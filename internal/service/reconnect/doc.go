// Package reconnect 处理断线重连业务逻辑（ADR-0050 L4 服务层）。
//
// 职责：会话令牌校验、内存房间状态读取、重连决策。
// 禁止：proto 类型投影、传输层引用、handler/adapter/gateway 依赖。
package reconnect
