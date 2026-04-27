package room

import (
	"context"
	"fmt"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// ApplyQueMen 记录某座位的定缺确认；四家齐备后开局并摸首张牌。
func (e *Engine) ApplyQueMen(ctx context.Context, rs *RoundState, seat int, suit int32) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, fmt.Errorf("round closed")
	}
	if !rs.waitingQueMen {
		return nil, fmt.Errorf("que men not allowed")
	}
	if seat < 0 || seat > 3 {
		return nil, fmt.Errorf("invalid seat")
	}
	if rs.queSubmitted[seat] {
		return nil, fmt.Errorf("que men already submitted")
	}
	if suit >= 0 && suit <= 2 {
		rs.queBySeat[seat] = suit
	} else {
		rs.queBySeat[seat] = int32(chooseQueSuit(rs.hands[seat]))
	}
	rs.queSubmitted[seat] = true
	for _, done := range rs.queSubmitted {
		if !done {
			return nil, nil
		}
	}
	rs.waitingQueMen = false
	quePayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "que-men",
		Body: &clientv1.Envelope_QueMenDone{
			QueMenDone: &clientv1.QueMenDoneNotify{QueSuitBySeat: rs.queBySeat},
		},
	})
	if err != nil {
		return nil, err
	}
	startPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "start",
		Body: &clientv1.Envelope_StartGame{
			StartGame: &clientv1.StartGameNotify{RoomId: rs.roomID, DealerSeat: int32(rs.dealerSeat)}, //nolint:gosec // 座位范围固定
		},
	})
	if err != nil {
		return nil, err
	}
	out := []Notification{
		{Kind: KindQueMenDone, Payload: quePayload},
		{Kind: KindStartGame, Payload: startPayload},
	}
	next, err := e.drawForCurrentTurn(rs)
	if err != nil {
		return nil, err
	}
	_ = ctx
	return append(out, next...), nil
}

func chooseQueSuit(h *hand.Hand) tile.Suit {
	counts := map[tile.Suit]int{
		tile.SuitCharacters: 0,
		tile.SuitDots:       0,
		tile.SuitBamboo:     0,
	}
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
