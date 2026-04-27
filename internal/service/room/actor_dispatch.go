package room

import (
	"context"
	"fmt"

	domainroom "racoo.cn/lsp/internal/domain/room"
)

func (a *roomActor) doJoin(userID string) (int, error) {
	if a.room == nil {
		return -1, fmt.Errorf("nil room")
	}
	seat, ok := a.room.JoinAutoSeat(userID)
	if !ok {
		return -1, fmt.Errorf("room full")
	}
	return seat, nil
}

func (a *roomActor) doReady(userID string) ([]Notification, error) {
	if a.room == nil {
		return nil, fmt.Errorf("nil room")
	}
	r := a.room
	seat := -1
	for i := 0; i < 4; i++ {
		if r.PlayerIDs[i] == userID {
			seat = i
			break
		}
	}
	if seat < 0 {
		return nil, fmt.Errorf("not in room")
	}
	if err := r.SetReady(seat, true); err != nil {
		return nil, err
	}
	if r.FSM.State() == domainroom.StateReady {
		if err := r.StartPlaying(); err != nil {
			return nil, err
		}
		if a.engine == nil {
			return nil, fmt.Errorf("nil engine")
		}
		if a.round != nil {
			return nil, nil
		}
		round, notifications, err := a.engine.StartRound(context.Background(), r.ID, r.PlayerIDs)
		if err != nil {
			return nil, err
		}
		a.round = round
		if a.round != nil && a.round.closed {
			a.closeRoomAfterRound()
		}
		return notifications, nil
	}
	return nil, nil
}

func (a *roomActor) doLeave(userID string) error {
	if a.room == nil {
		return fmt.Errorf("nil room")
	}
	return a.room.Leave(userID)
}

func (a *roomActor) doDiscard(userID, tile string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyDiscard(context.Background(), a.round, seat, tile)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doPong(userID string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyPong(context.Background(), a.round, seat)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doGang(userID, tile string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyGang(context.Background(), a.round, seat, tile)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doHu(userID string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyHu(context.Background(), a.round, seat)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doAutoTimeout() ([]Notification, error) {
	notifications, err := a.engine.ApplyTimeout(context.Background(), a.round)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doExchangeThree(userID string, tiles []string, direction int32) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	return a.engine.ApplyExchangeThree(context.Background(), a.round, seat, tiles, direction)
}

func (a *roomActor) doQueMen(userID string, suit int32) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	return a.engine.ApplyQueMen(context.Background(), a.round, seat, suit)
}

func (a *roomActor) seatOf(userID string) (int, error) {
	if a.room == nil {
		return -1, fmt.Errorf("nil room")
	}
	if a.round == nil {
		return -1, fmt.Errorf("round not started")
	}
	for i := 0; i < 4; i++ {
		if a.room.PlayerIDs[i] == userID {
			return i, nil
		}
	}
	return -1, fmt.Errorf("not in room")
}

func (a *roomActor) closeRoomAfterRound() {
	if a == nil || a.room == nil {
		return
	}
	_ = a.room.CloseToSettling()
	_ = a.room.CloseRoom()
}
