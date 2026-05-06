package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
)

func TestNewClaimDialogDefaults(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	d := NewClaimDialog(ClaimTriggerSelfDraw, 1, "racoo", "p5",
		[]ClaimAction{ClaimActionHu, ClaimActionPass}, now, 5*time.Second)
	require.Equal(t, now, d.OpenedAt)
	require.Equal(t, now.Add(5*time.Second), d.Deadline)
	require.Equal(t, 0, d.SelectedIndex)
	require.False(t, d.Pending)
	require.InDelta(t, 1.0, d.Progress(now), 1e-9)
	require.InDelta(t, 0.0, d.Progress(d.Deadline), 1e-9)
}

func TestClaimDialogMoveWraps(t *testing.T) {
	d := &ClaimDialogState{Actions: []ClaimAction{ClaimActionHu, ClaimActionPong, ClaimActionPass}}
	d.Move(1)
	require.Equal(t, 1, d.SelectedIndex)
	d.Move(1)
	require.Equal(t, 2, d.SelectedIndex)
	d.Move(1)
	require.Equal(t, 0, d.SelectedIndex, "右移到末尾后回到第一个")
	d.Move(-1)
	require.Equal(t, 2, d.SelectedIndex)
}

func TestClaimDialogMoveBlockedDuringPending(t *testing.T) {
	d := &ClaimDialogState{Actions: []ClaimAction{ClaimActionHu, ClaimActionPass}, Pending: true}
	d.Move(1)
	require.Equal(t, 0, d.SelectedIndex)
}

func TestClaimDialogProgressMidway(t *testing.T) {
	now := time.Now()
	d := NewClaimDialog(ClaimTriggerRon, 0, "alice", "m4",
		[]ClaimAction{ClaimActionHu, ClaimActionPass}, now, 5*time.Second)
	mid := now.Add(2500 * time.Millisecond)
	require.InDelta(t, 0.5, d.Progress(mid), 0.001)
	require.False(t, d.Expired(mid))
	require.True(t, d.Expired(d.Deadline.Add(time.Millisecond)))
}

func TestClaimActionLabelSelfDrawSpecialPass(t *testing.T) {
	require.Equal(t, "不胡", claimActionLabel(ClaimActionPass, ClaimTriggerSelfDraw))
	require.Equal(t, "过", claimActionLabel(ClaimActionPass, ClaimTriggerRon))
}

func TestClaimDialogTitleTemplates(t *testing.T) {
	now := time.Now()
	cases := []struct {
		trigger ClaimTrigger
		want    string
	}{
		{ClaimTriggerSelfDraw, "你 自 摸 了 !"},
		{ClaimTriggerRon, "胡 alice 打出的 m4"},
		{ClaimTriggerPong, "碰 alice 打出的 m4"},
		{ClaimTriggerGang, "杠 alice 打出的 m4"},
		{ClaimTriggerChow, "吃 alice 打出的 m4"},
		{ClaimTriggerPongOrHu, "胡/碰 alice 打出的 m4"},
	}
	for _, tc := range cases {
		d := NewClaimDialog(tc.trigger, 0, "alice", "m4", nil, now, time.Second)
		require.Equal(t, tc.want, d.title())
	}
}

func TestClaimDialogTitleFallbackBySeat(t *testing.T) {
	d := NewClaimDialog(ClaimTriggerRon, 2, "", "p5", nil, time.Now(), time.Second)
	require.Equal(t, "胡 3 号位 打出的 p5", d.title())
}

func TestClaimDialogLinesContainTitleButtonsAndBar(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	d := NewClaimDialog(ClaimTriggerRon, 0, "alice", "m4",
		[]ClaimAction{ClaimActionHu, ClaimActionPass}, now, 4*time.Second)
	d.SelectedIndex = 0
	lines := claimDialogLines(d, now.Add(time.Second), 36)
	require.Len(t, lines, 5)
	require.Contains(t, lines[0], "胡 alice 打出的 m4")
	require.Contains(t, lines[2], "[ 胡 ]")
	require.Contains(t, lines[2], "过")
	bar := lines[4]
	require.Contains(t, bar, "█")
	require.Contains(t, bar, "░")
	require.Contains(t, bar, "3.0s")
}

func TestClaimDialogLinesEmphasizesSelected(t *testing.T) {
	d := &ClaimDialogState{
		Trigger:       ClaimTriggerRon,
		Actions:       []ClaimAction{ClaimActionHu, ClaimActionPass},
		SelectedIndex: 1,
		OpenedAt:      time.Now(),
		Deadline:      time.Now().Add(time.Second),
	}
	lines := claimDialogLines(d, time.Now(), 30)
	require.Contains(t, lines[2], "[ 过 ]")
	require.NotContains(t, lines[2], "[ 胡 ]")
}

func TestDrawClaimDialogRendersFullBox(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, scr.Init())
	defer scr.Fini()
	scr.SetSize(MinTableWidth, MinTableHeight)

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	d := NewClaimDialog(ClaimTriggerSelfDraw, 1, "racoo", "p5",
		[]ClaimAction{ClaimActionHu, ClaimActionPass}, now, 5*time.Second)
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	DrawClaimDialog(scr, layout, d, now.Add(2*time.Second))
	scr.Show()

	out := dumpScreen(scr)
	require.Contains(t, out, "你 自 摸 了 !")
	require.Contains(t, out, "[ 胡 ]")
	require.Contains(t, out, "不胡")
	require.True(t, strings.Contains(out, "┌") && strings.Contains(out, "└"), "需要绘制完整边框: %q", out)
	require.Contains(t, out, "3.0s")
}
