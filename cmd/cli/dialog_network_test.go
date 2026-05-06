package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
)

func TestNetOverlayDefaultsToOnline(t *testing.T) {
	n := NewNetOverlay(0)
	require.Equal(t, NetStatusOnline, n.Status)
	require.False(t, n.Visible())
	require.Equal(t, DefaultNetTimeout, n.Threshold)
}

func TestNetOverlayEnterReconnectingOnDisconnect(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	n := NewNetOverlay(5 * time.Second)
	n.Update(false, now)
	require.Equal(t, NetStatusReconnecting, n.Status)
	require.Equal(t, now, n.Since)
	require.True(t, n.Visible())
}

func TestNetOverlayPromotesToOfflineAfterThreshold(t *testing.T) {
	start := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	n := NewNetOverlay(2 * time.Second)
	n.Update(false, start)
	n.Update(false, start.Add(time.Second))
	require.Equal(t, NetStatusReconnecting, n.Status)

	n.Update(false, start.Add(3*time.Second))
	require.Equal(t, NetStatusOffline, n.Status)
	require.True(t, n.Visible())
}

func TestNetOverlayResetsToOnlineOnReconnect(t *testing.T) {
	start := time.Now()
	n := NewNetOverlay(2 * time.Second)
	n.Update(false, start)
	n.Update(false, start.Add(3*time.Second))
	require.Equal(t, NetStatusOffline, n.Status)
	n.Update(true, start.Add(4*time.Second))
	require.Equal(t, NetStatusOnline, n.Status)
	require.True(t, n.Since.IsZero())
}

func TestNetOverlayDetailShowsElapsed(t *testing.T) {
	start := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	n := NewNetOverlay(time.Minute)
	n.Update(false, start)
	require.Contains(t, n.detail(start.Add(2500*time.Millisecond)), "2.5s")
}

func TestDrawNetOverlayShowsBox(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, scr.Init())
	defer scr.Fini()
	scr.SetSize(MinTableWidth, MinTableHeight)
	layout, _ := CalcLayout(MinTableWidth, MinTableHeight)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	n := NewNetOverlay(2 * time.Second)
	n.Update(false, now)
	DrawNetOverlay(scr, layout, n, now.Add(time.Second))
	scr.Show()
	out := dumpScreen(scr)
	require.Contains(t, out, "网 络 异 常")
	require.Contains(t, out, "1.0s")
}

func TestDrawNetOverlayOfflineShowsButton(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, scr.Init())
	defer scr.Fini()
	scr.SetSize(MinTableWidth, MinTableHeight)
	layout, _ := CalcLayout(MinTableWidth, MinTableHeight)
	now := time.Now()
	n := NewNetOverlay(time.Second)
	n.Update(false, now)
	n.Update(false, now.Add(2*time.Second))
	DrawNetOverlay(scr, layout, n, now.Add(2*time.Second))
	scr.Show()
	out := dumpScreen(scr)
	require.Contains(t, out, "无 法 恢 复")
	require.Contains(t, out, "返回大厅")
}

func TestDrawNetOverlayOnlineNoOp(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, scr.Init())
	defer scr.Fini()
	scr.SetSize(MinTableWidth, MinTableHeight)
	layout, _ := CalcLayout(MinTableWidth, MinTableHeight)
	n := NewNetOverlay(0)
	DrawNetOverlay(scr, layout, n, time.Now())
	scr.Show()
	require.Empty(t, strings.TrimSpace(dumpScreen(scr)))
}
