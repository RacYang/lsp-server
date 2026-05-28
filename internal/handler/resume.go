package handler

import clientv1 "racoo.cn/lsp/api/gen/go/client/v1"

// ResumeResult 为断线重连恢复结果，供 WebSocket 登录分支下发快照与后续订阅。
type ResumeResult struct {
	UserID              string
	RoomID              string
	Resumed             bool
	Snapshot            *clientv1.SnapshotNotify
	SnapshotSinceCursor string
	// SettlementPayload 是已序列化的 Envelope proto 字节（可能为空）；
	// 仅在房间已结算但客户端重连时填充，供 handler 直接推送给客户端，无须重新序列化。
	SettlementPayload []byte
	Redirect          *clientv1.RouteRedirectNotify
}

// ResumeError 为恢复链路上的显式业务错误（而非底层传输故障）。
type ResumeError struct {
	Code    clientv1.ErrorCode
	Message string
}

func (e *ResumeError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}
