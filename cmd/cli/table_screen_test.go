package main

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func TestRoomPrepEnterReadiesWhenSeatsFull(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.SeatIndex = 0
		v.RoomState = "waiting"
		v.Players[0] = PlayerView{UserID: "u0", Nickname: "racoo", Online: true}
		for seat := 1; seat < 4; seat++ {
			v.Players[seat] = PlayerView{UserID: "bot", Nickname: "机器人", Ready: true, IsBot: true, Status: "ready"}
		}
	})
	gateway := &fakeTableGateway{ready: make(chan struct{}, 1)}

	result := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyEnter, 0, 0),
		state, gateway, &HandCursor{}, &OverlayState{}, nil, nil, nil)

	require.Nil(t, result.exit)
	select {
	case <-gateway.ready:
	case <-time.After(time.Second):
		t.Fatal("Enter should send Ready when room prep seats are full")
	}
	require.Eventually(t, func() bool {
		view := state.Snapshot()
		return view.Players[0].Ready && view.Players[0].Status == "ready"
	}, time.Second, 10*time.Millisecond)
}

func TestRoomPrepEnterDoesNothingWhenSeatsEmpty(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.SeatIndex = 0
		v.RoomState = "waiting"
		v.Players[0] = PlayerView{UserID: "u0", Nickname: "racoo", Online: true}
	})
	gateway := &fakeTableGateway{ready: make(chan struct{}, 1)}

	result := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyEnter, 0, 0),
		state, gateway, &HandCursor{}, &OverlayState{}, nil, nil, nil)

	require.Nil(t, result.exit)
	select {
	case <-gateway.ready:
		t.Fatal("Enter should not ready while seats are still empty")
	default:
	}
}

func TestTableSceneLeaveRoomReturnsToLobbyWithoutQuittingApp(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "room-1"
		v.SeatIndex = 0
		v.RoomState = "playing"
		v.Players[0] = PlayerView{UserID: "u0", Nickname: "racoo", Online: true}
	})
	scene := NewTableScene(state, nil, &fakeTableGateway{}, nil)

	scene.HandleKey(context.Background(), tcell.NewEventKey(tcell.KeyRune, 'q', 0))

	require.False(t, scene.ShouldQuit())
	view := state.Snapshot()
	require.Equal(t, phaseLobby, view.Phase)
	require.Empty(t, view.RoomID)
	require.Equal(t, "room-1", view.PendingLeaveRoomID)
}

type fakeTableGateway struct {
	ready chan struct{}
}

func (g *fakeTableGateway) Ready(context.Context) error {
	if g.ready != nil {
		g.ready <- struct{}{}
	}
	return nil
}

func (g *fakeTableGateway) Discard(context.Context, string) error { return nil }

func (g *fakeTableGateway) ExchangeThree(context.Context, []string, int32) error { return nil }

func (g *fakeTableGateway) QueMen(context.Context, int32) error { return nil }

func (g *fakeTableGateway) Pong(context.Context) error { return nil }

func (g *fakeTableGateway) Gang(context.Context, string) error { return nil }

func (g *fakeTableGateway) Hu(context.Context) error { return nil }

func (g *fakeTableGateway) Pass(context.Context) error { return nil }

func (g *fakeTableGateway) LeaveRoom(context.Context) error { return nil }

func (g *fakeTableGateway) AddBot(context.Context, int32) ([]*clientv1.SeatInfo, error) {
	return nil, nil
}
