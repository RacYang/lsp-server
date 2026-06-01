// Package contract 定义 handler、gateway、adapter 之间的共享契约类型，
// 解耦 handler（传输层）与实现层（adapter/local、gateway/remote）。
package contract

import (
	"context"
	"errors"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// ErrRateLimited 表示网关动作被限流拒绝；adapter 实现在触发限流时返回此错误。
var ErrRateLimited = errors.New("rate limited")

// PhaseUpdater 由 PhaseDriftError 实现，供 handler 在不引用 engine 类型的情况下提取 PhaseUpdate。
type PhaseUpdater interface {
	PhaseUpdate() *clientv1.PhaseUpdate
}

// ResumeResult 为断线重连恢复结果，供 WebSocket 登录分支下发快照与后续订阅。
type ResumeResult struct {
	UserID              string
	RoomID              string
	Resumed             bool
	Snapshot            *clientv1.SnapshotNotify
	SnapshotSinceCursor string
	// SettlementPayload 是已序列化的 Envelope proto 字节（可能为空）；
	// 仅在房间已结算但客户端重连时填充，供 handler 直接推送给客户端。
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

// RoomGateway 抽象本地房间服务或远程 room/lobby gRPC 协调器。
// 定义在 contract 包，handler 与 adapter/local、gateway/remote 均引用此接口，
// 避免 adapter/gateway → handler 的反向依赖。
type RoomGateway interface {
	Join(ctx context.Context, roomID, userID string) (int, error)
	Ready(ctx context.Context, roomID, userID string) (func(), error)
	Leave(ctx context.Context, roomID, userID string) (func(), error)
	MarkSeatOffline(ctx context.Context, roomID, userID string) error
	CancelOfflineSurrender(ctx context.Context, roomID, userID string) error
	OpeningAction(ctx context.Context, roomID, userID, action string, tiles []string, direction, suit int32, params map[string]string, tok *clientv1.PhaseToken) (func(), error)
	Discard(ctx context.Context, roomID, userID, tile string, tok *clientv1.PhaseToken) (func(), error)
	Pong(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error)
	Chi(ctx context.Context, roomID, userID string, tiles []string, tok *clientv1.PhaseToken) (func(), error)
	Gang(ctx context.Context, roomID, userID, tile string, tok *clientv1.PhaseToken) (func(), error)
	Hu(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error)
	Pass(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error)
	ListRooms(ctx context.Context, pageSize int32, pageToken string) ([]*clientv1.RoomMeta, string, error)
	ListRules(ctx context.Context) ([]*clientv1.RuleMeta, error)
	AutoMatch(ctx context.Context, ruleID, userID string, padWithBots bool) (string, int, error)
	CreateRoom(ctx context.Context, ruleID, displayName string, private bool, userID string) (string, int, error)
	AddBot(ctx context.Context, roomID, userID string, count int32, difficulty, opID string) ([]*clientv1.SeatInfo, func(), error)
	Resume(ctx context.Context, sessionToken string) (*ResumeResult, error)
	EnsureRoomEventSubscription(ctx context.Context, roomID, sinceCursor string) error
}
