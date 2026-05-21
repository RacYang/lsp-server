package room

import domainroom "racoo.cn/lsp/internal/domain/room"

type Seat = domainroom.Seat

const (
	SeatInvalid   = domainroom.SeatInvalid
	BroadcastSeat = domainroom.BroadcastSeat
	SeatCount     = domainroom.SeatCount
)

func SeatFromInt(v int) Seat {
	return domainroom.SeatFromInt(v)
}
