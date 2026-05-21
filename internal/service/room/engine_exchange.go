package room

import (
	"context"
	"fmt"
	"sort"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func (e *Engine) ApplyOpeningActionByPlayer(_ context.Context, rs *RoundState, seat Seat, action string, tiles []string, direction, suit int32, params map[string]string) ([]Notification, error) {
	return e.applyOpeningAction(rs, seat, rules.OpeningActionName(action), tiles, direction, suit, params, false, false)
}

func (e *Engine) initRoundNotifications(ctx context.Context, rs *RoundState) ([]Notification, error) {
	if step, ok := rs.currentOpeningStep(); ok {
		rs.enterPhase(openingWaitingReason(step))
		return rs.promptSeatActions(step.Action), nil
	}
	return e.completeOpening(ctx, rs)
}

func (e *Engine) completeOpening(ctx context.Context, rs *RoundState) ([]Notification, error) {
	rs.enterPhase(ReasonNone)
	progress := rs.drawTransitionProgress()
	start := &clientv1.StartGameNotify{
		RoomId:     rs.roomID,
		DealerSeat: rs.dealerSeat.Proto(),
		RoundIndex: 0,
		HandIndex:  0,
		RuleMeta:   rs.ruleMeta(),
	}
	progress.applyToStart(start)
	startPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "start",
		Body: &clientv1.Envelope_StartGame{
			StartGame: start,
		},
	})
	if err != nil {
		return nil, err
	}
	out := []Notification{{Kind: KindStartGame, Payload: startPayload, TargetSeat: BroadcastSeat}}
	next, err := e.drawForCurrentTurn(rs)
	if err != nil {
		return nil, err
	}
	_ = ctx
	return append(out, next...), nil
}

func (rs *RoundState) currentOpeningStep() (*rules.OpeningStep, bool) {
	if rs == nil || rs.caps.Opening == nil {
		return nil, false
	}
	return rs.caps.Opening.CurrentStep(rules.OpeningContext{RuleState: rs.ruleState, Hands: rs.hands})
}

func (e *Engine) applyOpeningAction(rs *RoundState, seat Seat, action rules.OpeningActionName, tiles []string, direction, suit int32, params map[string]string, timeout, surrender bool) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, fmt.Errorf("round closed")
	}
	rs.ensureRuleRuntime()
	if rs.caps.Opening == nil {
		return nil, fmt.Errorf("opening action not allowed")
	}
	step, ok := rs.currentOpeningStep()
	if !ok || step.Action != string(action) {
		return nil, fmt.Errorf("opening action not allowed")
	}
	if seat < 0 || seat > 3 {
		return nil, fmt.Errorf("invalid seat")
	}
	result, err := rs.caps.Opening.Apply(rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: rs.ruleState, Hands: rs.hands},
		Seat:           seat,
		Action:         action,
		Tiles:          append([]string(nil), tiles...),
		Suit:           suit,
		Direction:      direction,
		Params:         cloneStringMap(params),
		Timeout:        timeout,
		Surrender:      surrender,
	})
	if err != nil {
		return nil, err
	}
	rs.ruleState = result.RuleState
	if result.Hands != nil {
		rs.hands = result.Hands
	}
	return e.projectOpeningResult(context.Background(), rs, result)
}

func (e *Engine) projectOpeningResult(ctx context.Context, rs *RoundState, result rules.OpeningResult) ([]Notification, error) {
	out := make([]Notification, 0, 2)
	for _, projection := range result.Notifications {
		notification, err := rs.openingProjectionNotification(projection, result.NextStep)
		if err != nil {
			return nil, err
		}
		out = append(out, notification)
	}
	if result.AllOpeningComplete {
		next, err := e.completeOpening(ctx, rs)
		if err != nil {
			return nil, err
		}
		return append(out, next...), nil
	}
	if result.NextStep != nil {
		rs.enterPhase(openingWaitingReason(result.NextStep))
		if result.CompletedStep == nil {
			return out, nil
		}
		out = append(out, rs.promptSeatActions(result.NextStep.Action)...)
	}
	return out, nil
}

func (rs *RoundState) openingProjectionNotification(projection rules.OpeningNotification, nextStep *rules.OpeningStep) (Notification, error) {
	progress := rs.roundProgress()
	if nextStep == nil {
		progress = rs.drawTransitionProgress()
	}
	return openingProjectionToNotification(projection, progress)
}

func openingProjectionToNotification(projection rules.OpeningNotification, progress RoundProgress) (Notification, error) {
	payload, err := marshalOpeningProjection(projection.Done, progress, -1)
	if err != nil {
		return Notification{}, err
	}
	notification := Notification{Kind: KindOpeningDone, Payload: payload, TargetSeat: BroadcastSeat}
	if len(projection.Done.LocalTiles) > 0 {
		notification.Privacy = PrivacyPerSeat
		notification.Project = func(target Seat) []byte {
			if !target.Valid() {
				return nil
			}
			projected, err := marshalOpeningProjection(projection.Done, progress, target)
			if err != nil {
				return nil
			}
			return projected
		}
	}
	return notification, nil
}

func marshalOpeningProjection(done rules.OpeningDoneProjection, progress RoundProgress, target Seat) ([]byte, error) {
	if done.Action == "" || done.Kind == "" {
		return nil, fmt.Errorf("unsupported opening projection")
	}
	payload := &clientv1.OpeningDoneNotify{
		Action:     done.Action,
		StepId:     done.StepID,
		Kind:       done.Kind,
		Params:     cloneStringMap(done.Params),
		SeatTiles:  openingSeatTilesToProto(done.SeatTiles),
		SeatInts:   openingSeatIntsToProto(done.SeatInts),
		LocalTiles: openingLocalTilesToProto(done.LocalTiles, target),
	}
	progress.applyToOpeningDone(payload)
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: "opening-done",
		Body:  &clientv1.Envelope_OpeningDone{OpeningDone: payload},
	})
}

func openingSeatTilesToProto(in map[string][]rules.OpeningSeatTilesProjection) []*clientv1.OpeningSeatTiles {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*clientv1.OpeningSeatTiles, 0, len(keys))
	for _, key := range keys {
		items := append([]rules.OpeningSeatTilesProjection(nil), in[key]...)
		sort.SliceStable(items, func(i, j int) bool { return items[i].Seat < items[j].Seat })
		seats := make([]*clientv1.SeatTiles, 0, len(items))
		for _, item := range items {
			seats = append(seats, &clientv1.SeatTiles{
				SeatIndex: item.Seat.Proto(),
				Tiles:     append([]string(nil), item.Tiles...),
			})
		}
		out = append(out, &clientv1.OpeningSeatTiles{Key: key, Seats: seats})
	}
	return out
}

func openingSeatIntsToProto(in map[string][]int32) []*clientv1.OpeningSeatInts {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*clientv1.OpeningSeatInts, 0, len(keys))
	for _, key := range keys {
		out = append(out, &clientv1.OpeningSeatInts{Key: key, Values: append([]int32(nil), in[key]...)})
	}
	return out
}

func openingLocalTilesToProto(in map[Seat]map[string][]string, target Seat) []*clientv1.OpeningLocalTiles {
	if !target.Valid() {
		return nil
	}
	items := in[target]
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*clientv1.OpeningLocalTiles, 0, len(keys))
	for _, key := range keys {
		out = append(out, &clientv1.OpeningLocalTiles{Key: key, Tiles: append([]string(nil), items[key]...)})
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func openingWaitingReason(step *rules.OpeningStep) WaitingReason {
	if step == nil {
		return ReasonNone
	}
	return ReasonOpening
}

func (rs *RoundState) promptSeatActions(action string) []Notification {
	out := make([]Notification, 0, 4)
	progress := rs.roundProgress()
	for seat := 0; seat < 4; seat++ {
		seatIndex := SeatFromInt(seat).Proto()
		notify := &clientv1.ActionNotify{
			SeatIndex: seatIndex,
			Action:    action,
		}
		progress.applyToAction(notify)
		payload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("%s-%d", action, seat),
			Body: &clientv1.Envelope_Action{
				Action: notify,
			},
		})
		if err != nil {
			continue
		}
		out = append(out, Notification{Kind: KindAction, Payload: payload, TargetSeat: BroadcastSeat})
	}
	return out
}

func tilesToStrings(ts []tile.Tile) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.String())
	}
	return out
}
