package game

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"

	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	_ "racoo.cn/lsp/internal/mahjong/sichuanxzdd"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// Kind 表示 room/game 产出的局内通知种类，由上层适配为具体 msg_id。
type Kind string

const (
	KindExchangeThreeDone Kind = "exchange_three_done"
	KindQueMenDone        Kind = "que_men_done"
	KindStartGame         Kind = "start_game"
	KindDrawTile          Kind = "draw_tile"
	KindAction            Kind = "action"
	KindSettlement        Kind = "settlement"
)

// Notification 为服务层产出的通知载荷；payload 已是 client.v1.Envelope 的序列化结果。
type Notification struct {
	Kind    Kind
	Payload []byte
}

// Engine 负责在单房上下文内生成确定性的血战流程通知。
type Engine struct {
	ruleID string
}

// NewEngine 创建牌局引擎；ruleID 为空时回退到四川血战到底默认规则。
func NewEngine(ruleID string) *Engine {
	if ruleID == "" {
		ruleID = "sichuan_xzdd"
	}
	return &Engine{ruleID: ruleID}
}

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

	totalFanBySeat := make([]int32, 4)
	winnerSeat := -1
	turn := 0
	// 自动回放只需覆盖核心换三张/定缺/摸打/结算链路，步数过大只会放大 e2e 噪音。
	maxSteps := 16
	for step := 0; step < maxSteps && w.Remaining() > 0; step++ {
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
			fanTotal := rule.ScoreFans(result, rules.ScoreContext{}).Total
			totalFanBySeat[turn] = int32(fanTotal) //nolint:gosec // G115：当前番型总和极小，远低于 int32 上限
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

	seatScores, penalties, detail := buildSettlement(playerIDs, hands, tile.Suit(queBySeat[0]), queBySeat, winnerSeat, totalFanBySeat)
	winnerIDs := make([]string, 0, 1)
	if winnerSeat >= 0 {
		winnerIDs = append(winnerIDs, playerIDs[winnerSeat])
	}
	settlementPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "settlement",
		Body: &clientv1.Envelope_Settlement{
			Settlement: &clientv1.SettlementNotify{
				RoomId:        roomID,
				WinnerUserIds: winnerIDs,
				TotalFan:      sumInt32(totalFanBySeat),
				SeatScores:    seatScores,
				Penalties:     penalties,
				DetailText:    detail,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	out = append(out, Notification{Kind: KindSettlement, Payload: settlementPayload})
	return out, nil
}

func marshalEnvelope(env *clientv1.Envelope) ([]byte, error) {
	if env == nil {
		return nil, fmt.Errorf("nil envelope")
	}
	return proto.Marshal(env)
}

func seedFromRoomID(roomID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(roomID))
	return h.Sum64()
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
		from := (seat + 3) % 4
		seatIndex := int32(seat) //nolint:gosec // G115：seat 仅在 0..3 范围
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

func chooseDiscard(h *hand.Hand, queSuit tile.Suit) tile.Tile {
	ts := append([]tile.Tile(nil), h.Tiles()...)
	sort.Slice(ts, func(i, j int) bool {
		if ts[i].Suit() != ts[j].Suit() {
			return ts[i].Suit() < ts[j].Suit()
		}
		return ts[i].Rank() > ts[j].Rank()
	})
	for _, t := range ts {
		if t.Suit() == queSuit {
			return t
		}
	}
	return ts[0]
}

func tilesToStrings(ts []tile.Tile) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.String())
	}
	return out
}

func buildSettlement(playerIDs [4]string, hands []*hand.Hand, _ tile.Suit, queBySeat []int32, winnerSeat int, totalFanBySeat []int32) ([]*clientv1.SeatScore, []*clientv1.PenaltyItem, string) {
	seatScores := make([]*clientv1.SeatScore, 0, 4)
	penalties := make([]*clientv1.PenaltyItem, 0)
	detail := "荒牌"
	if winnerSeat >= 0 {
		detail = fmt.Sprintf("座位 %d 自摸", winnerSeat)
	}
	for seat := 0; seat < 4; seat++ {
		seatIndex := int32(seat) //nolint:gosec // G115：seat 仅在 0..3 范围
		skipped := false
		if holdsQueSuit(hands[seat], tile.Suit(queBySeat[seat])) {
			skipped = true
			for to := 0; to < 4; to++ {
				toSeat := int32(to) //nolint:gosec // G115：to 仅在 0..3 范围
				if to == seat {
					continue
				}
				penalties = append(penalties, &clientv1.PenaltyItem{
					Reason:   "查花猪",
					FromSeat: seatIndex,
					ToSeat:   toSeat,
					Amount:   2,
				})
			}
		}
		if winnerSeat >= 0 && seat != winnerSeat && totalFanBySeat[seat] == 0 {
			winnerSeat32 := int32(winnerSeat) //nolint:gosec // G115：winnerSeat 仅可能为 0..3
			penalties = append(penalties, &clientv1.PenaltyItem{
				Reason:   "查大叫",
				FromSeat: seatIndex,
				ToSeat:   winnerSeat32,
				Amount:   1,
			})
		}
		seatScores = append(seatScores, &clientv1.SeatScore{
			SeatIndex: seatIndex,
			UserId:    playerIDs[seat],
			TotalFan:  totalFanBySeat[seat],
			Skipped:   skipped,
		})
	}
	return seatScores, penalties, detail
}

func holdsQueSuit(h *hand.Hand, queSuit tile.Suit) bool {
	for _, t := range h.Tiles() {
		if t.Suit() == queSuit {
			return true
		}
	}
	return false
}

func sumInt32(xs []int32) int32 {
	var total int32
	for _, x := range xs {
		total += x
	}
	return total
}
