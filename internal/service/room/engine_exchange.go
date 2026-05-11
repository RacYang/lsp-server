package room

import (
	"context"
	"fmt"
	"sort"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// ApplyExchangeThree 兼容旧调用方，按玩家显式命令处理。
func (e *Engine) ApplyExchangeThree(ctx context.Context, rs *RoundState, seat Seat, tiles []string, direction int32) ([]Notification, error) {
	return e.ApplyExchangeThreeByPlayer(ctx, rs, seat, tiles, direction)
}

// ApplyExchangeThreeByPlayer 记录玩家已完成换三张确认；玩家提交必须严格校验选牌。
func (e *Engine) ApplyExchangeThreeByPlayer(_ context.Context, rs *RoundState, seat Seat, tiles []string, direction int32) ([]Notification, error) {
	return e.applyExchangeThree(rs, seat, tiles, direction, false)
}

// ApplyExchangeThreeByTimeout 为未响应座位托管选择三张牌；仅托管入口允许自动选牌。
func (e *Engine) ApplyExchangeThreeByTimeout(_ context.Context, rs *RoundState, seat Seat) ([]Notification, error) {
	direction := defaultExchangeDirection
	if rs != nil && rs.exchangeDirection > 0 {
		direction = rs.exchangeDirection
	}
	return e.applyExchangeThree(rs, seat, nil, direction, true)
}

func (e *Engine) applyExchangeThree(rs *RoundState, seat Seat, tiles []string, direction int32, timeout bool) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, fmt.Errorf("round closed")
	}
	if !rs.waitingExchange {
		return nil, fmt.Errorf("exchange not allowed")
	}
	if seat < 0 || seat > 3 {
		return nil, fmt.Errorf("invalid seat")
	}
	if rs.exchangeSubmitted[seat] {
		return nil, fmt.Errorf("exchange already submitted")
	}
	normalizedDirection := rs.exchangeDirection
	if normalizedDirection < 0 {
		var ok bool
		normalizedDirection, ok = normalizeExchangeDirection(direction)
		if !ok {
			normalizedDirection = defaultExchangeDirection
		}
	}
	rs.exchangeDirection = normalizedDirection
	selection, err := normalizeExchangeSelection(rs.hands[seat], tiles, timeout)
	if err != nil {
		return nil, err
	}
	rs.exchangeSelection[seat] = selection
	rs.exchangeSubmitted[seat] = true
	for _, done := range rs.exchangeSubmitted {
		if !done {
			return nil, nil
		}
	}
	rs.waitingExchange = false
	exchangedAwayBySeat := exchangeSelectionsToStrings(rs.exchangeSelection)
	receivedTiles := exchangeThreeWithSelections(rs.hands, rs.exchangeSelection, rs.exchangeDirection)
	for i := range rs.exchangeSelection {
		rs.exchangeSelection[i] = nil
	}
	rs.waitingQueMen = true
	progress := rs.roundProgress()
	exchangePayload, err := marshalExchangeThreeDoneEnvelope(
		receivedTiles,
		nil,
		rs.exchangeDirection,
		progress,
	)
	if err != nil {
		return nil, err
	}
	project := func(target Seat) []byte {
		if !target.Valid() {
			return nil
		}
		seatTiles := receivedTilesForSeat(receivedTiles, target)
		payload, err := marshalExchangeThreeDoneEnvelope(
			seatTiles,
			exchangedAwayBySeat[target],
			rs.exchangeDirection,
			progress,
		)
		if err != nil {
			return nil
		}
		return payload
	}
	out := []Notification{{
		Kind:       KindExchangeThreeDone,
		Payload:    exchangePayload,
		TargetSeat: BroadcastSeat,
		Privacy:    PrivacyPerSeat,
		Project:    project,
	}}
	return append(out, rs.promptSeatActions("que_men")...), nil
}

func marshalExchangeThreeDoneEnvelope(receivedTiles []*clientv1.SeatTiles, exchangedAway []string, direction int32, progress RoundProgress) ([]byte, error) {
	done := &clientv1.ExchangeThreeDoneNotify{
		PerSeat:           receivedTiles,
		Direction:         direction,
		YourExchangedAway: exchangedAway,
	}
	progress.applyToExchangeDone(done)
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: "exchange",
		Body: &clientv1.Envelope_ExchangeThreeDone{
			ExchangeThreeDone: done,
		},
	})
}

func receivedTilesForSeat(receivedTiles []*clientv1.SeatTiles, seat Seat) []*clientv1.SeatTiles {
	out := make([]*clientv1.SeatTiles, 0, 1)
	for _, item := range receivedTiles {
		if item.GetSeatIndex() != seat.Proto() {
			continue
		}
		out = append(out, &clientv1.SeatTiles{
			SeatIndex: item.GetSeatIndex(),
			Tiles:     append([]string(nil), item.GetTiles()...),
		})
	}
	return out
}

func exchangeSelectionsToStrings(selections [][]tile.Tile) [][]string {
	out := make([][]string, len(selections))
	for seat := range selections {
		out[seat] = tilesToStrings(selections[seat])
	}
	return out
}

func (rs *RoundState) initRoundNotifications() ([]Notification, error) {
	if rs.hasOpeningStep("exchange_three") {
		rs.waitingExchange = true
		return rs.promptSeatActions("exchange_three"), nil
	}
	if rs.hasOpeningStep("que_men") {
		rs.waitingQueMen = true
		return rs.promptSeatActions("que_men"), nil
	}
	return nil, fmt.Errorf("unsupported opening flow")
}

func (rs *RoundState) hasOpeningStep(step string) bool {
	if rs == nil || rs.caps.Opening == nil {
		return false
	}
	for _, current := range rs.caps.Opening.Steps() {
		if current == step {
			return true
		}
	}
	return false
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

func exchangeThree(hands []*hand.Hand) []*clientv1.SeatTiles {
	exchanged := make([][]tile.Tile, 4)
	for seat := 0; seat < 4; seat++ {
		chosen := chooseExchangeTiles(hands[seat])
		for _, t := range chosen {
			_ = hands[seat].Remove(t)
		}
		exchanged[seat] = chosen
	}
	perSeat := make([]*clientv1.SeatTiles, 0, 4)
	for seat := 0; seat < 4; seat++ {
		from := (seat + int(defaultExchangeDirection)) % 4
		seatIndex := SeatFromInt(seat).Proto()
		for _, t := range exchanged[from] {
			hands[seat].Add(t)
		}
		perSeat = append(perSeat, &clientv1.SeatTiles{
			SeatIndex: seatIndex,
			Tiles:     tilesToStrings(exchanged[from]),
		})
	}
	return perSeat
}

func exchangeThreeWithSelections(hands []*hand.Hand, selections [][]tile.Tile, direction int32) []*clientv1.SeatTiles {
	offset, ok := normalizeExchangeDirection(direction)
	if !ok {
		offset = defaultExchangeDirection
	}
	exchanged := make([][]tile.Tile, 4)
	for seat := 0; seat < 4; seat++ {
		chosen := append([]tile.Tile(nil), selections[seat]...)
		if len(chosen) == 0 {
			chosen = chooseExchangeTiles(hands[seat])
		}
		for _, t := range chosen {
			_ = hands[seat].Remove(t)
		}
		exchanged[seat] = chosen
	}
	perSeat := make([]*clientv1.SeatTiles, 0, 4)
	for seat := 0; seat < 4; seat++ {
		from := (seat + int(offset)) % 4
		seatIndex := SeatFromInt(seat).Proto()
		for _, t := range exchanged[from] {
			hands[seat].Add(t)
		}
		perSeat = append(perSeat, &clientv1.SeatTiles{
			SeatIndex: seatIndex,
			Tiles:     tilesToStrings(exchanged[from]),
		})
	}
	return perSeat
}

func normalizeExchangeDirection(direction int32) (int32, bool) {
	switch direction {
	case 1, 2, 3:
		return direction, true
	default:
		return 0, false
	}
}

func normalizeExchangeSelection(h *hand.Hand, raws []string, allowFallback bool) ([]tile.Tile, error) {
	if h == nil {
		return nil, fmt.Errorf("missing hand")
	}
	if len(raws) == 0 && allowFallback {
		return chooseExchangeTiles(h), nil
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

func chooseExchangeTiles(h *hand.Hand) []tile.Tile {
	ts := append([]tile.Tile(nil), h.Tiles()...)
	sort.Slice(ts, func(i, j int) bool {
		if ts[i].Suit() != ts[j].Suit() {
			return ts[i].Suit() < ts[j].Suit()
		}
		return ts[i].Rank() < ts[j].Rank()
	})
	suitCount := map[tile.Suit]int{
		tile.SuitCharacters: 0,
		tile.SuitDots:       0,
		tile.SuitBamboo:     0,
	}
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

func tilesToStrings(ts []tile.Tile) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.String())
	}
	return out
}
