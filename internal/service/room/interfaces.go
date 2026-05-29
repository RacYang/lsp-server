package room

import "context"

// CommandHandler 是房间游戏命令接口；gateway/adapter 调用此子集驱动局内状态机。
// 声明在 service/room 包，由 Service 实现，供注入测试用替身。
type CommandHandler interface {
	Join(ctx context.Context, roomID, userID string) (int, error)
	Ready(ctx context.Context, roomID, userID string) ([]Notification, error)
	Leave(ctx context.Context, roomID, userID string) error
	Discard(ctx context.Context, roomID, userID, tile string, tok *PhaseToken) ([]Notification, error)
	Pong(ctx context.Context, roomID, userID string, tok *PhaseToken) ([]Notification, error)
	Chi(ctx context.Context, roomID, userID string, tiles []string, tok *PhaseToken) ([]Notification, error)
	Gang(ctx context.Context, roomID, userID, tile string, tok *PhaseToken) ([]Notification, error)
	Hu(ctx context.Context, roomID, userID string, tok *PhaseToken) ([]Notification, error)
	Pass(ctx context.Context, roomID, userID string, tok *PhaseToken) ([]Notification, error)
	OpeningAction(ctx context.Context, roomID, userID, action string, tiles []string, direction, suit int32, params map[string]string, tok *PhaseToken) ([]Notification, error)
	AutoTimeout(ctx context.Context, roomID string) ([]Notification, error)
	MarkSeatOffline(roomID, userID string)
	CancelOfflineSurrender(roomID, userID string)
}

// RoomQueries 是房间状态查询接口；适配层与持久化层调用此子集获取局面快照。
type RoomQueries interface {
	RoundView(ctx context.Context, roomID string) (RoundView, bool, error)
	RoomSnapshot(roomID string) (playerIDs []string, fsmState string, ready [4]bool, ok bool)
	PlayerIDs(roomID string) ([4]string, bool)
	ActiveRoomCount() int
	RuleID() string
}

// RoomRecovery 是房间恢复接口；仅在节点重启时调用，与正常命令流解耦。
type RoomRecovery interface {
	EnsureRoom(roomID string) error
	RecoverRoom(roomID string, playerIDs []string, fsmState string, roundPersistJSON []byte) error
	RoundPersistSnapshot(ctx context.Context, roomID string) ([]byte, error)
}

// RoomService 组合 CommandHandler 与 RoomQueries，供需要完整房间运行时访问的调用方使用。
type RoomService interface {
	CommandHandler
	RoomQueries
}
