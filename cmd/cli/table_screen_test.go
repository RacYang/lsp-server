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

func TestPlayerJourney_L9_1_QDoesNotLeaveRoom(t *testing.T) {
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
	require.Equal(t, phaseTable, view.Phase)
	require.Equal(t, "room-1", view.RoomID)
	require.Empty(t, view.PendingLeaveRoomID)
}

func TestQueMenUsesSelectionCursorAndEnter(t *testing.T) {
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.SeatIndex = 0
		v.ActingSeat = 0
		v.RoomState = "playing"
		v.WaitingAction = "que_men"
		v.Players[0] = PlayerView{UserID: "u0", Nickname: "racoo", Online: true}
		v.Players[0].Hand = []string{"m1", "p1", "p2", "s1", "s2"}
		for i := range v.QueBySeat {
			v.QueBySeat[i] = -1
		}
	})
	gateway := &fakeTableGateway{openingActions: make(chan fakeOpeningAction, 1)}
	cursor := &HandCursor{}

	result := handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyRight, 0, 0),
		state, gateway, cursor, &OverlayState{}, nil, nil, nil)
	require.Nil(t, result.exit)
	require.Equal(t, CursorModeQueMen, cursor.Mode)
	require.Equal(t, 1, cursor.Index)

	result = handleTableKey(context.Background(), tcell.NewEventKey(tcell.KeyEnter, 0, 0),
		state, gateway, cursor, &OverlayState{}, nil, nil, nil)
	require.Nil(t, result.exit)
	select {
	case got := <-gateway.openingActions:
		require.Equal(t, ActionQueMen, got.action)
		require.EqualValues(t, 1, got.suit)
	case <-time.After(time.Second):
		t.Fatal("Enter should submit selected que men suit")
	}
}

type fakeTableGateway struct {
	ready          chan struct{}
	openingActions chan fakeOpeningAction
}

type fakeOpeningAction struct {
	action    PlayerAction
	tiles     []string
	direction int32
	suit      int32
}

func (g *fakeTableGateway) Ready(context.Context) error {
	if g.ready != nil {
		g.ready <- struct{}{}
	}
	return nil
}

func (g *fakeTableGateway) Discard(context.Context, string) error { return nil }

func (g *fakeTableGateway) OpeningAction(_ context.Context, action PlayerAction, tiles []string, direction, suit int32) error {
	if g.openingActions != nil {
		g.openingActions <- fakeOpeningAction{
			action:    action,
			tiles:     append([]string(nil), tiles...),
			direction: direction,
			suit:      suit,
		}
	}
	return nil
}

func (g *fakeTableGateway) Pong(context.Context) error { return nil }

func (g *fakeTableGateway) Chi(context.Context, []string) error { return nil }

func (g *fakeTableGateway) Gang(context.Context, string) error { return nil }

func (g *fakeTableGateway) Hu(context.Context) error { return nil }

func (g *fakeTableGateway) Pass(context.Context) error { return nil }

func (g *fakeTableGateway) LeaveRoom(context.Context) error { return nil }

func (g *fakeTableGateway) AddBot(context.Context, int32) ([]*clientv1.SeatInfo, error) {
	return nil, nil
}
