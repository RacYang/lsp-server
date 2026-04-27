package room

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"

	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	_ "racoo.cn/lsp/internal/mahjong/sichuanxzdd"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

// Kind 表示 room 服务产出的局内通知种类，由 handler 适配为具体 msg_id。
type Kind string

const (
	KindExchangeThreeDone Kind = "exchange_three_done"
	KindQueMenDone        Kind = "que_men_done"
	KindStartGame         Kind = "start_game"
	KindDrawTile          Kind = "draw_tile"
	KindAction            Kind = "action"
	KindSettlement        Kind = "settlement"
)

// Notification 为 room 服务产出的通知载荷；payload 已是 client.v1.Envelope 的序列化结果。
type Notification struct {
	Kind    Kind
	Payload []byte
}

// Engine 负责在单房上下文内生成确定性的血战流程通知。
type Engine struct {
	ruleID string
}

// RoundState 保存交互式单局运行态，仅在 room actor 内被串行访问。
type RoundState struct {
	roomID    string
	ruleID    string
	playerIDs [4]string
	rule      rules.Rule
	wall      *wall.Wall
	hands     []*hand.Hand
	queBySeat []int32

	waitingExchange   bool
	waitingQueMen     bool
	exchangeSubmitted []bool
	exchangeSelection [][]tile.Tile
	queSubmitted      []bool
	waitingDiscard    bool
	waitingTsumo      bool
	pendingDraw       tile.Tile
	currentDraw       tile.Tile
	lastDiscard       tile.Tile
	lastDiscardSeat   int
	claimWindowOpen   bool
	claimCandidates   []claimCandidate
	turn              int
	step              int
	maxSteps          int
	winnerSeat        int
	totalFanBySeat    []int32
	closed            bool
}

type claimCandidate struct {
	seat    int
	actions []string
}

type claimCandidatePersist struct {
	Seat    int      `json:"seat"`
	Actions []string `json:"actions"`
}

type roundPersist struct {
	RuleID          string                  `json:"rule_id"`
	PlayerIDs       [4]string               `json:"player_ids"`
	QueBySeat       []int32                 `json:"que_by_seat"`
	WaitingExchange bool                    `json:"waiting_exchange"`
	WaitingQueMen   bool                    `json:"waiting_que_men"`
	ExchangeDone    []bool                  `json:"exchange_done,omitempty"`
	ExchangeTiles   [][]string              `json:"exchange_tiles,omitempty"`
	QueDone         []bool                  `json:"que_done,omitempty"`
	Turn            int                     `json:"turn"`
	Step            int                     `json:"step"`
	MaxSteps        int                     `json:"max_steps"`
	WaitingDiscard  bool                    `json:"waiting_discard"`
	WaitingTsumo    bool                    `json:"waiting_tsumo"`
	PendingDraw     string                  `json:"pending_draw,omitempty"`
	CurrentDraw     string                  `json:"current_draw,omitempty"`
	LastDiscard     string                  `json:"last_discard,omitempty"`
	LastDiscardSeat int                     `json:"last_discard_seat"`
	ClaimWindowOpen bool                    `json:"claim_window_open,omitempty"`
	ClaimCandidates []claimCandidatePersist `json:"claim_candidates,omitempty"`
	WinnerSeat      int                     `json:"winner_seat"`
	TotalFanBySeat  []int32                 `json:"total_fan_by_seat"`
	Hands           [][]string              `json:"hands"`
	WallRemaining   []string                `json:"wall_remaining"`
}

// RoundView 描述客户端恢复时所需的最小等待态摘要。
type RoundView struct {
	ActingSeat       int32
	WaitingAction    string
	PendingTile      string
	AvailableActions []string
}

// NewEngine 创建牌局引擎；ruleID 为空时回退到四川血战到底默认规则。
func NewEngine(ruleID string) *Engine {
	if ruleID == "" {
		ruleID = "sichuan_xzdd"
	}
	return &Engine{ruleID: ruleID}
}

// StartRound 初始化交互式牌局，并推进到首个等待出牌的状态。
func (e *Engine) StartRound(ctx context.Context, roomID string, playerIDs [4]string) (*RoundState, []Notification, error) {
	if e == nil {
		return nil, nil, fmt.Errorf("nil engine")
	}
	rule := rules.MustGet(e.ruleID)
	rs := &RoundState{
		roomID:            roomID,
		ruleID:            e.ruleID,
		playerIDs:         playerIDs,
		rule:              rule,
		wall:              rule.BuildWall(ctx, int64(seedFromRoomID(roomID)&0x7fff_ffff_ffff_ffff)), //nolint:gosec // 已清零最高位
		hands:             make([]*hand.Hand, 4),
		queBySeat:         make([]int32, 4),
		exchangeSubmitted: make([]bool, 4),
		exchangeSelection: make([][]tile.Tile, 4),
		queSubmitted:      make([]bool, 4),
		maxSteps:          16,
		lastDiscardSeat:   -1,
		winnerSeat:        -1,
		totalFanBySeat:    make([]int32, 4),
	}
	for i := range rs.hands {
		rs.hands[i] = hand.New()
	}
	for round := 0; round < 13; round++ {
		for seat := 0; seat < 4; seat++ {
			t, err := rs.wall.Draw()
			if err != nil {
				return nil, nil, err
			}
			rs.hands[seat].Add(t)
		}
	}

	out, err := rs.initRoundNotifications()
	if err != nil {
		return nil, nil, err
	}
	return rs, out, nil
}

// ApplyExchangeThree 记录某座位已完成换三张确认；四家齐备后再统一换牌并进入定缺阶段。
func (e *Engine) ApplyExchangeThree(_ context.Context, rs *RoundState, seat int, tiles []string) ([]Notification, error) {
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
	rs.exchangeSelection[seat] = normalizeExchangeSelection(rs.hands[seat], tiles)
	rs.exchangeSubmitted[seat] = true
	for _, done := range rs.exchangeSubmitted {
		if !done {
			return nil, nil
		}
	}
	rs.waitingExchange = false
	receivedTiles := exchangeThreeWithSelections(rs.hands, rs.exchangeSelection)
	for i := range rs.exchangeSelection {
		rs.exchangeSelection[i] = nil
	}
	exchangePayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "exchange",
		Body: &clientv1.Envelope_ExchangeThreeDone{
			ExchangeThreeDone: &clientv1.ExchangeThreeDoneNotify{PerSeat: receivedTiles},
		},
	})
	if err != nil {
		return nil, err
	}
	rs.waitingQueMen = true
	out := []Notification{{Kind: KindExchangeThreeDone, Payload: exchangePayload}}
	return append(out, rs.promptSeatActions("que_men")...), nil
}

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
	_ = suit
	rs.queBySeat[seat] = int32(chooseQueSuit(rs.hands[seat]))
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
			StartGame: &clientv1.StartGameNotify{RoomId: rs.roomID, DealerSeat: 0},
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

// ApplyDiscard 推进当前轮次出牌，并在需要时继续发出下一次摸牌或结算。
func (e *Engine) ApplyDiscard(ctx context.Context, rs *RoundState, seat int, tileText string) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, fmt.Errorf("round closed")
	}
	if seat != rs.turn {
		return nil, fmt.Errorf("not your turn")
	}
	discard, err := tile.Parse(tileText)
	if err != nil {
		return nil, fmt.Errorf("parse discard tile: %w", err)
	}
	if rs.waitingTsumo {
		rs.hands[seat].Add(rs.pendingDraw)
		rs.pendingDraw = 0
		rs.waitingTsumo = false
		rs.waitingDiscard = true
	}
	if !rs.waitingDiscard {
		return nil, fmt.Errorf("round not waiting discard")
	}
	if err := rs.hands[seat].Remove(discard); err != nil {
		return nil, fmt.Errorf("discard tile from hand: %w", err)
	}
	rs.waitingDiscard = false
	rs.currentDraw = 0
	seatIndex := int32(seat) //nolint:gosec // seat 范围固定
	actionPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("discard-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "discard", Tile: discard.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	out := []Notification{{Kind: KindAction, Payload: actionPayload}}
	rs.step++
	if rs.step >= rs.maxSteps || rs.wall.Remaining() <= 0 {
		settlement, err := rs.finishRound()
		if err != nil {
			return nil, err
		}
		out = append(out, settlement)
		return out, nil
	}
	rs.lastDiscard = discard
	rs.lastDiscardSeat = seat
	rs.turn = (rs.turn + 1) % 4
	rs.openClaimWindow()
	if len(rs.claimCandidates) > 0 {
		claimPrompts, err := rs.claimPromptNotifications(discard)
		if err != nil {
			return nil, err
		}
		return append(out, claimPrompts...), nil
	}
	rs.clearClaimWindow()
	next, err := e.drawForCurrentTurn(rs)
	if err != nil {
		return nil, err
	}
	_ = ctx
	return append(out, next...), nil
}

// ApplyHu 处理当前轮次的自摸胡牌。
func (e *Engine) ApplyHu(ctx context.Context, rs *RoundState, seat int) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, fmt.Errorf("round closed")
	}
	if seat != rs.turn {
		return nil, fmt.Errorf("not your turn")
	}
	if !rs.waitingTsumo {
		return nil, fmt.Errorf("hu not allowed")
	}
	result, ok := rs.rule.CheckHu(rs.hands[seat], rs.pendingDraw, rules.HuContext{})
	if !ok {
		return nil, fmt.Errorf("hu not allowed")
	}
	drawn := rs.pendingDraw
	fanTotal := rs.rule.ScoreFans(result, rules.ScoreContext{}).Total
	rs.totalFanBySeat[seat] = int32(fanTotal) //nolint:gosec // 番数极小
	rs.winnerSeat = seat
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.waitingTsumo = false
	rs.waitingDiscard = false
	seatIndex := int32(seat) //nolint:gosec // seat 范围固定
	huPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("hu-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "hu", Tile: drawn.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	settlement, err := rs.finishRound()
	if err != nil {
		return nil, err
	}
	_ = ctx
	return []Notification{
		{Kind: KindAction, Payload: huPayload},
		settlement,
	}, nil
}

// ApplyPong 处理弃牌抢答窗口中的碰牌动作，并中断原本轮到的座位。
func (e *Engine) ApplyPong(_ context.Context, rs *RoundState, seat int) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if seat < 0 || seat > 3 {
		return nil, fmt.Errorf("invalid seat")
	}
	if !rs.canClaimPong(seat) {
		return nil, fmt.Errorf("pong not allowed")
	}
	claimedTile := rs.lastDiscard
	rs.clearClaimWindow()
	if err := rs.rewindInterruptedTurn(); err != nil {
		return nil, err
	}
	for i := 0; i < 2; i++ {
		if err := rs.hands[seat].Remove(claimedTile); err != nil {
			return nil, fmt.Errorf("consume pong tiles: %w", err)
		}
	}
	rs.turn = seat
	rs.waitingDiscard = true
	rs.waitingTsumo = false
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.lastDiscard = 0
	rs.lastDiscardSeat = -1
	seatIndex := int32(seat) //nolint:gosec // seat 范围固定
	payload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("pong-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "pong", Tile: claimedTile.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	out := []Notification{{Kind: KindAction, Payload: payload}}
	discard := chooseDiscard(rs.hands[seat], tile.Suit(rs.queBySeat[seat]))
	next, err := e.ApplyDiscard(context.Background(), rs, seat, discard.String())
	if err != nil {
		return nil, err
	}
	return append(out, next...), nil
}

// ApplyGang 处理弃牌抢杠或当前座位自杠，并继续摸补牌。
func (e *Engine) ApplyGang(_ context.Context, rs *RoundState, seat int, tileText string) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if seat < 0 || seat > 3 {
		return nil, fmt.Errorf("invalid seat")
	}
	claimGang := rs.canClaimGang(seat)
	selfGang := rs.canSelfGang(seat, tileText)
	if !claimGang && !selfGang {
		return nil, fmt.Errorf("gang not allowed")
	}
	var gangTile tile.Tile
	var err error
	var out []Notification
	if claimGang {
		gangTile = rs.lastDiscard
		rs.clearClaimWindow()
		if err := rs.rewindInterruptedTurn(); err != nil {
			return nil, err
		}
		for i := 0; i < 3; i++ {
			if err := rs.hands[seat].Remove(gangTile); err != nil {
				return nil, fmt.Errorf("consume gang tiles: %w", err)
			}
		}
		rs.lastDiscard = 0
		rs.lastDiscardSeat = -1
	} else {
		gangTile, err = tile.Parse(tileText)
		if err != nil {
			return nil, fmt.Errorf("parse gang tile: %w", err)
		}
		for i := 0; i < 4; i++ {
			if err := rs.hands[seat].Remove(gangTile); err != nil {
				return nil, fmt.Errorf("consume self gang tiles: %w", err)
			}
		}
	}
	rs.turn = seat
	rs.waitingDiscard = false
	rs.waitingTsumo = false
	rs.pendingDraw = 0
	rs.currentDraw = 0
	seatIndex := int32(seat) //nolint:gosec // seat 范围固定
	payload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("gang-%d", rs.step),
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "gang", Tile: gangTile.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	out = append(out, Notification{Kind: KindAction, Payload: payload})
	next, err := e.drawForCurrentTurn(rs)
	if err != nil {
		return nil, err
	}
	return append(out, next...), nil
}

// ApplyTimeout 执行服务端超时/托管动作；调用方可在定时器到期后提交到同一 room actor。
func (e *Engine) ApplyTimeout(ctx context.Context, rs *RoundState) ([]Notification, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, fmt.Errorf("round closed")
	}
	if rs.waitingExchange {
		for seat, done := range rs.exchangeSubmitted {
			if !done {
				return e.ApplyExchangeThree(ctx, rs, seat, nil)
			}
		}
	}
	if rs.waitingQueMen {
		for seat, done := range rs.queSubmitted {
			if !done {
				return e.ApplyQueMen(ctx, rs, seat, 0)
			}
		}
	}
	if rs.claimWindowOpen {
		candidate, ok := rs.bestClaimCandidate()
		if !ok {
			rs.clearClaimWindow()
			return e.drawForCurrentTurn(rs)
		}
		if hasAction(candidate.actions, "gang") {
			return e.ApplyGang(ctx, rs, candidate.seat, rs.lastDiscard.String())
		}
		return e.ApplyPong(ctx, rs, candidate.seat)
	}
	if rs.waitingTsumo {
		return e.ApplyDiscard(ctx, rs, rs.turn, rs.pendingDraw.String())
	}
	if rs.waitingDiscard {
		discard := chooseDiscard(rs.hands[rs.turn], tile.Suit(rs.queBySeat[rs.turn]))
		return e.ApplyDiscard(ctx, rs, rs.turn, discard.String())
	}
	return nil, fmt.Errorf("round not waiting for action")
}

func (rs *RoundState) initRoundNotifications() ([]Notification, error) {
	rs.waitingExchange = true
	return rs.promptSeatActions("exchange_three"), nil
}

func (rs *RoundState) canClaimPong(seat int) bool {
	return rs != nil && rs.isTopClaimSeat(seat) && rs.hasClaimAction(seat, "pong")
}

func (rs *RoundState) canClaimGang(seat int) bool {
	return rs != nil && rs.isTopClaimSeat(seat) && rs.hasClaimAction(seat, "gang")
}

func (rs *RoundState) canSelfGang(seat int, tileText string) bool {
	if rs == nil || seat != rs.turn || !rs.waitingDiscard {
		return false
	}
	target, err := tile.Parse(tileText)
	if err != nil {
		return false
	}
	count := 0
	for _, t := range rs.hands[seat].Tiles() {
		if t == target {
			count++
		}
	}
	return count >= 4
}

func (rs *RoundState) rewindInterruptedTurn() error {
	if rs == nil || rs.currentDraw == 0 {
		return nil
	}
	if rs.waitingTsumo {
		rs.pendingDraw = 0
		rs.waitingTsumo = false
	} else if rs.waitingDiscard {
		if err := rs.hands[rs.turn].Remove(rs.currentDraw); err != nil {
			return fmt.Errorf("rewind current draw: %w", err)
		}
		rs.waitingDiscard = false
	}
	if err := rs.wall.PushFront(rs.currentDraw); err != nil {
		return fmt.Errorf("restore draw to wall: %w", err)
	}
	rs.currentDraw = 0
	return nil
}

func (rs *RoundState) snapshotWaiting() (int32, string, string, []string) {
	if rs == nil {
		return -1, "", "", nil
	}
	if rs.waitingExchange {
		for seat, done := range rs.exchangeSubmitted {
			if !done {
				return int32(seat), "exchange_three", "", []string{"exchange_three"} //nolint:gosec // 座位范围固定
			}
		}
	}
	if rs.waitingQueMen {
		for seat, done := range rs.queSubmitted {
			if !done {
				return int32(seat), "que_men", "", []string{"que_men"} //nolint:gosec // 座位范围固定
			}
		}
	}
	if seat := rs.claimSeat(); seat >= 0 {
		actions := make([]string, 0, 2)
		if rs.hasClaimAction(seat, "gang") {
			actions = append(actions, "gang")
		}
		if rs.hasClaimAction(seat, "pong") {
			actions = append(actions, "pong")
		}
		if len(actions) > 0 {
			return int32(seat), "claim", rs.lastDiscard.String(), actions //nolint:gosec // 座位范围固定
		}
	}
	if rs.waitingTsumo {
		return int32(rs.turn), "tsumo_choice", rs.pendingDraw.String(), []string{"hu", "discard"} //nolint:gosec // 座位范围固定
	}
	if rs.waitingDiscard {
		actions := []string{"discard"}
		for _, t := range rs.hands[rs.turn].Tiles() {
			if rs.canSelfGang(rs.turn, t.String()) {
				actions = append(actions, "gang")
				break
			}
		}
		return int32(rs.turn), "discard", "", actions //nolint:gosec // 座位范围固定
	}
	return -1, "", "", nil
}

func (rs *RoundState) claimSeat() int {
	candidate, ok := rs.bestClaimCandidate()
	if !ok {
		return -1
	}
	return candidate.seat
}

func (rs *RoundState) openClaimWindow() {
	if rs == nil {
		return
	}
	rs.claimWindowOpen = true
	rs.claimCandidates = rs.buildClaimCandidates()
}

func (rs *RoundState) clearClaimWindow() {
	if rs == nil {
		return
	}
	rs.claimWindowOpen = false
	rs.claimCandidates = nil
}

func (rs *RoundState) buildClaimCandidates() []claimCandidate {
	if rs == nil || rs.lastDiscard == 0 || rs.lastDiscardSeat < 0 {
		return nil
	}
	out := make([]claimCandidate, 0, 3)
	for offset := 1; offset < 4; offset++ {
		seat := (rs.lastDiscardSeat + offset) % 4
		actions := make([]string, 0, 2)
		if rs.rawCanClaimGang(seat) {
			actions = append(actions, "gang")
		}
		if rs.rawCanClaimPong(seat) {
			actions = append(actions, "pong")
		}
		if len(actions) > 0 {
			out = append(out, claimCandidate{seat: seat, actions: actions})
		}
	}
	return out
}

func (rs *RoundState) bestClaimCandidate() (claimCandidate, bool) {
	if rs == nil || !rs.claimWindowOpen || len(rs.claimCandidates) == 0 {
		return claimCandidate{}, false
	}
	best := rs.claimCandidates[0]
	bestPriority := claimPriority(best.actions)
	for _, candidate := range rs.claimCandidates[1:] {
		priority := claimPriority(candidate.actions)
		if priority > bestPriority {
			best = candidate
			bestPriority = priority
		}
	}
	return best, true
}

func (rs *RoundState) isTopClaimSeat(seat int) bool {
	candidate, ok := rs.bestClaimCandidate()
	return ok && candidate.seat == seat
}

func (rs *RoundState) hasClaimAction(seat int, action string) bool {
	if rs == nil || !rs.claimWindowOpen {
		return false
	}
	for _, candidate := range rs.claimCandidates {
		if candidate.seat == seat {
			return hasAction(candidate.actions, action)
		}
	}
	return false
}

func claimPriority(actions []string) int {
	if hasAction(actions, "gang") {
		return 2
	}
	if hasAction(actions, "pong") {
		return 1
	}
	return 0
}

func hasAction(actions []string, action string) bool {
	for _, current := range actions {
		if current == action {
			return true
		}
	}
	return false
}

func (rs *RoundState) claimPromptNotifications(discard tile.Tile) ([]Notification, error) {
	out := make([]Notification, 0, len(rs.claimCandidates))
	for _, candidate := range rs.claimCandidates {
		claimAction := "pong_choice"
		if hasAction(candidate.actions, "gang") {
			claimAction = "gang_choice"
		}
		claimSeatIndex := int32(candidate.seat) //nolint:gosec // 座位范围固定
		claimPayload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("claim-%d-%d", rs.step, candidate.seat),
			Body: &clientv1.Envelope_Action{
				Action: &clientv1.ActionNotify{SeatIndex: claimSeatIndex, Action: claimAction, Tile: discard.String()},
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, Notification{Kind: KindAction, Payload: claimPayload})
	}
	return out, nil
}

func (rs *RoundState) rawCanClaimPong(seat int) bool {
	if rs == nil || rs.lastDiscard == 0 || rs.lastDiscardSeat < 0 || seat == rs.lastDiscardSeat {
		return false
	}
	count := 0
	for _, t := range rs.hands[seat].Tiles() {
		if t == rs.lastDiscard {
			count++
		}
	}
	return count >= 2
}

func (rs *RoundState) rawCanClaimGang(seat int) bool {
	if rs == nil || rs.lastDiscard == 0 || rs.lastDiscardSeat < 0 || seat == rs.lastDiscardSeat {
		return false
	}
	count := 0
	for _, t := range rs.hands[seat].Tiles() {
		if t == rs.lastDiscard {
			count++
		}
	}
	return count >= 3
}

// SnapshotView 返回当前局面的最小等待态摘要。
func (rs *RoundState) SnapshotView() RoundView {
	actingSeat, waitingAction, pendingTile, available := rs.snapshotWaiting()
	return RoundView{
		ActingSeat:       actingSeat,
		WaitingAction:    waitingAction,
		PendingTile:      pendingTile,
		AvailableActions: append([]string(nil), available...),
	}
}

func (rs *RoundState) promptSeatActions(action string) []Notification {
	out := make([]Notification, 0, 4)
	for seat := 0; seat < 4; seat++ {
		seatIndex := int32(seat) //nolint:gosec // 座位范围固定
		payload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("%s-%d", action, seat),
			Body: &clientv1.Envelope_Action{
				Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: action},
			},
		})
		if err != nil {
			continue
		}
		out = append(out, Notification{Kind: KindAction, Payload: payload})
	}
	return out
}

func (e *Engine) drawForCurrentTurn(rs *RoundState) ([]Notification, error) {
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.wall.Remaining() <= 0 {
		settlement, err := rs.finishRound()
		if err != nil {
			return nil, err
		}
		return []Notification{settlement}, nil
	}
	drawn, err := rs.wall.Draw()
	if err != nil {
		return nil, err
	}
	seatIndex := int32(rs.turn) //nolint:gosec // turn 范围固定
	drawPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: fmt.Sprintf("draw-%d", rs.step),
		Body: &clientv1.Envelope_DrawTile{
			DrawTile: &clientv1.DrawTileNotify{SeatIndex: seatIndex, Tile: drawn.String()},
		},
	})
	if err != nil {
		return nil, err
	}
	rs.currentDraw = drawn
	out := []Notification{{Kind: KindDrawTile, Payload: drawPayload}}
	if _, ok := rs.rule.CheckHu(rs.hands[rs.turn], drawn, rules.HuContext{}); ok {
		rs.pendingDraw = drawn
		rs.waitingTsumo = true
		rs.waitingDiscard = false
		choicePayload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("tsumo-choice-%d", rs.step),
			Body: &clientv1.Envelope_Action{
				Action: &clientv1.ActionNotify{SeatIndex: seatIndex, Action: "tsumo_choice", Tile: drawn.String()},
			},
		})
		if err != nil {
			return nil, err
		}
		return append(out, Notification{Kind: KindAction, Payload: choicePayload}), nil
	}
	rs.hands[rs.turn].Add(drawn)
	rs.waitingDiscard = true
	return out, nil
}

func (rs *RoundState) finishRound() (Notification, error) {
	seatScores, penalties, detail := buildSettlement(rs.playerIDs, rs.hands, rs.queBySeat, rs.winnerSeat, rs.totalFanBySeat)
	winnerIDs := make([]string, 0, 1)
	if rs.winnerSeat >= 0 {
		winnerIDs = append(winnerIDs, rs.playerIDs[rs.winnerSeat])
	}
	settlementPayload, err := marshalEnvelope(&clientv1.Envelope{
		ReqId: "settlement",
		Body: &clientv1.Envelope_Settlement{
			Settlement: &clientv1.SettlementNotify{
				RoomId:        rs.roomID,
				WinnerUserIds: winnerIDs,
				TotalFan:      sumInt32(rs.totalFanBySeat),
				SeatScores:    seatScores,
				Penalties:     penalties,
				DetailText:    detail,
			},
		},
	})
	if err != nil {
		return Notification{}, err
	}
	rs.closed = true
	rs.waitingDiscard = false
	rs.waitingTsumo = false
	rs.pendingDraw = 0
	rs.currentDraw = 0
	rs.lastDiscard = 0
	rs.lastDiscardSeat = -1
	rs.clearClaimWindow()
	return Notification{Kind: KindSettlement, Payload: settlementPayload}, nil
}

// MarshalRoundPersistJSON 将当前局内状态序列化为 JSON，供 Redis snapmeta 保存。
func (rs *RoundState) MarshalRoundPersistJSON() ([]byte, error) {
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	if rs.closed {
		return nil, nil
	}
	rp := roundPersist{
		RuleID:          rs.ruleID,
		PlayerIDs:       rs.playerIDs,
		QueBySeat:       append([]int32(nil), rs.queBySeat...),
		WaitingExchange: rs.waitingExchange,
		WaitingQueMen:   rs.waitingQueMen,
		ExchangeDone:    append([]bool(nil), rs.exchangeSubmitted...),
		QueDone:         append([]bool(nil), rs.queSubmitted...),
		Turn:            rs.turn,
		Step:            rs.step,
		MaxSteps:        rs.maxSteps,
		WaitingDiscard:  rs.waitingDiscard,
		WaitingTsumo:    rs.waitingTsumo,
		ClaimWindowOpen: rs.claimWindowOpen,
		WinnerSeat:      rs.winnerSeat,
		TotalFanBySeat:  append([]int32(nil), rs.totalFanBySeat...),
		Hands:           make([][]string, 4),
		ExchangeTiles:   make([][]string, 4),
	}
	if rs.claimWindowOpen {
		rp.ClaimCandidates = make([]claimCandidatePersist, 0, len(rs.claimCandidates))
		for _, candidate := range rs.claimCandidates {
			rp.ClaimCandidates = append(rp.ClaimCandidates, claimCandidatePersist{
				Seat:    candidate.seat,
				Actions: append([]string(nil), candidate.actions...),
			})
		}
	}
	if rs.pendingDraw != 0 {
		rp.PendingDraw = rs.pendingDraw.String()
	}
	if rs.currentDraw != 0 {
		rp.CurrentDraw = rs.currentDraw.String()
	}
	if rs.lastDiscard != 0 {
		rp.LastDiscard = rs.lastDiscard.String()
		rp.LastDiscardSeat = rs.lastDiscardSeat
	}
	for seat := 0; seat < 4; seat++ {
		var ts []tile.Tile
		if seat < len(rs.hands) && rs.hands[seat] != nil {
			ts = append([]tile.Tile(nil), rs.hands[seat].Tiles()...)
		}
		sort.Slice(ts, func(i, j int) bool { return ts[i].Index() < ts[j].Index() })
		rp.Hands[seat] = tilesToStrings(ts)
		if seat < len(rs.exchangeSelection) {
			rp.ExchangeTiles[seat] = tilesToStrings(rs.exchangeSelection[seat])
		}
	}
	if rs.wall != nil {
		rp.WallRemaining = tilesToStrings(rs.wall.Tiles())
	}
	return json.Marshal(rp)
}

// RestoreRoundFromPersistJSON 从 JSON 恢复进行中牌局的最小运行态。
func RestoreRoundFromPersistJSON(roomID string, data []byte) (*RoundState, error) {
	if roomID == "" {
		return nil, fmt.Errorf("empty room_id")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty round json")
	}
	var rp roundPersist
	if err := json.Unmarshal(data, &rp); err != nil {
		return nil, fmt.Errorf("unmarshal round json: %w", err)
	}
	ruleID := rp.RuleID
	if ruleID == "" {
		ruleID = "sichuan_xzdd"
	}
	rule := rules.MustGet(ruleID)
	wallTiles := make([]tile.Tile, 0, len(rp.WallRemaining))
	for _, raw := range rp.WallRemaining {
		t, err := tile.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse wall tile %q: %w", raw, err)
		}
		wallTiles = append(wallTiles, t)
	}
	hands := make([]*hand.Hand, 4)
	for seat := 0; seat < 4; seat++ {
		hands[seat] = hand.New()
		if seat >= len(rp.Hands) {
			continue
		}
		for _, raw := range rp.Hands[seat] {
			t, err := tile.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("parse hand tile %q: %w", raw, err)
			}
			hands[seat].Add(t)
		}
	}
	rs := &RoundState{
		roomID:            roomID,
		ruleID:            ruleID,
		playerIDs:         rp.PlayerIDs,
		rule:              rule,
		wall:              wall.NewFromOrderedTiles(wallTiles),
		hands:             hands,
		queBySeat:         append([]int32(nil), rp.QueBySeat...),
		waitingExchange:   rp.WaitingExchange,
		waitingQueMen:     rp.WaitingQueMen,
		exchangeSubmitted: append([]bool(nil), rp.ExchangeDone...),
		exchangeSelection: make([][]tile.Tile, 4),
		queSubmitted:      append([]bool(nil), rp.QueDone...),
		waitingDiscard:    rp.WaitingDiscard,
		waitingTsumo:      rp.WaitingTsumo,
		claimWindowOpen:   rp.ClaimWindowOpen,
		turn:              rp.Turn,
		step:              rp.Step,
		maxSteps:          rp.MaxSteps,
		winnerSeat:        rp.WinnerSeat,
		totalFanBySeat:    append([]int32(nil), rp.TotalFanBySeat...),
	}
	for len(rs.queBySeat) < 4 {
		rs.queBySeat = append(rs.queBySeat, 0)
	}
	for len(rs.exchangeSubmitted) < 4 {
		rs.exchangeSubmitted = append(rs.exchangeSubmitted, false)
	}
	for len(rs.exchangeSelection) < 4 {
		rs.exchangeSelection = append(rs.exchangeSelection, nil)
	}
	for len(rs.queSubmitted) < 4 {
		rs.queSubmitted = append(rs.queSubmitted, false)
	}
	for len(rs.totalFanBySeat) < 4 {
		rs.totalFanBySeat = append(rs.totalFanBySeat, 0)
	}
	if rs.maxSteps <= 0 {
		rs.maxSteps = 16
	}
	if rp.PendingDraw != "" {
		t, err := tile.Parse(rp.PendingDraw)
		if err != nil {
			return nil, fmt.Errorf("parse pending draw: %w", err)
		}
		rs.pendingDraw = t
	}
	if rp.CurrentDraw != "" {
		t, err := tile.Parse(rp.CurrentDraw)
		if err != nil {
			return nil, fmt.Errorf("parse current draw: %w", err)
		}
		rs.currentDraw = t
	}
	if rp.LastDiscard != "" {
		t, err := tile.Parse(rp.LastDiscard)
		if err != nil {
			return nil, fmt.Errorf("parse last discard: %w", err)
		}
		rs.lastDiscard = t
		rs.lastDiscardSeat = rp.LastDiscardSeat
	} else {
		rs.lastDiscardSeat = -1
	}
	for _, candidate := range rp.ClaimCandidates {
		if candidate.Seat < 0 || candidate.Seat > 3 {
			return nil, fmt.Errorf("invalid claim candidate seat: %d", candidate.Seat)
		}
		actions := make([]string, 0, len(candidate.Actions))
		for _, action := range candidate.Actions {
			switch action {
			case "gang", "pong":
				actions = append(actions, action)
			default:
				return nil, fmt.Errorf("invalid claim candidate action: %s", action)
			}
		}
		if len(actions) > 0 {
			rs.claimCandidates = append(rs.claimCandidates, claimCandidate{seat: candidate.Seat, actions: actions})
		}
	}
	if rs.claimWindowOpen && len(rs.claimCandidates) == 0 {
		rs.claimCandidates = rs.buildClaimCandidates()
	}
	if !rs.claimWindowOpen && rp.LastDiscard != "" && !rs.waitingDiscard && !rs.waitingTsumo {
		rs.claimWindowOpen = true
		rs.claimCandidates = rs.buildClaimCandidates()
	}
	for seat := 0; seat < len(rp.ExchangeTiles) && seat < 4; seat++ {
		for _, raw := range rp.ExchangeTiles[seat] {
			t, err := tile.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("parse exchange tile %q: %w", raw, err)
			}
			rs.exchangeSelection[seat] = append(rs.exchangeSelection[seat], t)
		}
	}
	return rs, nil
}

// RoundViewFromPersistJSON 直接从持久化 JSON 还原等待态摘要，供快照 fallback 使用。
func RoundViewFromPersistJSON(roomID string, data []byte) (RoundView, error) {
	rs, err := RestoreRoundFromPersistJSON(roomID, data)
	if err != nil {
		return RoundView{}, err
	}
	return rs.SnapshotView(), nil
}

// QueSuitsFromPersistJSON 直接从持久化 JSON 读取定缺结果，供本地重连快照复用。
func QueSuitsFromPersistJSON(data []byte) ([]int32, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty round json")
	}
	var rp roundPersist
	if err := json.Unmarshal(data, &rp); err != nil {
		return nil, fmt.Errorf("unmarshal round json: %w", err)
	}
	return append([]int32(nil), rp.QueBySeat...), nil
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

	seatScores, penalties, detail := buildSettlement(playerIDs, hands, queBySeat, winnerSeat, totalFanBySeat)
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

func exchangeThreeWithSelections(hands []*hand.Hand, selections [][]tile.Tile) []*clientv1.SeatTiles {
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

func normalizeExchangeSelection(h *hand.Hand, raws []string) []tile.Tile {
	if h == nil || len(raws) != 3 {
		return chooseExchangeTiles(h)
	}
	tmp := hand.FromTiles(append([]tile.Tile(nil), h.Tiles()...))
	out := make([]tile.Tile, 0, 3)
	for _, raw := range raws {
		t, err := tile.Parse(raw)
		if err != nil {
			return chooseExchangeTiles(h)
		}
		if err := tmp.Remove(t); err != nil {
			return chooseExchangeTiles(h)
		}
		out = append(out, t)
	}
	return out
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

func buildSettlement(playerIDs [4]string, hands []*hand.Hand, queBySeat []int32, winnerSeat int, totalFanBySeat []int32) ([]*clientv1.SeatScore, []*clientv1.PenaltyItem, string) {
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
