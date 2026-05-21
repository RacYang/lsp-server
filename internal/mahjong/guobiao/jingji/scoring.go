package jingji

import (
	"racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/rules"
)

type scoringPolicy struct{}

func (scoringPolicy) FeatureFlags() []string { return nil }

func (scoringPolicy) ScoreWin(result rules.HuResult, sc rules.ScoreContext) (fan.Breakdown, []rules.ScoreEvent, bool) {
	breakdown := scoreMCR(result, sc)
	if breakdown.Total <= 0 {
		return breakdown, nil, false
	}
	reason := "hu_tsumo"
	if !sc.IsTsumo {
		reason = "hu_discard"
	}
	names := fanLabels(breakdown)
	amount := int32(breakdown.Total) //nolint:gosec // 国标番值总和远小于 int32 上限。
	events := make([]rules.ScoreEvent, 0, len(sc.ActiveSeats))
	if sc.IsTsumo {
		for _, other := range sc.ActiveSeats {
			if other == sc.HuSeat {
				continue
			}
			events = append(events, scoreEvent(reason, other, sc.HuSeat, amount, sc.Step, sc.HuSeat, names))
		}
		return breakdown, events, true
	}
	if sc.ResponsibleSeat >= 0 && sc.ResponsibleSeat < 4 && sc.ResponsibleSeat != sc.HuSeat {
		events = append(events, scoreEvent(reason, sc.ResponsibleSeat, sc.HuSeat, amount, sc.Step, sc.HuSeat, names))
	}
	return breakdown, events, true
}

func (scoringPolicy) ScoreGang(sc rules.GangScoreContext) ([]rules.ScoreEvent, rules.GangRecord) {
	return nil, rules.GangRecord{
		Seat:            sc.Seat,
		Kind:            sc.Kind,
		Tile:            sc.Tile,
		FromSeat:        sc.FromSeat,
		ResponsibleSeat: sc.FromSeat,
		Step:            sc.Step,
	}
}

func scoreEvent(reason string, from, to room.Seat, amount int32, step int, winner room.Seat, names []string) rules.ScoreEvent {
	return rules.ScoreEvent{
		Reason:     reason,
		FromSeat:   from,
		ToSeat:     to,
		Amount:     amount,
		Step:       step,
		WinnerSeat: winner,
		WinnerFan:  amount,
		FanNames:   append([]string(nil), names...),
	}
}

func fanLabels(b fan.Breakdown) []string {
	out := make([]string, 0, len(b.Items))
	for _, item := range b.Items {
		if item.Label != "" {
			out = append(out, item.Label)
			continue
		}
		out = append(out, string(item.Kind))
	}
	return out
}
