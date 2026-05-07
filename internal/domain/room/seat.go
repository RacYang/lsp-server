package room

// Seat 表示房间内固定座位号，正常取值为 0..3。
type Seat int32

const (
	// SeatInvalid 表示没有有效座位。
	SeatInvalid Seat = -1
	// BroadcastSeat 表示通知面向房间内所有座位广播。
	BroadcastSeat Seat = -1
	// SeatCount 是一桌固定座位数量。
	SeatCount = 4
)

// Proto 返回协议层使用的 int32 座位号。
func (s Seat) Proto() int32 {
	return int32(s)
}

// SeatFromProto 从协议层座位号恢复领域座位类型。
func SeatFromProto(v int32) Seat {
	return Seat(v)
}

// SeatFromInt 从本地固定四人桌循环下标恢复领域座位类型。
func SeatFromInt(v int) Seat {
	return Seat(v) //nolint:gosec // G115：调用方只传入本地四人桌 0..3 或 -1 哨兵值
}

// Valid 判断座位是否在当前四人桌范围内。
func (s Seat) Valid() bool {
	return s >= 0 && int(s) < SeatCount
}
