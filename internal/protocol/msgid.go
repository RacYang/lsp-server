// Package protocol — 消息类型编号常量，与 docs/PROTOCOL.md 及 Protobuf Envelope 对齐。
//
// 数值与协议文档一一对应，客户端与服务器共用同一套枚举，避免双真相源。
// Phase 2 起增加心跳、离房等房间扩展编号，字段仍由 Protobuf 承载。
package protocol

// 以下为客户端帧类型编号（载荷为 Protobuf Envelope；Phase 1 基础编号兼容延续）。
const (
	LoginReq     uint16 = 1
	LoginResp    uint16 = 2
	JoinRoomReq  uint16 = 3
	JoinRoomResp uint16 = 4
	ReadyReq     uint16 = 5
	ReadyResp    uint16 = 6
	StartGame    uint16 = 7
	DrawTile     uint16 = 8
	DiscardReq   uint16 = 9
	DiscardResp  uint16 = 10
	PongReq      uint16 = 11
	GangReq      uint16 = 12
	HuReq        uint16 = 13
	ActionNotify uint16 = 14
	Settlement   uint16 = 15
	// 以下为 Phase 2 集群扩展；与 messages.proto 中 oneof 字段配套使用。
	HeartbeatReq        uint16 = 16
	HeartbeatResp       uint16 = 17
	LeaveRoomReq        uint16 = 18
	LeaveRoomResp       uint16 = 19
	RouteRedirectNotify uint16 = 20
	// 21..26 reserved: 已废弃的开局专用换三/缺门请求与应答编号，保留占位以兼容客户端。
	// 使用 OpeningActionReq/Resp 与 OpeningDone 替代。
	// SnapshotNotify 为 Phase 3 重连恢复下发的房间快照通知。
	SnapshotNotify uint16 = 27
	// Phase 4 交互闭环动作响应。
	PongResp uint16 = 28
	GangResp uint16 = 29
	HuResp   uint16 = 30
	// InitialDealNotify 为玩家客户端开局私有手牌通知。
	InitialDealNotify uint16 = 31
	// 大厅交互消息：房间列表、自动匹配与显式创建。
	ListRoomsReq   uint16 = 32
	ListRoomsResp  uint16 = 33
	AutoMatchReq   uint16 = 34
	AutoMatchResp  uint16 = 35
	CreateRoomReq  uint16 = 36
	CreateRoomResp uint16 = 37
	PassReq        uint16 = 38
	PassResp       uint16 = 39
	RenameReq      uint16 = 40
	RenameResp     uint16 = 41
	AddBotReq      uint16 = 42
	AddBotResp     uint16 = 43
	ListRulesReq   uint16 = 44
	ListRulesResp  uint16 = 45
	ChiReq         uint16 = 46
	ChiResp        uint16 = 47
	// OpeningActionReq/Resp 是所有规则开局动作的统一提交入口。
	OpeningActionReq  uint16 = 48
	OpeningActionResp uint16 = 49
	OpeningDone       uint16 = 50
)
