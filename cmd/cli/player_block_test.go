package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlayerBlockLinesGameStartedTreatsEmptySeatAsOccupied(t *testing.T) {
	view := RoomView{SeatIndex: 1, RoomState: "playing"}
	for i := range view.QueBySeat {
		view.QueBySeat[i] = -1
	}
	lines := playerBlockLines(view, 2)
	require.NotContains(t, lines[0], "空座")
	require.Contains(t, lines[0], "3号位")
	require.Contains(t, lines[1], "13张")
}

func TestPlayerBlockLinesWaitingPhaseShowsEmptySeat(t *testing.T) {
	view := RoomView{SeatIndex: 1}
	for i := range view.QueBySeat {
		view.QueBySeat[i] = -1
	}
	lines := playerBlockLines(view, 2)
	require.Contains(t, lines[0], "空座")
	require.Contains(t, lines[1], "0张")
}

func TestPlayerBlockLinesRendersMeldGlyphs(t *testing.T) {
	view := RoomView{SeatIndex: 1, RoomState: "playing"}
	for i := range view.QueBySeat {
		view.QueBySeat[i] = -1
	}
	view.QueBySeat[2] = 1
	view.Players[2] = PlayerView{Nickname: "BOT-2", HandCnt: 13, Melds: []string{"pong:p5", "gang:m1"}}
	lines := playerBlockLines(view, 2)
	require.Contains(t, lines[0], "BOT-2")
	require.Contains(t, lines[1], "缺筒")
	require.Contains(t, lines[2], "🀝 🀝 🀝 碰")
	require.Contains(t, lines[2], "🀇 🀇 🀇 🀇 杠")
}

func TestPlayerSeatedConsidersGameSignals(t *testing.T) {
	require.False(t, playerSeated(PlayerView{}))
	require.True(t, playerSeated(PlayerView{Nickname: "alice"}))
	require.True(t, playerSeated(PlayerView{HandCnt: 13}))
	require.True(t, playerSeated(PlayerView{Discards: []string{"m1"}}))
	require.True(t, playerSeated(PlayerView{Melds: []string{"pong:5p"}}))
	require.True(t, playerSeated(PlayerView{Hued: true}))
}
