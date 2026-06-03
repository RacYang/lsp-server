package engine

import (
	"context"
	"fmt"
	"sort"

	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	codec "racoo.cn/lsp/internal/service/room/engine/codec"
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
	startPayload, err := codec.BuildStartGame(
		"start", rs.roomID,
		rs.dealerSeat.Proto(), 0, 0,
		rs.ruleMeta().ToCodecData(),
		progress.ToCodecData(),
	)
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
		notifications, err := rs.openingProjectionNotifications(projection, result.NextStep)
		if err != nil {
			return nil, err
		}
		out = append(out, notifications...)
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

func (rs *RoundState) openingProjectionNotifications(projection rules.OpeningNotification, nextStep *rules.OpeningStep) ([]Notification, error) {
	progress := rs.roundProgress()
	if nextStep == nil {
		progress = rs.drawTransitionProgress()
	}
	return openingProjectionToNotification(projection, progress)
}

func openingProjectionToNotification(projection rules.OpeningNotification, progress RoundProgress) ([]Notification, error) {
	done := codec.OpeningDoneData{
		Action:    projection.Done.Action,
		StepID:    projection.Done.StepID,
		Kind:      projection.Done.Kind,
		Params:    cloneStringMap(projection.Done.Params),
		SeatTiles: toCodecSeatTiles(projection.Done.SeatTiles),
		SeatInts:  toCodecSeatInts(projection.Done.SeatInts),
	}
	progressData := progress.ToCodecData()
	if len(projection.Done.LocalTiles) > 0 {
		var buildErr error
		notifications := perSeatNotifications(KindOpeningDone, func(target Seat) []byte {
			d := done
			d.LocalTiles = localTilesForSeat(projection.Done.LocalTiles, target)
			payload, err := codec.BuildOpeningDone("opening-done", d, progressData)
			if err != nil {
				buildErr = err
				return nil
			}
			return payload
		})
		if buildErr != nil {
			return nil, buildErr
		}
		return notifications, nil
	}
	payload, err := codec.BuildOpeningDone("opening-done", done, progressData)
	if err != nil {
		return nil, err
	}
	return []Notification{{Kind: KindOpeningDone, Payload: payload, TargetSeat: BroadcastSeat}}, nil
}

func toCodecSeatTiles(in map[string][]rules.OpeningSeatTilesProjection) []codec.SeatTilesData {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]codec.SeatTilesData, 0, len(keys))
	for _, key := range keys {
		items := append([]rules.OpeningSeatTilesProjection(nil), in[key]...)
		sort.SliceStable(items, func(i, j int) bool { return items[i].Seat < items[j].Seat })
		seats := make([]codec.SeatTilesItemData, 0, len(items))
		for _, item := range items {
			seats = append(seats, codec.SeatTilesItemData{
				Seat:  item.Seat.Proto(),
				Tiles: append([]string(nil), item.Tiles...),
			})
		}
		out = append(out, codec.SeatTilesData{Key: key, Seats: seats})
	}
	return out
}

func toCodecSeatInts(in map[string][]int32) []codec.SeatIntsData {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]codec.SeatIntsData, 0, len(keys))
	for _, key := range keys {
		out = append(out, codec.SeatIntsData{Key: key, Values: append([]int32(nil), in[key]...)})
	}
	return out
}

func localTilesForSeat(in map[Seat]map[string][]string, target Seat) []codec.LocalTilesData {
	if !target.Valid() {
		return nil
	}
	items := in[target]
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]codec.LocalTilesData, 0, len(keys))
	for _, key := range keys {
		out = append(out, codec.LocalTilesData{Key: key, Tiles: append([]string(nil), items[key]...)})
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
	progress := rs.roundProgress()
	progressData := progress.ToCodecData()
	out := make([]Notification, 0, 4)
	for seat := 0; seat < 4; seat++ {
		seatIndex := SeatFromInt(seat).Proto()
		detail := codec.ActionDetail{
			Step:      progress.Step,
			ActorSeat: seatIndex,
			Action:    action,
		}
		payload, err := codec.BuildAction(
			fmt.Sprintf("%s-%d", action, seat),
			seatIndex, action, "",
			detail, progressData,
		)
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
