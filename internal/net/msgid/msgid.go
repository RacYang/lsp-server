// Package msgid 定义二进制帧头中的 msg_id 常量，与 docs/PROTOCOL.md 及 Protobuf Envelope 对齐。
//
// 数值与协议文档一一对应，客户端与服务器共用同一套枚举，避免双真相源。
package msgid

// 以下为 Phase 1 客户端帧类型编号（载荷仍为 Protobuf）。
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
)
