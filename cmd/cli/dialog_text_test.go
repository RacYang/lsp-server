package main

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClaimDialogLinesUseTextDecision(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.Local)
	dialog := NewClaimDialog(ClaimTriggerPongOrHu, 1, "阿南", "五筒",
		[]ClaimAction{ClaimActionHu, ClaimActionPong, ClaimActionPass}, now, 5*time.Second)

	text := strings.Join(claimDialogLines(dialog, now.Add(time.Second), 44), "\n")

	require.Contains(t, text, "阿南 打出 五筒")
	require.Contains(t, text, "▶ 胡：就这张牌和牌")
	require.Contains(t, text, "碰：拿下这张牌")
	require.Contains(t, text, "过：放过这次机会")
	require.Contains(t, text, "现在：←→ 选择　Enter 确认")
	require.NotContains(t, text, "[ 胡 ]")
	require.NotContains(t, text, "快捷键")
}

func TestSettlementDialogLinesRevealResultFirst(t *testing.T) {
	dialog := NewSettlementDialog(SettlementSummary{
		Outcome:  SettlementOutcomeWin,
		Fans:     []SettlementFan{{Name: "清一色", Multiplier: 6}, {Name: "门清", Multiplier: 2}},
		TotalFan: 8,
		Winners:  []SettlementWinner{{Nickname: "你", IsSelf: true, Fan: 8, FanNames: []string{"清一色", "门清"}}},
		Scores: []SettlementScore{
			{Nickname: "racoo", Delta: 24, IsSelf: true},
			{Nickname: "阿南", Delta: -8},
		},
	}, time.Now(), 0)

	lines := dialog.allLines(48)

	require.Contains(t, lines[0], "胡了，你赢了")
	require.Contains(t, lines[1], "你本局 +24")
	require.Contains(t, strings.Join(lines, "\n"), "胡家：你  8 番（清一色、门清）")
	require.Contains(t, strings.Join(lines, "\n"), "番种：清一色 +6、门清 +2")
	require.Contains(t, strings.Join(lines, "\n"), "阿南：-8")
}
