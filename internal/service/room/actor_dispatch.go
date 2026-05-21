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
	if a.room.FSM != nil {
		switch a.room.FSM.State() {
		case domainroom.StatePlaying, domainroom.StateSettling, domainroom.StateClosed:
			for seat := 0; seat < 4; seat++ {
				if a.room.PlayerIDs[seat] == userID && userID != "" {
					return seat, nil
				}
			}
			return -1, fmt.Errorf("room already started")
		}
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
	if err := r.SetReady(domainroom.Seat(seat), true); err != nil {
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
	if !a.allowLeaveDuringPlay && a.room.FSM != nil && a.room.FSM.State() == domainroom.StatePlaying {
		return fmt.Errorf("room not leaveable in state %s", a.room.FSM.State())
	}
	seat := -1
	for i := 0; i < 4; i++ {
		if a.room.PlayerIDs[i] == userID {
			seat = i
			break
		}
	}
	if err := a.room.Leave(userID); err != nil {
		return err
	}
	if seat >= 0 && a.room.Surrendered[seat] && a.round != nil {
		if len(a.round.surrendered) < 4 {
			a.round.surrendered = make([]bool, 4)
		}
		a.round.surrendered[seat] = true
	}
	return nil
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
	notifications, err := a.engine.ApplyPongByPlayer(context.Background(), a.round, seat)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doChi(userID string, tiles []string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyChi(context.Background(), a.round, seat, tiles)
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

func (a *roomActor) doPass(userID string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyPass(context.Background(), a.round, seat)
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
	a.syncSurrenderedFromRound()
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) syncSurrenderedFromRound() {
	if a == nil || a.room == nil || a.round == nil {
		return
	}
	for seat := 0; seat < 4 && seat < len(a.round.surrendered); seat++ {
		if a.round.surrendered[seat] {
			a.room.Surrendered[seat] = true
		}
	}
}

func (a *roomActor) doOpeningAction(userID, action string, tiles []string, direction, suit int32, params map[string]string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	return a.engine.ApplyOpeningActionByPlayer(context.Background(), a.round, seat, action, tiles, direction, suit, params)
}

func (a *roomActor) seatOf(userID string) (Seat, error) {
	if a.room == nil {
		return SeatInvalid, fmt.Errorf("nil room")
	}
	if a.round == nil {
		return SeatInvalid, fmt.Errorf("round not started")
	}
	for i := 0; i < 4; i++ {
		if a.room.PlayerIDs[i] == userID {
			return Seat(i), nil
		}
	}
	return SeatInvalid, fmt.Errorf("not in room")
}

func (a *roomActor) closeRoomAfterRound() {
	if a == nil || a.room == nil {
		return
	}
	if a.round != nil {
		var scores [4]int32
		for seat, score := range seatBalancesFromScoreEvents(a.round.scoreEvents) {
			if seat < len(scores) {
				scores[seat] = score
			}
		}
		a.room.AddRoundScores(scores)
	}
	_ = a.room.CloseToSettling()
	if a.room.CanStartNextHand() {
		_ = a.room.PrepareNextHand()
		return
	}
	_ = a.room.CloseRoom()
}
