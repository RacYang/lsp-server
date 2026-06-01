package actor

import (
	"context"
	"fmt"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/pkg/logx"
)

func (a *Actor) doJoin(userID string) (int, error) {
	if a.Room == nil {
		return -1, fmt.Errorf("nil room")
	}
	if a.Room.FSM != nil {
		switch a.Room.FSM.State() {
		case domainroom.StatePlaying, domainroom.StateSettling, domainroom.StateClosed:
			for seat := 0; seat < 4; seat++ {
				if a.Room.PlayerIDs[seat] == userID && userID != "" {
					return seat, nil
				}
			}
			return -1, fmt.Errorf("room already started")
		}
	}
	seat, ok := a.Room.JoinAutoSeat(userID)
	if !ok {
		return -1, ErrRoomFull
	}
	return seat, nil
}

func (a *Actor) doReady(userID string) ([]Notification, error) {
	if a.Room == nil {
		return nil, fmt.Errorf("nil room")
	}
	r := a.Room
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
		if a.round != nil && a.round.IsClosed() {
			a.closeRoomAfterRound()
		}
		return notifications, nil
	}
	return nil, nil
}

func (a *Actor) doLeave(userID string) error {
	if a.Room == nil {
		return fmt.Errorf("nil room")
	}
	if !a.allowLeaveDuringPlay && a.Room.FSM != nil && a.Room.FSM.State() == domainroom.StatePlaying {
		return fmt.Errorf("room not leaveable in state %s", a.Room.FSM.State())
	}
	seat := -1
	for i := 0; i < 4; i++ {
		if a.Room.PlayerIDs[i] == userID {
			seat = i
			break
		}
	}
	if err := a.Room.Leave(userID); err != nil {
		return err
	}
	if seat >= 0 && a.Room.Surrendered[seat] && a.round != nil {
		a.round.MarkSeatSurrendered(Seat(seat))
	}
	return nil
}

func (a *Actor) doDiscard(userID, tile string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyDiscard(context.Background(), a.round, seat, tile)
	if err != nil {
		return nil, err
	}
	if a.round.IsClosed() {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *Actor) doPong(userID string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyPongByPlayer(context.Background(), a.round, seat)
	if err != nil {
		return nil, err
	}
	if a.round.IsClosed() {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *Actor) doChi(userID string, tiles []string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyChi(context.Background(), a.round, seat, tiles)
	if err != nil {
		return nil, err
	}
	if a.round.IsClosed() {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *Actor) doGang(userID, tile string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyGang(context.Background(), a.round, seat, tile)
	if err != nil {
		return nil, err
	}
	if a.round.IsClosed() {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *Actor) doHu(userID string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyHu(context.Background(), a.round, seat)
	if err != nil {
		return nil, err
	}
	if a.round.IsClosed() {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *Actor) doPass(userID string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyPass(context.Background(), a.round, seat)
	if err != nil {
		return nil, err
	}
	if a.round.IsClosed() {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *Actor) doAutoTimeout() ([]Notification, error) {
	notifications, err := a.engine.ApplyTimeout(context.Background(), a.round)
	if err != nil {
		return nil, err
	}
	a.syncSurrenderedFromRound()
	if a.round.IsClosed() {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *Actor) syncSurrenderedFromRound() {
	if a == nil || a.Room == nil || a.round == nil {
		return
	}
	for seat := 0; seat < int(SeatCount); seat++ {
		if a.round.SurrenderedAt(seat) {
			a.Room.Surrendered[seat] = true
		}
	}
}

func (a *Actor) doOpeningAction(userID, action string, tiles []string, direction, suit int32, params map[string]string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	return a.engine.ApplyOpeningActionByPlayer(context.Background(), a.round, seat, action, tiles, direction, suit, params)
}

func (a *Actor) seatOf(userID string) (Seat, error) {
	if a.Room == nil {
		return SeatInvalid, fmt.Errorf("nil room")
	}
	if a.round == nil {
		return SeatInvalid, fmt.Errorf("round not started")
	}
	for i := 0; i < 4; i++ {
		if a.Room.PlayerIDs[i] == userID {
			return Seat(i), nil
		}
	}
	return SeatInvalid, fmt.Errorf("not in room")
}

func (a *Actor) closeRoomAfterRound() {
	if a == nil || a.Room == nil {
		return
	}
	if a.round != nil {
		var scores [4]int32
		for seat, score := range a.round.RoundScoreBalances() {
			if seat < len(scores) {
				scores[seat] = score
			}
		}
		a.Room.AddRoundScores(scores)
	}
	roomLogCtx := logx.WithRoomID(context.Background(), a.Room.ID)
	if err := a.Room.CloseToSettling(); err != nil {
		logx.Warn(roomLogCtx, "房间进入结算态失败", "err", err.Error())
	}
	if a.Room.CanStartNextHand() {
		if err := a.Room.PrepareNextHand(); err != nil {
			logx.Warn(roomLogCtx, "房间准备下一局失败", "err", err.Error())
		}
		return
	}
	if err := a.Room.CloseRoom(); err != nil {
		logx.Warn(roomLogCtx, "房间关闭失败", "err", err.Error())
	}
}
