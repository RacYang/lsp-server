package handler

import clientv1 "racoo.cn/lsp/api/gen/go/client/v1"

// ResumeResult 为断线重连恢复结果，供 WebSocket 登录分支下发快照与后续订阅。
type ResumeResult struct {
	UserID              string
	RoomID              string
	Resumed             bool
	Snapshot            *clientv1.SnapshotNotify
	SnapshotSinceCursor string
	Settlement          *clientv1.SettlementNotify
	Redirect            *clientv1.RouteRedirectNotify
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
