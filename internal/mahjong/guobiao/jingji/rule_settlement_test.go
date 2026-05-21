package jingji

import (
	"testing"

	"github.com/stretchr/testify/require"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/rules"
)

func TestRuleName(t *testing.T) {
	r := rule{}
	require.Equal(t, "国标麻将（竞技标准）", r.Name())
}

func TestTerminationPolicyFeatureFlags(t *testing.T) {
	p := terminationPolicy{}
	require.Contains(t, p.FeatureFlags(), "first_win_or_wall_empty")
}

func TestTerminationPolicyGameOver(t *testing.T) {
	p := terminationPolicy{}

	tests := []struct {
		desc string
		ctx  rules.TerminationContext
		want bool
	}{
		{
			desc: "有胡牌事件则结束",
			ctx:  rules.TerminationContext{WinEvents: []rules.WinEvent{{Seat: 0}}},
			want: true,
		},
		{
			desc: "墙空则结束",
			ctx:  rules.TerminationContext{WallRemaining: 0},
			want: true,
		},
		{
			desc: "墙未空且无胡牌则不结束",
			ctx:  rules.TerminationContext{WallRemaining: 10},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			require.Equal(t, tc.want, p.GameOver(tc.ctx))
		})
	}
}

func TestSettlementPolicyFeatureFlags(t *testing.T) {
	p := settlementPolicy{}
	flags := p.FeatureFlags()
	require.Contains(t, flags, "seat_scores")
	require.Contains(t, flags, "per_winner_breakdown")
}

func TestSettlementPolicyBuildSettlement(t *testing.T) {
	p := settlementPolicy{}
	playerIDs := [4]string{"u0", "u1", "u2", "u3"}

	t.Run("无胡牌事件荒牌", func(t *testing.T) {
		result := p.BuildSettlement(rules.SettlementContext{PlayerIDs: playerIDs})
		require.Empty(t, result.WinnerUserIDs)
		require.Equal(t, "荒牌", result.DetailText)
	})

	t.Run("单人胡牌结算", func(t *testing.T) {
		ctx := rules.SettlementContext{
			PlayerIDs: playerIDs,
			WinEvents: []rules.WinEvent{
				{Seat: domainroom.SeatFromInt(1)},
			},
			ScoreEvents: []rules.ScoreEvent{
				{WinnerSeat: domainroom.SeatFromInt(1), FromSeat: domainroom.SeatFromInt(2), ToSeat: domainroom.SeatFromInt(1), Amount: 32},
			},
		}
		result := p.BuildSettlement(ctx)
		require.Equal(t, []string{"u1"}, result.WinnerUserIDs)
		require.Equal(t, "国标麻将胡牌", result.DetailText)
		require.Len(t, result.SeatScores, 4)
		require.NotNil(t, result.PerWinnerBreakdown)
	})

	t.Run("同一座位胡牌事件不重复计入", func(t *testing.T) {
		ctx := rules.SettlementContext{
			PlayerIDs: playerIDs,
			WinEvents: []rules.WinEvent{
				{Seat: domainroom.SeatFromInt(0)},
				{Seat: domainroom.SeatFromInt(0)},
			},
		}
		result := p.BuildSettlement(ctx)
		require.Equal(t, []string{"u0"}, result.WinnerUserIDs)
	})
}

func TestWinnerBreakdownsDedupFanNames(t *testing.T) {
	playerIDs := [4]string{"u0", "u1", "u2", "u3"}
	winnerSeats := []domainroom.Seat{domainroom.SeatFromInt(0)}
	scoreEvents := []rules.ScoreEvent{
		{WinnerSeat: domainroom.SeatFromInt(0), WinnerFan: 24, FanNames: []string{"清一色"}, FromSeat: 1, ToSeat: 0, Amount: 24},
		{WinnerSeat: domainroom.SeatFromInt(0), WinnerFan: 24, FanNames: []string{"清一色"}, FromSeat: 2, ToSeat: 0, Amount: 24},
	}
	breakdowns := winnerBreakdowns(playerIDs, scoreEvents, winnerSeats)
	require.Len(t, breakdowns, 1)
	require.Equal(t, int32(24), breakdowns[0].Fan)
	require.Equal(t, []string{"清一色"}, breakdowns[0].FanNames)
}
