package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPondDirForSeat(t *testing.T) {
	require.Equal(t, PondUp, PondDirForSeat(SeatPosBottom))
	require.Equal(t, PondDown, PondDirForSeat(SeatPosTop))
	require.Equal(t, PondRight, PondDirForSeat(SeatPosLeft))
	require.Equal(t, PondLeft, PondDirForSeat(SeatPosRight))
}

func TestRotateForSeatBottomAndTop(t *testing.T) {
	frame := Region{X: 10, Y: 2, Width: 64, Height: 32}
	base := Region{X: 20, Y: 28, Width: 10, Height: 1}
	require.Equal(t, base, RotateForSeat(base, frame, SeatPosBottom))
	top := RotateForSeat(base, frame, SeatPosTop)
	require.Equal(t, 54, top.X)
	require.Equal(t, 7, top.Y)
	require.Equal(t, base.Width, top.Width)
	require.Equal(t, base.Height, top.Height)
}

func TestRelativeSeatAllSelfSeats(t *testing.T) {
	for self := int32(0); self < 4; self++ {
		require.Equal(t, SeatPosBottom, RelativeSeat(self, self))
		require.Equal(t, SeatPosLeft, RelativeSeat(self, (self+1)%4))
		require.Equal(t, SeatPosTop, RelativeSeat(self, (self+2)%4))
		require.Equal(t, SeatPosRight, RelativeSeat(self, (self+3)%4))
	}
}
