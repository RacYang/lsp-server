package lobby

import "context"

// RoomRecord 是持久化到外部存储的房间完整快照，含座位分配。
// 定义在 domain 层，service/lobby 与 store/redis 均依赖此包，不再互相引用。
type RoomRecord struct {
	RoomID      string           `json:"room_id"`
	NodeID      string           `json:"node_id"`
	RuleID      string           `json:"rule_id"`
	DisplayName string           `json:"display_name"`
	Private     bool             `json:"private"`
	CreatedAtMs int64            `json:"created_at_ms"`
	MaxSeats    int32            `json:"max_seats"`
	Seats       map[string]int32 `json:"seats"`
}

// RoomRegistry 是大厅房间注册表的接口，由 store 层实现、service 层消费。
// 实现方负责序列化；nil 注册表退化为纯内存模式（与未配置 Redis 时等价）。
type RoomRegistry interface {
	UpsertRoom(ctx context.Context, rec RoomRecord) error
	DeleteRoom(ctx context.Context, roomID string) error
	ListAll(ctx context.Context) ([]RoomRecord, error)
}
