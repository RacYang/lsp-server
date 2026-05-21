package xuezhandaodi

import (
	"testing"

	"github.com/stretchr/testify/require"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// NormalizeRuleState 规范化规则状态测试

func TestNormalizeRuleState(t *testing.T) {
	x := newRule(IDHuansanzhang, true)
	initial := x.InitialRuleState()
	normalized := x.NormalizeRuleState(initial)
	require.NotEmpty(t, normalized.Data)
}

// 开局策略换三张定缺测试

func TestOpeningPolicyInitialStateWithExchange(t *testing.T) {
	p := openingPolicy{withExchange: true}
	state := p.InitialState()
	require.NotEmpty(t, state.Data)
}

func TestOpeningPolicyCurrentStepWithExchange(t *testing.T) {
	p := openingPolicy{withExchange: true}
	state := p.InitialState()
	step, ok := p.CurrentStep(rules.OpeningContext{RuleState: state})
	require.True(t, ok)
	require.Equal(t, openingStepExchange, step.ID)
}

func TestOpeningPolicyCurrentStepWithoutExchange(t *testing.T) {
	p := openingPolicy{withExchange: false}
	state := p.InitialState()
	step, ok := p.CurrentStep(rules.OpeningContext{RuleState: state})
	require.True(t, ok)
	require.Equal(t, openingStepMissing, step.ID)
}

func TestOpeningPolicyApplyInvalidSeat(t *testing.T) {
	p := openingPolicy{withExchange: false}
	state := p.InitialState()
	ctx := rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: state},
		Seat:           -1,
		Action:         openingProtocolMissing,
	}
	_, err := p.Apply(ctx)
	require.Error(t, err)
}

func TestOpeningPolicyApplyWrongAction(t *testing.T) {
	p := openingPolicy{withExchange: false}
	state := p.InitialState()
	ctx := rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: state},
		Seat:           domainroom.SeatFromInt(0),
		Action:         "wrong_action",
	}
	_, err := p.Apply(ctx)
	require.Error(t, err)
}

func TestOpeningPolicyApplyMissingSuitAllSeats(t *testing.T) {
	p := openingPolicy{withExchange: false}
	state := p.InitialState()
	hands := makeTestHands()

	for seat := 0; seat < 4; seat++ {
		ctx := rules.OpeningActionContext{
			OpeningContext: rules.OpeningContext{RuleState: state, Hands: hands[:]},
			Seat:           domainroom.SeatFromInt(seat),
			Action:         openingProtocolMissing,
			Suit:           0,
		}
		result, err := p.Apply(ctx)
		require.NoError(t, err)
		state = result.RuleState
	}

	step, ok := p.CurrentStep(rules.OpeningContext{RuleState: state})
	require.False(t, ok)
	require.Nil(t, step)
}

func TestOpeningPolicyApplyMissingSuitAlreadySubmitted(t *testing.T) {
	p := openingPolicy{withExchange: false}
	state := p.InitialState()
	hands := makeTestHands()

	ctx := rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: state, Hands: hands[:]},
		Seat:           domainroom.SeatFromInt(0),
		Action:         openingProtocolMissing,
		Suit:           0,
	}
	result, err := p.Apply(ctx)
	require.NoError(t, err)

	// 再次提交同一座位
	ctx2 := rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: result.RuleState, Hands: hands[:]},
		Seat:           domainroom.SeatFromInt(0),
		Action:         openingProtocolMissing,
		Suit:           0,
	}
	_, err = p.Apply(ctx2)
	require.Error(t, err)
}

func TestOpeningPolicyApplyMissingSuitSurrender(t *testing.T) {
	p := openingPolicy{withExchange: false}
	state := p.InitialState()
	hands := makeTestHands()

	ctx := rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: state, Hands: hands[:]},
		Seat:           domainroom.SeatFromInt(0),
		Action:         openingProtocolMissing,
		Surrender:      true,
	}
	_, err := p.Apply(ctx)
	require.NoError(t, err)
}

func TestOpeningPolicyApplyMissingSuitDefaultChoice(t *testing.T) {
	p := openingPolicy{withExchange: false}
	state := p.InitialState()
	hands := makeTestHands()

	ctx := rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: state, Hands: hands[:]},
		Seat:           domainroom.SeatFromInt(0),
		Action:         openingProtocolMissing,
		Suit:           -1,
	}
	_, err := p.Apply(ctx)
	require.NoError(t, err)
}

func TestOpeningPolicyApplyExchangeWithSelections(t *testing.T) {
	p := openingPolicy{withExchange: true}
	state := p.InitialState()
	hands := makeTestHands()

	for seat := 0; seat < 4; seat++ {
		ctx := rules.OpeningActionContext{
			OpeningContext: rules.OpeningContext{RuleState: state, Hands: hands[:]},
			Seat:           domainroom.SeatFromInt(seat),
			Action:         openingProtocolExchange,
			Tiles:          []string{"m1", "m2", "m3"},
			Direction:      3,
		}
		result, err := p.Apply(ctx)
		require.NoError(t, err)
		state = result.RuleState
	}

	// 换三张完成后应该进入定缺
	step, ok := p.CurrentStep(rules.OpeningContext{RuleState: state})
	require.True(t, ok)
	require.Equal(t, openingStepMissing, step.ID)
}

func TestOpeningPolicyApplyExchangeAlreadySubmitted(t *testing.T) {
	p := openingPolicy{withExchange: true}
	state := p.InitialState()
	hands := makeTestHands()

	ctx := rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: state, Hands: hands[:]},
		Seat:           domainroom.SeatFromInt(0),
		Action:         openingProtocolExchange,
		Tiles:          []string{"m1", "m2", "m3"},
	}
	result, err := p.Apply(ctx)
	require.NoError(t, err)

	ctx2 := rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: result.RuleState, Hands: hands[:]},
		Seat:           domainroom.SeatFromInt(0),
		Action:         openingProtocolExchange,
		Tiles:          []string{"m1", "m2", "m3"},
	}
	_, err = p.Apply(ctx2)
	require.Error(t, err)
}

func TestOpeningPolicyApplyExchangeSurrender(t *testing.T) {
	p := openingPolicy{withExchange: true}
	state := p.InitialState()
	hands := makeTestHands()

	ctx := rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: state, Hands: hands[:]},
		Seat:           domainroom.SeatFromInt(0),
		Action:         openingProtocolExchange,
		Surrender:      true,
	}
	_, err := p.Apply(ctx)
	require.NoError(t, err)
}

// 川麻抢答策略测试

func TestClaimPolicyCandidatesHued(t *testing.T) {
	p := claimPolicy{}
	t1 := tile.Must(tile.SuitCharacters, 1)
	ctx := rules.ClaimContext{
		Hued:       true,
		Seat:       domainroom.SeatFromInt(1),
		SourceSeat: domainroom.SeatFromInt(0),
		Tile:       t1,
		Hand:       hand.FromTiles([]tile.Tile{t1, t1, t1}),
	}
	require.Nil(t, p.Candidates(ctx))
}

func TestClaimPolicyCandidatesMissingPong(t *testing.T) {
	p := claimPolicy{}
	t1 := tile.Must(tile.SuitCharacters, 1)
	state := newRule(IDHuansanzhang, true).InitialRuleState()
	// 定缺 m
	current := decodeRuleState(state)
	current.MissingSuits[1] = 0
	current.Submitted[openingStepMissing] = []bool{false, true, false, false}
	encoded := encodeRuleState(current)

	ctx := rules.ClaimContext{
		Seat:       domainroom.SeatFromInt(1),
		SourceSeat: domainroom.SeatFromInt(0),
		Tile:       t1,
		Hand:       hand.FromTiles([]tile.Tile{t1, t1, t1}),
		RuleState:  encoded,
	}
	actions := p.Candidates(ctx)
	// 缺门牌不能碰杠，应为空
	require.Empty(t, actions)
}

func TestClaimPolicyCandidatesNormalPong(t *testing.T) {
	p := claimPolicy{}
	t1 := tile.Must(tile.SuitDots, 1)
	state := newRule(IDHuansanzhang, true).InitialRuleState()

	ctx := rules.ClaimContext{
		Seat:       domainroom.SeatFromInt(1),
		SourceSeat: domainroom.SeatFromInt(0),
		Tile:       t1,
		Hand:       hand.FromTiles([]tile.Tile{t1, t1}),
		RuleState:  state,
	}
	actions := p.Candidates(ctx)
	require.NotEmpty(t, actions)
}

func TestClaimPolicyCandidatesQiangGangNoHu(t *testing.T) {
	p := claimPolicy{}
	t1 := tile.Must(tile.SuitDots, 5)
	state := newRule(IDHuansanzhang, true).InitialRuleState()

	ctx := rules.ClaimContext{
		Seat:            domainroom.SeatFromInt(2),
		SourceSeat:      domainroom.SeatFromInt(0),
		Tile:            t1,
		Hand:            hand.FromTiles([]tile.Tile{t1, t1, t1}),
		RuleState:       state,
		QiangGangWindow: true,
		CheckHu: func(h *hand.Hand, target tile.Tile, hc rules.HuContext) (rules.HuResult, bool) {
			return rules.HuResult{}, false
		},
	}
	actions := p.Candidates(ctx)
	require.Empty(t, actions)
}

// 自行动作策略测试

func TestSelfActionPolicyCanAnGangMissingSuit(t *testing.T) {
	p := selfActionPolicy{}
	t1 := tile.Must(tile.SuitCharacters, 1)
	state := newRule(IDHuansanzhang, true).InitialRuleState()
	current := decodeRuleState(state)
	current.MissingSuits[0] = 0
	current.Submitted[openingStepMissing] = []bool{true, false, false, false}
	encoded := encodeRuleState(current)

	ctx := rules.SelfActionContext{
		Seat:      domainroom.SeatFromInt(0),
		Tile:      t1,
		Hand:      hand.FromTiles([]tile.Tile{t1, t1, t1, t1}),
		RuleState: encoded,
	}
	require.False(t, p.CanAnGang(ctx))
}

func TestSelfActionPolicyCanBuGangMissingSuit(t *testing.T) {
	p := selfActionPolicy{}
	t1 := tile.Must(tile.SuitCharacters, 1)
	state := newRule(IDHuansanzhang, true).InitialRuleState()
	current := decodeRuleState(state)
	current.MissingSuits[0] = 0
	current.Submitted[openingStepMissing] = []bool{true, false, false, false}
	encoded := encodeRuleState(current)

	ctx := rules.SelfActionContext{
		Seat:      domainroom.SeatFromInt(0),
		Tile:      t1,
		Hand:      hand.FromTiles([]tile.Tile{t1}),
		RuleState: encoded,
		Melds:     []rules.MeldContext{{Kind: "pong", Tiles: []tile.Tile{t1, t1, t1}}},
	}
	require.False(t, p.CanBuGang(ctx))
}

// 计分策略特性开关测试

func TestScoringPolicyFeatureFlags(t *testing.T) {
	p := scoringPolicy{}
	flags := p.FeatureFlags()
	require.Contains(t, flags, "fan_breakdown")
}

// 牌张花色持有判断测试

func TestCountsHoldSuit(t *testing.T) {
	var c [34]int
	require.False(t, countsHoldSuit(c, tile.SuitCharacters))
	c[0] = 1
	require.True(t, countsHoldSuit(c, tile.SuitCharacters))
	require.False(t, countsHoldSuit(c, tile.SuitHonor))
}

// 结算策略测试

func TestSettlementPolicyFeatureFlags(t *testing.T) {
	p := settlementPolicy{}
	flags := p.FeatureFlags()
	require.Contains(t, flags, "score_events")
}

func TestSettlementPolicyBuildSettlementNoWinners(t *testing.T) {
	x := newRule(IDHuansanzhang, true)
	p := settlementPolicy{rule: x}
	hands := makeTestHands()
	ctx := rules.SettlementContext{
		PlayerIDs: [4]string{"u0", "u1", "u2", "u3"},
		Hands:     hands[:],
	}
	result := p.BuildSettlement(ctx)
	require.Equal(t, "荒牌", result.DetailText)
}

func TestSettlementPolicyBuildSettlementWithWinner(t *testing.T) {
	x := newRule(IDHuansanzhang, true)
	p := settlementPolicy{rule: x}
	playerIDs := [4]string{"u0", "u1", "u2", "u3"}
	hands := makeTestHands()
	ctx := rules.SettlementContext{
		PlayerIDs: playerIDs,
		Hands:     hands[:],
		WinEvents: []rules.WinEvent{{Seat: domainroom.SeatFromInt(1)}},
		ScoreEvents: []rules.ScoreEvent{
			{WinnerSeat: domainroom.SeatFromInt(1), FromSeat: domainroom.SeatFromInt(0), ToSeat: domainroom.SeatFromInt(1), Amount: 4},
		},
	}
	result := p.BuildSettlement(ctx)
	require.Equal(t, []string{"u1"}, result.WinnerUserIDs)
}

// 胡牌事件座位提取测试

func TestWinnerSeatsFromEventsDedup(t *testing.T) {
	events := []rules.WinEvent{
		{Seat: domainroom.SeatFromInt(0)},
		{Seat: domainroom.SeatFromInt(0)},
		{Seat: domainroom.SeatFromInt(2)},
	}
	seats := winnerSeatsFromEvents(events)
	require.Len(t, seats, 2)
}

// 终局策略特性开关测试

func TestTerminationPolicyFeatureFlags(t *testing.T) {
	p := terminationPolicy{}
	require.Contains(t, p.FeatureFlags(), "three_hued_or_wall_empty")
}

// 计分策略杠分测试

func TestScoringPolicyScoreGang(t *testing.T) {
	p := scoringPolicy{}
	t1 := tile.Must(tile.SuitCharacters, 1)
	sc := rules.GangScoreContext{
		Kind:        rules.GangKindAn,
		Seat:        domainroom.SeatFromInt(0),
		Tile:        t1,
		ActiveSeats: []domainroom.Seat{0, 1, 2, 3},
	}
	events, record := p.ScoreGang(sc)
	require.Len(t, events, 3)
	require.Equal(t, int32(2), events[0].Amount)
	require.Equal(t, t1, record.Tile)
}

func TestScoringPolicyScoreGangMing(t *testing.T) {
	p := scoringPolicy{}
	t1 := tile.Must(tile.SuitDots, 5)
	sc := rules.GangScoreContext{
		Kind:        rules.GangKindMing,
		Seat:        domainroom.SeatFromInt(1),
		FromSeat:    domainroom.SeatFromInt(0),
		Tile:        t1,
		ActiveSeats: []domainroom.Seat{0, 1, 2, 3},
	}
	events, _ := p.ScoreGang(sc)
	require.Len(t, events, 3)
	require.Equal(t, int32(1), events[0].Amount)
}

// 开局自动选择换三张牌测试

func TestChooseOpeningExchangeTilesNoTimeout(t *testing.T) {
	p := openingPolicy{withExchange: true}
	state := p.InitialState()
	hands := makeTestHands()

	// 不指定 tiles，让系统自动选择（allowFallback=false，raws 为空触发 error）
	// 触发 chooseOpeningExchangeTiles 需要 Timeout=true 且 Tiles 为空
	ctx := rules.OpeningActionContext{
		OpeningContext: rules.OpeningContext{RuleState: state, Hands: hands[:]},
		Seat:           domainroom.SeatFromInt(0),
		Action:         openingProtocolExchange,
		Tiles:          []string{},
		Timeout:        true,
	}
	_, err := p.Apply(ctx)
	require.NoError(t, err)
}

// makeTestHands 构造 4 手测试用牌（含万筒条各 4 张以上）。
func makeTestHands() [4]*hand.Hand {
	var hands [4]*hand.Hand
	for i := range hands {
		h := hand.New()
		h.Add(tile.Must(tile.SuitCharacters, 1))
		h.Add(tile.Must(tile.SuitCharacters, 2))
		h.Add(tile.Must(tile.SuitCharacters, 3))
		h.Add(tile.Must(tile.SuitDots, 1))
		h.Add(tile.Must(tile.SuitDots, 2))
		h.Add(tile.Must(tile.SuitDots, 3))
		h.Add(tile.Must(tile.SuitBamboo, 1))
		h.Add(tile.Must(tile.SuitBamboo, 2))
		h.Add(tile.Must(tile.SuitBamboo, 3))
		h.Add(tile.Must(tile.SuitCharacters, 4))
		h.Add(tile.Must(tile.SuitCharacters, 5))
		h.Add(tile.Must(tile.SuitCharacters, 6))
		h.Add(tile.Must(tile.SuitCharacters, 7))
		hands[i] = h
	}
	return hands
}
