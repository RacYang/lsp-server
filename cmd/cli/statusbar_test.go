package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleLabelTranslatesKnownIDsAndFallsBackGracefully(t *testing.T) {
	require.Equal(t, "川麻血战", ruleLabel(RoomView{RuleID: "blood"}))
	require.Equal(t, "国标麻将", ruleLabel(RoomView{RuleID: "international"}))
	// 防回归：DisplayName 是房间名，绝不能顶替规则名（旧实现 bug 会让 HUD 显示 "VMMEZ6 ▸ VMMEZ6 ▸ ..."）
	require.Equal(t, "川麻血战", ruleLabel(RoomView{DisplayName: "我的房间", RuleID: "blood"}))
	require.Equal(t, "麻将", ruleLabel(RoomView{}))
	require.Equal(t, "future_rule", ruleLabel(RoomView{RuleID: "future_rule"}))
}

func TestRoomLabelPrefersDisplayNameThenRoomID(t *testing.T) {
	require.Equal(t, "我的房间", roomLabel(RoomView{DisplayName: "我的房间", RoomID: "VMMEZ6"}))
	require.Equal(t, "VMMEZ6", roomLabel(RoomView{RoomID: "VMMEZ6"}))
	require.Equal(t, "--", roomLabel(RoomView{}))
}

func TestFocusSummaryUsesCurrentStageInsteadOfAlwaysDiscard(t *testing.T) {
	tests := []struct {
		name          string
		waitingAction string
		want          string
		notWant       string
	}{
		{
			name:          "exchange",
			waitingAction: "exchange_three",
			want:          "换三张",
			notWant:       "出牌中",
		},
		{
			name:          "que_men",
			waitingAction: "que_men",
			want:          "定缺",
			notWant:       "出牌中",
		},
		{
			name:          "discard",
			waitingAction: "discard",
			want:          "出牌中",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := RoomView{
				Phase:         phaseTable,
				RoomState:     "playing",
				SeatIndex:     2,
				ActingSeat:    0,
				WaitingAction: tt.waitingAction,
			}
			view.Players[0].Nickname = "BOT-0"

			got := focusSummary(view, nowForTest())

			require.Contains(t, got, tt.want)
			if tt.notWant != "" {
				require.NotContains(t, got, tt.notWant)
			}
		})
	}
}
