package room

import (
	"context"
	"fmt"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/sichuanxzdd"
	"racoo.cn/lsp/internal/mahjong/tile"
)

const autoRoundStepLimit = 16

// PlayAutoRound 生成一局确定性回放通知：换三张、定缺、开局、若干摸打与结算。
func (e *Engine) PlayAutoRound(ctx context.Context, roomID string, playerIDs [4]string) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	rule := rules.MustGet(e.ruleID)
	// 牌墙 seed 仅用于可复现回放；截到 63 位后再转 int64，避免无符号到有符号窄化告警。
	w := rule.BuildWall(ctx, int64(seedFromRoomID(roomID)&0x7fff_ffff_ffff_ffff)) //nolint:gosec // G115：上式已保证最高位清零
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

	var out []Notification
	receivedTiles := exchangeThree(hands)
	exchangePayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "exchange",
		Body: &clientv1.Envelope_ExchangeThreeDone{
			ExchangeThreeDone: &clientv1.ExchangeThreeDoneNotify{PerSeat: receivedTiles},
		},
	})
	if err != nil {
		return nil, err
	}
	out = append(out, Notification{Kind: KindExchangeThreeDone, Payload: exchangePayload})

	queBySeat := make([]int32, 4)
	for seat := 0; seat < 4; seat++ {
		queBySeat[seat] = int32(chooseQueSuit(hands[seat]))
	}
	quePayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "que-men",
		Body: &clientv1.Envelope_QueMenDone{
			QueMenDone: &clientv1.QueMenDoneNotify{QueSuitBySeat: queBySeat},
		},
	})
	if err != nil {
		return nil, err
	}
	out = append(out, Notification{Kind: KindQueMenDone, Payload: quePayload})

	startPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "start",
		Body: &clientv1.Envelope_StartGame{
			StartGame: &clientv1.StartGameNotify{RoomId: roomID, DealerSeat: 0},
		},
	})
	if err != nil {
		return nil, err
	}
	out = append(out, Notification{Kind: KindStartGame, Payload: startPayload})

	ledger := make([]sichuanxzdd.ScoreEntry, 0, 8)
	winnerSeat := -1
	turn := 0
	for step := 0; step < autoRoundStepLimit && w.Remaining() > 0; step++ {
		drawn, err := w.Draw()
		if err != nil {
			return nil, err
		}
		seatIndex := int32(turn) //nolint:gosec // G115：turn 仅在 0..3 间轮转
		drawPayload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("draw-%d", step),
			Body: &clientv1.Envelope_DrawTile{
				DrawTile: &clientv1.DrawTileNotify{SeatIndex: seatIndex, Tile: drawn.String()},
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, Notification{Kind: KindDrawTile, Payload: drawPayload})

		if result, ok := rule.CheckHu(hands[turn], drawn, rules.HuContext{}); ok {
			breakdown := rule.ScoreFans(result, rules.ScoreContext{
				HuSeat:        turn,
				DealerSeat:    0,
				IsTsumo:       true,
				IsOpeningDraw: step == 0 && turn == 0,
			})
			for other := 0; other < 4; other++ {
				if other == turn {
					continue
				}
				ledger = append(ledger, huScoreEntry(sichuanxzdd.ReasonHuTsumo, other, turn, int32(breakdown.Total), step, turn, fanLabels(breakdown))) //nolint:gosec // 番数极小
			}
			winnerSeat = turn
			huPayload, err := marshalEnvelope(&clientv1.Envelope{
				ReqId: fmt.Sprintf("hu-%d", step),
				Body: &clientv1.Envelope_Action{
					Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "hu", Tile: drawn.String()},
				},
			})
			if err != nil {
				return nil, err
			}
			out = append(out, Notification{Kind: KindAction, Payload: huPayload})
			break
		}

		hands[turn].Add(drawn)
		discard := chooseDiscard(hands[turn], tile.Suit(queBySeat[turn]))
		if err := hands[turn].Remove(discard); err != nil {
			return nil, err
		}
		actionPayload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("discard-%d", step),
			Body: &clientv1.Envelope_Action{
				Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "discard", Tile: discard.String()},
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, Notification{Kind: KindAction, Payload: actionPayload})
		turn = (turn + 1) % 4
	}

	var winnerSeats []int
	if winnerSeat >= 0 {
		winnerSeats = append(winnerSeats, winnerSeat)
	}
	seatScores, penalties, breakdowns, detail := sichuanxzdd.BuildSettlement(playerIDs, hands, queBySeat, ledger, winnerSeats)
	settlementPayload, err := buildSettlementNotification(roomID, playerIDs, winnerSeats, seatScores, penalties, breakdowns, detail)
	if err != nil {
		return nil, err
	}
	out = append(out, Notification{Kind: KindSettlement, Payload: settlementPayload})
	return out, nil
}
