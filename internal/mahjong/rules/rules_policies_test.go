package rules

import (
	"testing"

	"github.com/stretchr/testify/require"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// RuleState.Normalize 测试

func TestRuleStateNormalizeIsNoop(t *testing.T) {
	s := RuleState{SchemaVersion: 2}
	s.Normalize(1)
	require.Equal(t, 2, s.SchemaVersion)
}

// 空规则状态策略测试

func TestEmptyRuleStatePolicyAllMethods(t *testing.T) {
	p := EmptyRuleStatePolicy{}
	initial := p.InitialRuleState()
	require.Empty(t, initial.Data)
	normalized := p.NormalizeRuleState(initial)
	require.Equal(t, initial, normalized)
	proj := p.ProjectRuleState(initial)
	require.Empty(t, proj.SeatInts)
}

// 标准自行动作策略测试

func TestStandardSelfActionPolicyCanAnGang(t *testing.T) {
	p := StandardSelfActionPolicy{}
	t1 := tile.Must(tile.SuitCharacters, 1)

	tests := []struct {
		desc string
		ctx  SelfActionContext
		want bool
	}{
		{
			desc: "手牌为 nil 返回 false",
			ctx:  SelfActionContext{Tile: t1},
			want: false,
		},
		{
			desc: "牌面为 0 返回 false",
			ctx:  SelfActionContext{Hand: hand.FromTiles([]tile.Tile{t1, t1, t1, t1})},
			want: false,
		},
		{
			desc: "手牌有 4 张相同牌可暗杠",
			ctx:  SelfActionContext{Hand: hand.FromTiles([]tile.Tile{t1, t1, t1, t1}), Tile: t1},
			want: true,
		},
		{
			desc: "手牌只有 3 张相同牌不可暗杠",
			ctx:  SelfActionContext{Hand: hand.FromTiles([]tile.Tile{t1, t1, t1}), Tile: t1},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			require.Equal(t, tc.want, p.CanAnGang(tc.ctx))
		})
	}
}

func TestStandardSelfActionPolicyCanBuGang(t *testing.T) {
	p := StandardSelfActionPolicy{}
	t1 := tile.Must(tile.SuitCharacters, 1)
	t2 := tile.Must(tile.SuitCharacters, 2)

	tests := []struct {
		desc string
		ctx  SelfActionContext
		want bool
	}{
		{
			desc: "手牌为 nil 返回 false",
			ctx:  SelfActionContext{Tile: t1},
			want: false,
		},
		{
			desc: "牌面为 0 返回 false",
			ctx:  SelfActionContext{Hand: hand.FromTiles([]tile.Tile{t1})},
			want: false,
		},
		{
			desc: "有对应碰牌面子可补杠",
			ctx: SelfActionContext{
				Hand: hand.FromTiles([]tile.Tile{t1}),
				Tile: t1,
				Melds: []MeldContext{
					{Kind: "pong", Tiles: []tile.Tile{t1, t1, t1}},
				},
			},
			want: true,
		},
		{
			desc: "手牌无该牌但有碰面子也不可补杠",
			ctx: SelfActionContext{
				Hand: hand.FromTiles([]tile.Tile{t2}),
				Tile: t1,
				Melds: []MeldContext{
					{Kind: "pong", Tiles: []tile.Tile{t1, t1, t1}},
				},
			},
			want: false,
		},
		{
			desc: "无碰面子不可补杠",
			ctx: SelfActionContext{
				Hand: hand.FromTiles([]tile.Tile{t1}),
				Tile: t1,
			},
			want: false,
		},
		{
			desc: "面子类型不是 pong 不可补杠",
			ctx: SelfActionContext{
				Hand: hand.FromTiles([]tile.Tile{t1}),
				Tile: t1,
				Melds: []MeldContext{
					{Kind: "chi", Tiles: []tile.Tile{t1, t2, tile.Must(tile.SuitCharacters, 3)}},
				},
			},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			require.Equal(t, tc.want, p.CanBuGang(tc.ctx))
		})
	}
}

// 固定开局流程策略测试

func TestStaticOpeningFlowInitialState(t *testing.T) {
	flow := StaticOpeningFlow{"step_a", "step_b"}
	state := flow.InitialState()
	require.NotEmpty(t, state.Data)
}

func TestStaticOpeningFlowCurrentStep(t *testing.T) {
	flow := StaticOpeningFlow{"step_a"}
	state := flow.InitialState()

	step, ok := flow.CurrentStep(OpeningContext{RuleState: state})
	require.True(t, ok)
	require.Equal(t, "step_a", step.ID)
}

func TestStaticOpeningFlowApplyCompletes(t *testing.T) {
	flow := StaticOpeningFlow{"step_a"}
	state := flow.InitialState()

	for seat := 0; seat < 4; seat++ {
		ctx := OpeningActionContext{
			OpeningContext: OpeningContext{RuleState: state},
			Seat:           domainroom.SeatFromInt(seat),
		}
		result, err := flow.Apply(ctx)
		require.NoError(t, err)
		state = result.RuleState
	}

	_, ok := flow.CurrentStep(OpeningContext{RuleState: state})
	require.False(t, ok)
}

func TestStaticOpeningFlowApplyWhenAlreadyComplete(t *testing.T) {
	flow := StaticOpeningFlow{}
	state := flow.InitialState()
	ctx := OpeningActionContext{OpeningContext: OpeningContext{RuleState: state}}
	result, err := flow.Apply(ctx)
	require.NoError(t, err)
	require.True(t, result.AllOpeningComplete)
}

// 特性开关集合测试

func TestFeatureSetHuedSeatContinues(t *testing.T) {
	f := FeatureSet{"some_feature"}
	require.False(t, f.HuedSeatContinues())
}

func TestFeatureSetBuildSettlementNoWinners(t *testing.T) {
	f := FeatureSet{}
	result := f.BuildSettlement(SettlementContext{})
	require.Equal(t, "荒牌", result.DetailText)
	require.Empty(t, result.WinnerUserIDs)
}

func TestFeatureSetBuildSettlementWithWinner(t *testing.T) {
	f := FeatureSet{}
	playerIDs := [4]string{"u0", "u1", "u2", "u3"}
	ctx := SettlementContext{
		PlayerIDs: playerIDs,
		WinEvents: []WinEvent{{Seat: domainroom.SeatFromInt(1)}},
		ScoreEvents: []ScoreEvent{
			{FromSeat: domainroom.SeatFromInt(2), ToSeat: domainroom.SeatFromInt(1), Amount: 16},
		},
	}
	result := f.BuildSettlement(ctx)
	require.Equal(t, []string{"u1"}, result.WinnerUserIDs)
	require.Equal(t, "本局结束", result.DetailText)
}

// 标准抢答策略测试

func TestStandardClaimPolicyCandidates(t *testing.T) {
	p := StandardClaimPolicy{}
	t1 := tile.Must(tile.SuitCharacters, 1)
	t2 := tile.Must(tile.SuitCharacters, 2)
	t3 := tile.Must(tile.SuitCharacters, 3)

	tests := []struct {
		desc        string
		ctx         ClaimContext
		wantActions []string
	}{
		{
			desc: "Hued 返回 nil",
			ctx: ClaimContext{
				Hued: true,
				Seat: domainroom.SeatFromInt(1),
				Tile: t1,
				Hand: hand.FromTiles([]tile.Tile{t1, t1, t1}),
			},
			wantActions: nil,
		},
		{
			desc: "Hand 为 nil 返回 nil",
			ctx: ClaimContext{
				Seat:       domainroom.SeatFromInt(1),
				SourceSeat: domainroom.SeatFromInt(0),
				Tile:       t1,
			},
			wantActions: nil,
		},
		{
			desc: "Tile 为 0 返回 nil",
			ctx: ClaimContext{
				Seat:       domainroom.SeatFromInt(1),
				SourceSeat: domainroom.SeatFromInt(0),
				Hand:       hand.FromTiles([]tile.Tile{t1}),
			},
			wantActions: nil,
		},
		{
			desc: "同座位返回 nil",
			ctx: ClaimContext{
				Seat:       domainroom.SeatFromInt(1),
				SourceSeat: domainroom.SeatFromInt(1),
				Tile:       t1,
				Hand:       hand.FromTiles([]tile.Tile{t1, t1, t1}),
			},
			wantActions: nil,
		},
		{
			desc: "有 3 张相同可碰可杠",
			ctx: ClaimContext{
				Seat:       domainroom.SeatFromInt(2),
				SourceSeat: domainroom.SeatFromInt(0),
				Tile:       t1,
				Hand:       hand.FromTiles([]tile.Tile{t1, t1, t1}),
			},
			wantActions: []string{"gang", "pong"},
		},
		{
			desc: "下家可吃",
			ctx: ClaimContext{
				Seat:       domainroom.SeatFromInt(1),
				SourceSeat: domainroom.SeatFromInt(0),
				Tile:       t1,
				Hand:       hand.FromTiles([]tile.Tile{t2, t3}),
			},
			wantActions: []string{"chi"},
		},
		{
			desc: "抢杠窗口只能胡不能碰",
			ctx: ClaimContext{
				Seat:            domainroom.SeatFromInt(2),
				SourceSeat:      domainroom.SeatFromInt(0),
				Tile:            t1,
				Hand:            hand.FromTiles([]tile.Tile{t1, t1, t1}),
				QiangGangWindow: true,
				CheckHu: func(h *hand.Hand, target tile.Tile, hc HuContext) (HuResult, bool) {
					return HuResult{}, false
				},
			},
			wantActions: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			actions := p.Candidates(tc.ctx)
			if tc.wantActions == nil {
				require.Empty(t, actions)
				return
			}
			names := make([]string, len(actions))
			for i, a := range actions {
				names[i] = string(a.Name)
			}
			require.Equal(t, tc.wantActions, names)
		})
	}
}
