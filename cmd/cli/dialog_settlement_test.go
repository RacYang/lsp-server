package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
)

func sampleWinSummary() SettlementSummary {
	return SettlementSummary{
		RoomID:   "R7K2",
		RuleID:   "scmj",
		Outcome:  SettlementOutcomeWin,
		WinnerID: "u1",
		Fans: []SettlementFan{
			{Name: "平 胡", Multiplier: 1},
			{Name: "门 清", Multiplier: 1},
			{Name: "自 摸", Multiplier: 1},
		},
		TotalFan: 3,
		Scores: []SettlementScore{
			{Nickname: "你", Delta: 9, IsSelf: true},
			{Nickname: "alice", Delta: -3},
			{Nickname: "bob", Delta: -3},
			{Nickname: "carl", Delta: -3},
		},
	}
}

func TestSettlementDialogTotalLinesAccountsForAllSections(t *testing.T) {
	d := NewSettlementDialog(sampleWinSummary(), time.Now(), 0)
	// 标题 + 3 番 + 总番分隔 + 4 家分数 + 提示
	require.Equal(t, 1+3+1+4+1, d.totalRevealLines())
}

func TestSettlementDialogVisibleLinesAdvancesByInterval(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	d := NewSettlementDialog(sampleWinSummary(), now, 200*time.Millisecond)
	require.Equal(t, 1, d.VisibleLines(now))
	require.Equal(t, 2, d.VisibleLines(now.Add(200*time.Millisecond)))
	require.Equal(t, 3, d.VisibleLines(now.Add(400*time.Millisecond)))
	require.False(t, d.AllRevealed(now.Add(time.Second)))
	require.True(t, d.AllRevealed(now.Add(10*time.Second)))
}

func TestSettlementDialogVisibleLinesZeroIntervalShowsAll(t *testing.T) {
	d := NewSettlementDialog(sampleWinSummary(), time.Now(), 0)
	require.Equal(t, d.totalRevealLines(), d.VisibleLines(time.Now()))
}

func TestSettlementDialogTitleByOutcome(t *testing.T) {
	d := NewSettlementDialog(SettlementSummary{Outcome: SettlementOutcomeWin}, time.Now(), 0)
	require.Equal(t, "胡 了 !", d.title())
	d.Summary.Outcome = SettlementOutcomeLose
	require.Equal(t, "输 了", d.title())
	d.Summary.Outcome = SettlementOutcomeDraw
	require.Equal(t, "流 局", d.title())
}

func TestSettlementDialogRenderLinesPreservesHeight(t *testing.T) {
	now := time.Now()
	d := NewSettlementDialog(sampleWinSummary(), now, 200*time.Millisecond)
	width := 30
	lines := d.renderLines(now, width)
	require.Len(t, lines, d.totalRevealLines())
	require.Contains(t, lines[0], "胡 了 !")
	require.Equal(t, strings.Repeat(" ", width), lines[1], "第二行还未到揭晓时间应保持空白占位")
}

func TestSettlementDialogIncludesAllScoresWhenRevealed(t *testing.T) {
	now := time.Now()
	d := NewSettlementDialog(sampleWinSummary(), now, 0)
	all := d.allLines(40)
	joined := strings.Join(all, "\n")
	require.Contains(t, joined, "胡 了 !")
	require.Contains(t, joined, "平 胡")
	require.Contains(t, joined, "共 3 番")
	require.Contains(t, joined, "+9")
	require.Contains(t, joined, "-3")
	require.Contains(t, joined, "Enter 继续")
}

func TestDrawSettlementDialogRendersFullBox(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, scr.Init())
	defer scr.Fini()
	scr.SetSize(MinTableWidth, MinTableHeight)
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	now := time.Now()
	d := NewSettlementDialog(sampleWinSummary(), now, 0)
	DrawSettlementDialog(scr, layout, d, now)
	scr.Show()
	out := dumpScreen(scr)
	require.Contains(t, out, "胡 了 !")
	require.Contains(t, out, "+9")
	require.Contains(t, out, "╭")
}

func TestWriteStdoutSummaryWin(t *testing.T) {
	w := &strings.Builder{}
	WriteStdoutSummary(w, sampleWinSummary())
	got := w.String()
	require.Contains(t, got, "本局摘要")
	require.Contains(t, got, "房间: R7K2")
	require.Contains(t, got, "胜者: 你")
	require.Contains(t, got, "+9")
	require.Contains(t, got, "alice: -3")
}

func TestWriteStdoutSummaryDraw(t *testing.T) {
	w := &strings.Builder{}
	WriteStdoutSummary(w, SettlementSummary{Outcome: SettlementOutcomeDraw, Scores: []SettlementScore{{Nickname: "你", IsSelf: true}}})
	require.Contains(t, w.String(), "流局")
}

func TestWriteStdoutSummaryNilWriterNoPanic(t *testing.T) {
	require.NotPanics(t, func() { WriteStdoutSummary(nil, sampleWinSummary()) })
}
