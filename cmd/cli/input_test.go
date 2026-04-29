package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandHandlerHelpQuitAndUnknown(t *testing.T) {
	state := NewAppState("我")
	handler := NewCommandHandler(NewWSClient("ws://127.0.0.1/ws", "我", "", "", false, state), state)
	require.True(t, handler.Handle(context.Background(), "help"))
	require.False(t, handler.Handle(context.Background(), "quit"))
	require.True(t, handler.Handle(context.Background(), "badcmd"))
	view := state.Snapshot()
	require.NotEmpty(t, view.Log)
	require.Contains(t, view.Log[len(view.Log)-1].Text, "未知命令")
}

func TestCommandHandlerExchangeMutatesLocalHandBeforeSend(t *testing.T) {
	state := NewAppState("我")
	state.Mutate(func(v *RoomView) {
		v.SeatIndex = 0
		v.Players[0].Hand = []string{"m1", "m2", "m3", "p9"}
		v.Players[0].HandCnt = 4
	})
	handler := NewCommandHandler(NewWSClient("ws://127.0.0.1/ws", "我", "", "", false, state), state)
	require.True(t, handler.Handle(context.Background(), "ex m1 m2 m3 3"))
	view := state.Snapshot()
	require.Equal(t, []string{"p9"}, view.Players[0].Hand)
}

func TestDiscardIndexIgnoresOutOfRange(t *testing.T) {
	state := NewAppState("我")
	state.Mutate(func(v *RoomView) {
		v.SeatIndex = 0
		v.Players[0].Hand = []string{"m1"}
	})
	handler := NewCommandHandler(NewWSClient("ws://127.0.0.1/ws", "我", "", "", false, state), state)
	handler.DiscardIndex(context.Background(), 9)
	require.Equal(t, []string{"m1"}, state.Snapshot().Players[0].Hand)
}
