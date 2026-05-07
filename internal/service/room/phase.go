package room

import clientv1 "racoo.cn/lsp/api/gen/go/client/v1"

func (rs *RoundState) phase() clientv1.Phase {
	if rs == nil {
		return clientv1.Phase_PHASE_UNSPECIFIED
	}
	switch {
	case rs.closed:
		return clientv1.Phase_PHASE_CLOSED
	case rs.waitingExchange:
		return clientv1.Phase_PHASE_EXCHANGE
	case rs.waitingQueMen:
		return clientv1.Phase_PHASE_QUE_MEN
	case rs.claimWindowOpen:
		return clientv1.Phase_PHASE_CLAIM
	case rs.waitingTsumo:
		return clientv1.Phase_PHASE_TSUMO
	case rs.waitingDiscard:
		return clientv1.Phase_PHASE_DISCARD
	default:
		return clientv1.Phase_PHASE_UNSPECIFIED
	}
}

func (rs *RoundState) actingSeats() []int32 {
	if rs == nil {
		return nil
	}
	switch {
	case rs.waitingExchange:
		return pendingSeats(rs.exchangeSubmitted)
	case rs.waitingQueMen:
		return pendingSeats(rs.queSubmitted)
	case rs.claimWindowOpen:
		if seat := rs.claimSeat(); seat >= 0 {
			return []int32{seat.Proto()}
		}
	case rs.waitingTsumo || rs.waitingDiscard:
		return []int32{rs.turn.Proto()}
	}
	return nil
}

func pendingSeats(done []bool) []int32 {
	out := make([]int32, 0, 4)
	for seat := 0; seat < 4; seat++ {
		if seat >= len(done) || !done[seat] {
			out = append(out, Seat(seat).Proto())
		}
	}
	return out
}
