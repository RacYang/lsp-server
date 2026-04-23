package room

// Room 为房间聚合根（Phase 1 内存版），包含状态机与座位绑定。
type Room struct {
	ID        string
	FSM       *FSM
	PlayerIDs [4]string
	Ready     [4]bool
}

// NewRoom 创建空房间并进入 waiting（等待玩家加入）。
func NewRoom(id string) *Room {
	r := &Room{ID: id, FSM: NewFSM()}
	_ = r.FSM.Transition(StateWaiting)
	return r
}

// SeatOf 返回座位上的 user_id，空字符串表示空位。
func (r *Room) SeatOf(seat int) string {
	if r == nil || seat < 0 || seat > 3 {
		return ""
	}
	return r.PlayerIDs[seat]
}

// JoinAutoSeat 将玩家放入第一个空座位；满座返回 false。
func (r *Room) JoinAutoSeat(userID string) (int, bool) {
	if r == nil {
		return -1, false
	}
	for i := 0; i < 4; i++ {
		if r.PlayerIDs[i] == "" {
			r.PlayerIDs[i] = userID
			return i, true
		}
	}
	return -1, false
}

// SetReady 标记座位准备状态；若四人全准备则切到 ready 态。
func (r *Room) SetReady(seat int, v bool) error {
	if r == nil {
		return nil
	}
	if seat < 0 || seat > 3 {
		return nil
	}
	r.Ready[seat] = v
	all := r.PlayerIDs[0] != "" && r.PlayerIDs[1] != "" && r.PlayerIDs[2] != "" && r.PlayerIDs[3] != ""
	allReady := all && r.Ready[0] && r.Ready[1] && r.Ready[2] && r.Ready[3]
	if allReady && r.FSM.State() == StateWaiting {
		return r.FSM.Transition(StateReady)
	}
	if r.FSM.State() == StateReady && !allReady {
		return r.FSM.Transition(StateWaiting)
	}
	return nil
}

// StartPlaying 从 ready 进入 playing。
func (r *Room) StartPlaying() error {
	if r == nil || r.FSM == nil {
		return nil
	}
	if r.FSM.State() != StateReady {
		return nil
	}
	return r.FSM.Transition(StatePlaying)
}

// CloseToSettling 进入结算态（Phase 1 简化为直接标记）。
func (r *Room) CloseToSettling() error {
	if r == nil || r.FSM == nil {
		return nil
	}
	if r.FSM.State() != StatePlaying {
		return nil
	}
	return r.FSM.Transition(StateSettling)
}

// CloseRoom 从结算态进入关闭态。
func (r *Room) CloseRoom() error {
	if r == nil || r.FSM == nil {
		return nil
	}
	if r.FSM.State() != StateSettling {
		return nil
	}
	return r.FSM.Transition(StateClosed)
}
