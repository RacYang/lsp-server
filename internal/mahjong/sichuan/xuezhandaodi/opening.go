package xuezhandaodi

import (
	"fmt"
	"sort"
	"strconv"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

const (
	openingStepExchange = "sichuan.exchange"
	openingStepMissing  = "sichuan.missing_suit"

	openingProtocolExchange = "exchange_three"
	openingProtocolMissing  = "que_men"
)

type openingPolicy struct {
	withExchange bool
}

func (p openingPolicy) Steps() []string {
	if p.withExchange {
		return []string{openingProtocolExchange, openingProtocolMissing}
	}
	return []string{openingProtocolMissing}
}

func (p openingPolicy) InitialState() rules.RuleState {
	return encodeRuleState(initialRuleState(p.withExchange))
}

func (p openingPolicy) CurrentStep(ctx rules.OpeningContext) (*rules.OpeningStep, bool) {
	state := decodeRuleState(ctx.RuleState)
	normalizeRuleState(&state, p.withExchange)
	if p.withExchange && !allSubmitted(state.Submitted[openingStepExchange]) {
		return exchangeStep(), true
	}
	if !allSubmitted(state.Submitted[openingStepMissing]) {
		return missingStep(), true
	}
	return nil, false
}

func (p openingPolicy) Apply(ctx rules.OpeningActionContext) (rules.OpeningResult, error) {
	state := decodeRuleState(ctx.RuleState)
	normalizeRuleState(&state, p.withExchange)
	result := rules.OpeningResult{
		RuleState: encodeRuleState(state),
		Hands:     ctx.Hands,
	}
	step, ok := p.CurrentStep(rules.OpeningContext{RuleState: result.RuleState, Hands: ctx.Hands})
	if !ok {
		result.AllOpeningComplete = true
		return result, nil
	}
	if ctx.Seat < 0 || ctx.Seat > 3 {
		return result, fmt.Errorf("invalid seat")
	}
	switch step.ID {
	case openingStepExchange:
		if string(ctx.Action) != openingProtocolExchange {
			return result, fmt.Errorf("opening action not allowed")
		}
		return p.applyExchange(ctx, state)
	case openingStepMissing:
		if string(ctx.Action) != openingProtocolMissing {
			return result, fmt.Errorf("opening action not allowed")
		}
		return p.applyMissingSuit(ctx, state)
	default:
		return result, fmt.Errorf("unknown opening step")
	}
}

func (p openingPolicy) applyExchange(ctx rules.OpeningActionContext, state ruleState) (rules.OpeningResult, error) {
	if submitted := state.Submitted[openingStepExchange]; submitted[ctx.Seat] {
		return rules.OpeningResult{RuleState: encodeRuleState(state), Hands: ctx.Hands}, fmt.Errorf("opening action already submitted")
	}
	direction := state.Direction[openingStepExchange]
	if direction < 0 {
		var ok bool
		direction, ok = normalizeOpeningExchangeDirection(ctx.Direction)
		if !ok {
			direction = 3
		}
	}
	state.Direction[openingStepExchange] = direction
	selection := []tile.Tile{}
	if !ctx.Surrender {
		var err error
		selection, err = normalizeOpeningExchangeSelection(ctx.Hands[ctx.Seat], ctx.Tiles, ctx.Timeout)
		if err != nil {
			return rules.OpeningResult{RuleState: encodeRuleState(state), Hands: ctx.Hands}, err
		}
	}
	state.Selections[openingStepExchange][ctx.Seat] = append([]tile.Tile(nil), selection...)
	state.Submitted[openingStepExchange][ctx.Seat] = true
	result := rules.OpeningResult{RuleState: encodeRuleState(state), Hands: ctx.Hands}
	if !allSubmitted(state.Submitted[openingStepExchange]) {
		result.NextStep = exchangeStep()
		return result, nil
	}
	away := openingSelectionsToStrings(state.Selections[openingStepExchange])
	received := openingExchangeThreeWithSelections(ctx.Hands, state.Selections[openingStepExchange], direction)
	state.Selections[openingStepExchange] = make([][]tile.Tile, 4)
	result.RuleState = encodeRuleState(state)
	result.Hands = ctx.Hands
	result.CompletedStep = exchangeStep()
	result.NextStep = missingStep()
	localTiles := make(map[domainroom.Seat]map[string][]string, 4)
	for seat := 0; seat < 4; seat++ {
		localTiles[domainroom.Seat(seat)] = map[string][]string{"exchanged_away": append([]string(nil), away[seat]...)}
	}
	result.Notifications = []rules.OpeningNotification{{
		Done: rules.OpeningDoneProjection{
			Action:     openingProtocolExchange,
			StepID:     openingStepExchange,
			Kind:       "exchange_done",
			Params:     map[string]string{"direction": strconv.FormatInt(int64(direction), 10)},
			SeatTiles:  map[string][]rules.OpeningSeatTilesProjection{"received": received},
			LocalTiles: localTiles,
		},
	}}
	return result, nil
}

func (p openingPolicy) applyMissingSuit(ctx rules.OpeningActionContext, state ruleState) (rules.OpeningResult, error) {
	if submitted := state.Submitted[openingStepMissing]; submitted[ctx.Seat] {
		return rules.OpeningResult{RuleState: encodeRuleState(state), Hands: ctx.Hands}, fmt.Errorf("opening action already submitted")
	}
	switch {
	case ctx.Surrender:
		state.MissingSuits[ctx.Seat] = -1
	case ctx.Suit >= 0 && ctx.Suit <= 2:
		state.MissingSuits[ctx.Seat] = ctx.Suit
	default:
		state.MissingSuits[ctx.Seat] = int32(chooseMissingSuit(ctx.Hands[ctx.Seat]))
	}
	state.Submitted[openingStepMissing][ctx.Seat] = true
	result := rules.OpeningResult{RuleState: encodeRuleState(state), Hands: ctx.Hands}
	if !allSubmitted(state.Submitted[openingStepMissing]) {
		result.NextStep = missingStep()
		return result, nil
	}
	result.CompletedStep = missingStep()
	result.AllOpeningComplete = true
	result.Notifications = []rules.OpeningNotification{{
		Done: rules.OpeningDoneProjection{
			Action:   openingProtocolMissing,
			StepID:   openingStepMissing,
			Kind:     "missing_suit_done",
			SeatInts: map[string][]int32{"que_suit": append([]int32(nil), state.MissingSuits...)},
		},
	}}
	return result, nil
}

func exchangeStep() *rules.OpeningStep {
	return &rules.OpeningStep{ID: openingStepExchange, Action: openingProtocolExchange, Reason: openingProtocolExchange, CompleteKind: "exchange"}
}

func missingStep() *rules.OpeningStep {
	return &rules.OpeningStep{ID: openingStepMissing, Action: openingProtocolMissing, Reason: openingProtocolMissing, CompleteKind: "missing_suit"}
}

func allSubmitted(done []bool) bool {
	if len(done) < 4 {
		return false
	}
	for seat := 0; seat < 4; seat++ {
		if !done[seat] {
			return false
		}
	}
	return true
}

func ensureBoolStep(m map[string][]bool, stateStep string) {
	for len(m[stateStep]) < 4 {
		m[stateStep] = append(m[stateStep], false)
	}
}

func ensureTileStep(m map[string][][]tile.Tile, stateStep string) {
	for len(m[stateStep]) < 4 {
		m[stateStep] = append(m[stateStep], nil)
	}
}

func normalizeOpeningExchangeDirection(direction int32) (int32, bool) {
	switch direction {
	case 1, 2, 3:
		return direction, true
	default:
		return 0, false
	}
}

func normalizeOpeningExchangeSelection(h *hand.Hand, raws []string, allowFallback bool) ([]tile.Tile, error) {
	if h == nil {
		return nil, fmt.Errorf("missing hand")
	}
	if len(raws) == 0 && allowFallback {
		return chooseOpeningExchangeTiles(h), nil
	}
	if len(raws) != 3 {
		return nil, fmt.Errorf("invalid exchange selection")
	}
	tmp := hand.FromTiles(append([]tile.Tile(nil), h.Tiles()...))
	out := make([]tile.Tile, 0, 3)
	for _, raw := range raws {
		t, err := tile.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse exchange tile %q: %w", raw, err)
		}
		if err := tmp.Remove(t); err != nil {
			return nil, fmt.Errorf("exchange tile from hand: %w", err)
		}
		out = append(out, t)
	}
	return out, nil
}

func chooseOpeningExchangeTiles(h *hand.Hand) []tile.Tile {
	ts := append([]tile.Tile(nil), h.Tiles()...)
	sort.Slice(ts, func(i, j int) bool {
		if ts[i].Suit() != ts[j].Suit() {
			return ts[i].Suit() < ts[j].Suit()
		}
		return ts[i].Rank() < ts[j].Rank()
	})
	suitCount := map[tile.Suit]int{tile.SuitCharacters: 0, tile.SuitDots: 0, tile.SuitBamboo: 0}
	for _, t := range ts {
		suitCount[t.Suit()]++
	}
	targetSuit := tile.SuitCharacters
	maxCount := -1
	for _, suit := range []tile.Suit{tile.SuitCharacters, tile.SuitDots, tile.SuitBamboo} {
		if suitCount[suit] > maxCount {
			targetSuit = suit
			maxCount = suitCount[suit]
		}
	}
	picked := make([]tile.Tile, 0, 3)
	for _, t := range ts {
		if t.Suit() == targetSuit {
			picked = append(picked, t)
			if len(picked) == 3 {
				return picked
			}
		}
	}
	return ts[:3]
}

func openingExchangeThreeWithSelections(hands []*hand.Hand, selections [][]tile.Tile, direction int32) []rules.OpeningSeatTilesProjection {
	offset, ok := normalizeOpeningExchangeDirection(direction)
	if !ok {
		offset = 3
	}
	exchanged := make([][]tile.Tile, 4)
	for seat := 0; seat < 4; seat++ {
		chosen := append([]tile.Tile(nil), selections[seat]...)
		for _, t := range chosen {
			_ = hands[seat].Remove(t)
		}
		exchanged[seat] = chosen
	}
	perSeat := make([]rules.OpeningSeatTilesProjection, 0, 4)
	for seat := 0; seat < 4; seat++ {
		from := (seat + int(offset)) % 4
		for _, t := range exchanged[from] {
			hands[seat].Add(t)
		}
		perSeat = append(perSeat, rules.OpeningSeatTilesProjection{
			Seat:  domainroom.Seat(seat),
			Tiles: openingTilesToStrings(exchanged[from]),
		})
	}
	return perSeat
}

func openingSelectionsToStrings(selections [][]tile.Tile) [][]string {
	out := make([][]string, len(selections))
	for seat := range selections {
		out[seat] = openingTilesToStrings(selections[seat])
	}
	return out
}

func openingTilesToStrings(ts []tile.Tile) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.String())
	}
	return out
}

func chooseMissingSuit(h *hand.Hand) tile.Suit {
	counts := map[tile.Suit]int{tile.SuitCharacters: 0, tile.SuitDots: 0, tile.SuitBamboo: 0}
	for _, t := range h.Tiles() {
		counts[t.Suit()]++
	}
	bestSuit := tile.SuitCharacters
	bestCount := counts[bestSuit]
	for _, suit := range []tile.Suit{tile.SuitDots, tile.SuitBamboo} {
		if counts[suit] < bestCount {
			bestSuit = suit
			bestCount = counts[suit]
		}
	}
	return bestSuit
}
