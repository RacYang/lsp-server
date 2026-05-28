package room

import (
	"context"
	"fmt"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

const autoRoundStepLimit = 256

// PlayAutoRound 生成一局确定性回放通知：按规则开局、摸打与结算。
func (e *Engine) PlayAutoRound(ctx context.Context, roomID string, playerIDs [4]string) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	rule := rules.MustGet(e.ruleID)
	caps := rules.CapabilitiesOf(rule)
	// 牌墙 seed 仅用于可复现回放；截到 63 位后再转 int64，避免无符号到有符号窄化告警。
	w := caps.TileSet.BuildWall(ctx, int64(seedFromRoomID(roomID)&0x7fff_ffff_ffff_ffff)) //nolint:gosec // G115：上式已保证最高位清零
	hands := make([]*hand.Hand, 4)
	for i := range hands {
		hands[i] = hand.New()
	}
	for round := 0; round < 13; round++ {
		for seat := 0; seat < 4; seat++ {
			t, err := w.Draw()
			if err != nil {
				return nil, err
			}
			hands[seat].Add(t)
		}
	}

	ruleState, hands, openingNotifications, err := runAutoOpening(caps, hands)
	if err != nil {
		return nil, err
	}
	out := append([]Notification(nil), openingNotifications...)

	startProgress := autoDrawProgress(0, Seat(0), w.Remaining())
	start := &clientv1.StartGameNotify{RoomId: roomID, DealerSeat: 0}
	startProgress.applyToStart(start)
	startPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "start",
		Body: &clientv1.Envelope_StartGame{
			StartGame: start,
		},
	})
	if err != nil {
		return nil, err
	}
	out = append(out, Notification{Kind: KindStartGame, Payload: startPayload, TargetSeat: BroadcastSeat})

	scoreEvents := make([]rules.ScoreEvent, 0, 16)
	wonSeats := make([]bool, 4)
	winEvents := make([]rules.WinEvent, 0, 3)
	turn := Seat(0)
	for step := 0; step < autoRoundStepLimit && !caps.Termination.GameOver(rules.TerminationContext{WinEvents: winEvents, WallRemaining: w.Remaining(), ActiveSeats: autoActiveSeats(wonSeats)}); step++ {
		drawn, err := w.Draw()
		if err != nil {
			return nil, err
		}
		seatIndex := turn.Proto()
		drawProgress := autoDrawProgress(step, turn, w.Remaining())
		draw := &clientv1.DrawTileNotify{SeatIndex: seatIndex, Tile: drawn.String()}
		drawProgress.applyToDraw(draw)
		drawPayload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("draw-%d", step),
			Body: &clientv1.Envelope_DrawTile{
				DrawTile: draw,
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, Notification{Kind: KindDrawTile, Payload: drawPayload, TargetSeat: BroadcastSeat})

		huCtx := rules.HuContext{Seat: turn, Source: rules.HuSourceTsumo, PendingTile: drawn, RuleState: ruleState}
		if result, ok := caps.Win.CheckHu(hands[turn], drawn, huCtx); ok {
			breakdown, events, ok := caps.Scoring.ScoreWin(result, rules.ScoreContext{
				HuSeat:        turn,
				DealerSeat:    0,
				WinningTile:   drawn,
				IsTsumo:       true,
				IsOpeningDraw: step == 0 && turn == 0,
				ActiveSeats:   autoActiveSeats(wonSeats),
				WallRemaining: w.Remaining(),
				Step:          step,
			})
			if !ok {
				hands[turn].Add(drawn)
			} else {
				scoreEvents = append(scoreEvents, events...)
				wonSeats[turn] = true
				winEvents = append(winEvents, rules.WinEvent{
					Seat:     turn,
					Source:   rules.HuSourceTsumo,
					Tile:     drawn,
					FromSeat: SeatInvalid,
					Step:     step,
					TotalFan: int32(breakdown.Total), //nolint:gosec // 番数很小
					FanNames: fanLabels(breakdown),
				})
				action := &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "hu", Tile: drawn.String()}
				drawProgress.applyToAction(action)
				huPayload, err := marshalEnvelope(&clientv1.Envelope{
					ReqId: fmt.Sprintf("hu-%d", step),
					Body: &clientv1.Envelope_Action{
						Action: action,
					},
				})
				if err != nil {
					return nil, err
				}
				out = append(out, Notification{Kind: KindAction, Payload: huPayload, TargetSeat: BroadcastSeat})
				turn = nextAutoActiveSeat(turn, wonSeats)
				continue
			}
		}

		if !containsTile(hands[turn].Tiles(), drawn) {
			hands[turn].Add(drawn)
		}
		discard := chooseDiscard(hands[turn], autoMissingSuit(caps, ruleState, turn))
		if err := hands[turn].Remove(discard); err != nil {
			return nil, err
		}
		action := &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "discard", Tile: discard.String()}
		drawProgress.applyToAction(action)
		actionPayload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("discard-%d", step),
			Body: &clientv1.Envelope_Action{
				Action: action,
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, Notification{Kind: KindAction, Payload: actionPayload, TargetSeat: BroadcastSeat})
		turn = nextAutoActiveSeat(turn, wonSeats)
	}
	settlement := caps.Settlement.BuildSettlement(rules.SettlementContext{
		PlayerIDs:   playerIDs,
		Hands:       hands,
		RuleState:   ruleState,
		WinEvents:   winEvents,
		ScoreEvents: scoreEvents,
	})
	settlementPayload, err := buildSettlementNotification(roomID, settlement)
	if err != nil {
		return nil, err
	}
	out = append(out, Notification{Kind: KindSettlement, Payload: settlementPayload, TargetSeat: BroadcastSeat})
	return out, nil
}

func runAutoOpening(caps rules.CapabilitySet, hands []*hand.Hand) (rules.RuleState, []*hand.Hand, []Notification, error) {
	state := newInitialRuleState(caps)
	if caps.Opening == nil {
		return state, hands, nil, nil
	}
	out := make([]Notification, 0, 2)
	for {
		step, ok := caps.Opening.CurrentStep(rules.OpeningContext{RuleState: state, Hands: hands})
		if !ok {
			return state, hands, out, nil
		}
		for seat := 0; seat < 4; seat++ {
			input := rules.OpeningActionContext{
				OpeningContext: rules.OpeningContext{RuleState: state, Hands: hands},
				Seat:           Seat(seat),
				Action:         rules.OpeningActionName(step.Action),
				Direction:      defaultExchangeDirection,
				Timeout:        true,
			}
			result, err := caps.Opening.Apply(input)
			if err != nil {
				return state, hands, out, err
			}
			state = result.RuleState
			if result.Hands != nil {
				hands = result.Hands
			}
			projected, err := autoOpeningNotifications(result)
			if err != nil {
				return state, hands, out, err
			}
			out = append(out, projected...)
			if result.AllOpeningComplete {
				return state, hands, out, nil
			}
		}
	}
}

func autoOpeningNotifications(result rules.OpeningResult) ([]Notification, error) {
	if len(result.Notifications) == 0 {
		return nil, nil
	}
	progress := autoOpeningProgress(result.NextStep)
	out := make([]Notification, 0, len(result.Notifications))
	for _, projection := range result.Notifications {
		notifications, err := openingProjectionToNotification(projection, progress)
		if err != nil {
			return nil, err
		}
		out = append(out, notifications...)
	}
	return out, nil
}

func autoOpeningProgress(nextStep *rules.OpeningStep) RoundProgress {
	if nextStep == nil {
		return autoDrawProgress(0, Seat(0), 0)
	}
	reason := openingWaitingReason(nextStep)
	return RoundProgress{
		Phase:            PhaseOpening,
		Step:             0,
		ActingSeats:      []int32{0, 1, 2, 3},
		WaitingAction:    nextStep.Action,
		AvailableActions: []string{nextStep.Action},
		Reason:           reason,
	}
}

func autoDrawProgress(step int, turn Seat, remaining int) RoundProgress {
	return RoundProgress{
		Phase:           PhaseDraw,
		Step:            int64(step),
		ActingSeat:      turn.Proto(),
		ActingSeats:     []int32{turn.Proto()},
		WaitingAction:   "none",
		WallRemaining:   int32(remaining), //nolint:gosec // 麻将牌墙小于 int32 上限
		Reason:          ReasonNone,
		ServerNowUnixMs: 0,
	}
}

func autoMissingSuit(caps rules.CapabilitySet, state rules.RuleState, seat Seat) tile.Suit {
	state = caps.State.NormalizeRuleState(state)
	projection := caps.StateView.ProjectRuleState(state)
	missingBySeat := projection.SeatInts[ruleProjectionKeyQueSuit]
	if seat < 0 || int(seat) >= len(missingBySeat) {
		return tile.Suit(255)
	}
	missing := missingBySeat[seat]
	if missing < int32(tile.SuitCharacters) || missing > int32(tile.SuitBamboo) {
		return tile.Suit(255)
	}
	return tile.Suit(missing)
}

func autoActiveSeats(hued []bool) []Seat {
	out := make([]Seat, 0, SeatCount)
	for seat := Seat(0); seat < SeatCount; seat++ {
		if int(seat) < len(hued) && hued[seat] {
			continue
		}
		out = append(out, seat)
	}
	return out
}

func containsTile(ts []tile.Tile, target tile.Tile) bool {
	for _, t := range ts {
		if t == target {
			return true
		}
	}
	return false
}

func nextAutoActiveSeat(from Seat, hued []bool) Seat {
	for offset := 1; offset <= 4; offset++ {
		seat := Seat((int(from) + offset) % 4)
		if seat >= 0 && int(seat) < len(hued) && !hued[seat] {
			return seat
		}
	}
	return from
}
