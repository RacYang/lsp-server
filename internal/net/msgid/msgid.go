// Package msgid 定义二进制帧头中的 msg_id 常量，与 docs/PROTOCOL.md 及 Protobuf Envelope 对齐。
//
// 数值与协议文档一一对应，客户端与服务器共用同一套枚举，避免双真相源。
// Phase 2 起增加心跳、离房、换三张、定缺等四川血战流程相关编号，字段仍由 Protobuf 承载。
package msgid

// 以下为客户端帧类型编号（载荷为 Protobuf Envelope；血战流程与 Phase 1 基础编号兼容延续）。
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
	// 以下为 Phase 2 集群与四川血战扩展；与 messages.proto 中 oneof 字段配套使用。
	HeartbeatReq        uint16 = 16
	HeartbeatResp       uint16 = 17
	LeaveRoomReq        uint16 = 18
	LeaveRoomResp       uint16 = 19
	RouteRedirectNotify uint16 = 20
	ExchangeThreeReq    uint16 = 21
	ExchangeThreeResp   uint16 = 22
	ExchangeThreeDone   uint16 = 23
	QueMenReq           uint16 = 24
	QueMenResp          uint16 = 25
	QueMenDone          uint16 = 26
	// SnapshotNotify 为 Phase 3 重连恢复下发的房间快照通知。
	SnapshotNotify uint16 = 27
	// Phase 4 交互闭环动作响应。
	PongResp uint16 = 28
	GangResp uint16 = 29
	HuResp   uint16 = 30
)
