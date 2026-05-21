package xuezhandaodi

import (
	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// claimPolicy 实现川麻血战的抢答限制：不能吃，定缺牌不能碰杠胡，抢杠窗口只允许胡。
type claimPolicy struct{}

func (claimPolicy) Candidates(ctx rules.ClaimContext) []rules.ClaimAction {
	if ctx.Hued || ctx.Hand == nil || ctx.Tile == 0 || ctx.Seat == ctx.SourceSeat {
		return nil
	}
	out := make([]rules.ClaimAction, 0, 3)
	if ctx.CheckHu != nil {
		huCtx := ctx.HuContext
		huCtx.RuleState = ctx.RuleState
		if _, ok := ctx.CheckHu(ctx.Hand, ctx.Tile, huCtx); ok {
			action := rules.ClaimAction{Name: rules.ActionHu, ChoiceAction: "hu_choice", Priority: 3}
			if ctx.QiangGangWindow {
				action.ChoiceAction = "qiang_gang_choice"
			}
			out = append(out, action)
		}
	}
	if ctx.QiangGangWindow || isMissingSuitClaim(ctx.RuleState, int(ctx.Seat), ctx.Tile) {
		return out
	}
	count := 0
	for _, current := range ctx.Hand.Tiles() {
		if current == ctx.Tile {
			count++
		}
	}
	if count >= 3 {
		out = append(out, rules.ClaimAction{Name: rules.ActionGang, ChoiceAction: "gang_choice", Priority: 2})
	}
	if count >= 2 {
		out = append(out, rules.ClaimAction{Name: rules.ActionPong, ChoiceAction: "pong_choice", Priority: 1})
	}
	return out
}

func isMissingSuitClaim(state rules.RuleState, seat int, t tile.Tile) bool {
	current := decodeRuleState(state)
	normalizeRuleState(&current, true)
	if t == 0 || seat < 0 || seat >= len(current.MissingSuits) {
		return false
	}
	submitted := current.Submitted[openingStepMissing]
	if seat >= len(submitted) || !submitted[seat] {
		return false
	}
	missing := current.MissingSuits[seat]
	return missing >= 0 && missing <= 2 && int32(t.Suit()) == missing
}

type selfActionPolicy struct{}

func (selfActionPolicy) CanAnGang(ctx rules.SelfActionContext) bool {
	if isMissingSuitClaim(ctx.RuleState, int(ctx.Seat), ctx.Tile) {
		return false
	}
	return rules.StandardSelfActionPolicy{}.CanAnGang(ctx)
}

func (selfActionPolicy) CanBuGang(ctx rules.SelfActionContext) bool {
	if isMissingSuitClaim(ctx.RuleState, int(ctx.Seat), ctx.Tile) {
		return false
	}
	return rules.StandardSelfActionPolicy{}.CanBuGang(ctx)
}

type scoringPolicy struct {
	rule *rule
}

func (scoringPolicy) FeatureFlags() []string {
	return []string{"fan_breakdown", "dealer", "advanced_fans", "gang_context"}
}

func (p scoringPolicy) ScoreWin(result rules.HuResult, sc rules.ScoreContext) (fan.Breakdown, []rules.ScoreEvent, bool) {
	breakdown := p.rule.scoreFans(result, sc)
	if breakdown.Total <= 0 {
		return breakdown, nil, false
	}
	reason := ReasonHuTsumo
	switch {
	case !sc.IsTsumo && sc.IsGangShangPao:
		reason = ReasonHuQiangGang
	case !sc.IsTsumo:
		reason = ReasonHuDiscard
	}
	amount := int32(breakdown.Total) //nolint:gosec // fan totals are small
	names := scoringFanLabels(breakdown)
	if reason == ReasonHuQiangGang && sc.ResponsibleSeat >= 0 {
		names = append(names, ReasonBaoPai)
	}
	events := make([]rules.ScoreEvent, 0, len(sc.ActiveSeats))
	if sc.IsTsumo {
		for _, other := range sc.ActiveSeats {
			if other == sc.HuSeat {
				continue
			}
			events = append(events, newScoreEvent(reason, other, sc.HuSeat, amount, sc.Step, sc.HuSeat, names))
		}
		return breakdown, events, true
	}
	if sc.ResponsibleSeat >= 0 && sc.ResponsibleSeat < 4 && sc.ResponsibleSeat != sc.HuSeat {
		events = append(events, newScoreEvent(reason, sc.ResponsibleSeat, sc.HuSeat, amount, sc.Step, sc.HuSeat, names))
	}
	return breakdown, events, true
}

func (scoringPolicy) ScoreGang(sc rules.GangScoreContext) ([]rules.ScoreEvent, rules.GangRecord) {
	amount := int32(1)
	reason := ReasonGangMing
	switch sc.Kind {
	case rules.GangKindAn:
		amount = 2
		reason = ReasonGangAn
	case rules.GangKindBu:
		reason = ReasonGangBu
	}
	events := make([]rules.ScoreEvent, 0, len(sc.ActiveSeats))
	for _, other := range sc.ActiveSeats {
		if other == sc.Seat {
			continue
		}
		events = append(events, rules.ScoreEvent{
			Reason:     reason,
			FromSeat:   other,
			ToSeat:     sc.Seat,
			Amount:     amount,
			Step:       sc.Step,
			WinnerSeat: -1,
		})
	}
	return events, rules.GangRecord{
		Seat:            sc.Seat,
		Kind:            sc.Kind,
		Tile:            sc.Tile,
		FromSeat:        sc.FromSeat,
		ResponsibleSeat: sc.FromSeat,
		Step:            sc.Step,
	}
}

func scoringFanLabels(b fan.Breakdown) []string {
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

func newScoreEvent(reason string, from, to domainroom.Seat, amount int32, step int, winner domainroom.Seat, names []string) rules.ScoreEvent {
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
