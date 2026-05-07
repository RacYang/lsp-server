package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRelativeSeatMapsAllRotations(t *testing.T) {
	// 出牌顺序逆时针 0→1→2→3→0，主流客户端布局：下家在左，上家在右。
	cases := []struct {
		self   int32
		target int32
		want   SeatPosition
	}{
		{1, 1, SeatPosBottom},
		{1, 2, SeatPosLeft}, // 下家 (self+1)
		{1, 3, SeatPosTop},
		{1, 0, SeatPosRight}, // 上家 (self+3)

		{0, 0, SeatPosBottom},
		{0, 1, SeatPosLeft},
		{0, 2, SeatPosTop},
		{0, 3, SeatPosRight},
	}
	for _, tc := range cases {
		got := RelativeSeat(tc.self, tc.target)
		require.Equalf(t, tc.want, got, "self=%d target=%d", tc.self, tc.target)
	}
}

func TestRelativeSeatGuardsAgainstInvalidIndices(t *testing.T) {
	require.Equal(t, SeatPosBottom, RelativeSeat(-1, 0))
	require.Equal(t, SeatPosBottom, RelativeSeat(0, 99))
}

func TestCalcLayoutRejectsTinyTerminal(t *testing.T) {
	_, ok := CalcLayout(40, 12)
	require.False(t, ok)
	_, ok = CalcLayout(MinTableWidth-1, MinTableHeight)
	require.False(t, ok)
	_, ok = CalcLayout(MinTableWidth, 23)
	require.False(t, ok)
}

func TestCalcLayoutMinimum(t *testing.T) {
	l, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	require.Equal(t, MinTableWidth, l.Width)
	require.Equal(t, MinTableHeight, l.Height)
	require.Equal(t, 0, l.TitleBar.Y)
	require.Equal(t, 1, l.TitleBar.Height)
	require.Equal(t, MinTableHeight-1, l.KeyBar.Y)
	require.Equal(t, 1, l.KeyBar.Height)
	require.False(t, l.SouthBand.Empty())
	require.False(t, l.CenterArea.Empty())
	require.Equal(t, 52, l.TableFrame.Width)
	require.Equal(t, 26, l.TableFrame.Height)
	require.Equal(t, l.TableFrame.Height*2, l.TableFrame.Width)
}

func TestCalcLayoutSquareCenteredInvariant(t *testing.T) {
	l, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	require.Equal(t, l.LeftPlayerSlot.Width, l.RightPlayerSlot.Width)
	require.Equal(t, l.TableFrame.Height*2, l.TableFrame.Width)
	centerX := l.TableFrame.X + l.TableFrame.Width/2
	require.InDelta(t, MinTableWidth/2, centerX, 1)
}

func TestCalcLayoutWideTerminalUsesMoreDensity(t *testing.T) {
	l, ok := CalcLayout(125, 41, LayoutTierStandard)
	require.True(t, ok)
	require.Equal(t, LayoutTierWide, l.Tier)
	require.Equal(t, 64, l.TableFrame.Width)
	require.Equal(t, 32, l.TableFrame.Height)
}

func TestResolveLayoutTierHysteresis(t *testing.T) {
	require.Equal(t, LayoutTierStandard, ResolveLayoutTier(120, 36, LayoutTierStandard))
	require.Equal(t, LayoutTierWide, ResolveLayoutTier(125, 41, LayoutTierStandard))
	require.Equal(t, LayoutTierStandard, ResolveLayoutTier(119, 35, LayoutTierWide))
}

func TestCalcLayoutSupportedTiersStaySquare(t *testing.T) {
	for _, size := range []struct{ w, h int }{{100, 30}, {120, 36}, {140, 40}, {200, 60}} {
		l, ok := CalcLayout(size.w, size.h)
		require.True(t, ok)
		require.Equalf(t, l.TableFrame.Height*2, l.TableFrame.Width, "%dx%d", size.w, size.h)
		require.Equalf(t, l.LeftPlayerSlot.Width, l.TableFrame.X, "%dx%d", size.w, size.h)
		require.Equalf(t, l.RightPlayerSlot.X, l.TableFrame.X+l.TableFrame.Width, "%dx%d", size.w, size.h)
	}
}
