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

func seatsToPersist(in []Seat) []int {
	out := make([]int, 0, len(in))
	for _, seat := range in {
		out = append(out, int(seat))
	}
	return out
}

func seatsFromPersist(in []int) []Seat {
	out := make([]Seat, 0, len(in))
	for _, seat := range in {
		out = append(out, SeatFromInt(seat))
	}
	return out
}
