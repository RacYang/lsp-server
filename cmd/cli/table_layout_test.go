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
}

func TestCalcLayoutMinimum(t *testing.T) {
	l, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	require.Equal(t, MinTableWidth, l.Width)
	require.Equal(t, MinTableHeight, l.Height)
	require.Equal(t, 0, l.TopArea.Y)
	require.Equal(t, 4, l.TopArea.Height)
	require.Equal(t, MinTableHeight-1, l.HintArea.Y)
	require.Equal(t, 1, l.HintArea.Height)
	require.False(t, l.HandArea.Empty())
	require.False(t, l.CenterArea.Empty())
	require.Greater(t, l.RightArea.X, l.LeftArea.X)
}

func TestCalcLayoutSelfBlocksOrderedAboveHand(t *testing.T) {
	// 自家"打 / 鸣 / protrusion / hand"四块在垂直方向必须按距离手牌从远到近排列,
	// 且互不重叠;protrusion 行紧贴 HandArea 上方,鸣行紧贴 protrusion 上方,
	// 牌河行再紧贴鸣行上方。这样光标凸起不会盖住自家鸣牌,鸣牌又不会盖住牌河。
	l, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	require.Equal(t, l.HandArea.Y-1, l.SelfMeldsArea.Y+l.SelfMeldsArea.Height,
		"protrusion 缓冲必须夹在 SelfMeldsArea 与 HandArea 之间")
	require.Equal(t, l.SelfMeldsArea.Y, l.SelfDiscardsArea.Y+l.SelfDiscardsArea.Height,
		"SelfMeldsArea 必须紧贴 SelfDiscardsArea 下方,无空隙也无重叠")
	require.GreaterOrEqual(t, l.SelfDiscardsArea.Y, l.CenterArea.Y+l.CenterArea.Height,
		"SelfDiscardsArea 不应回侵 CenterArea")
}

func TestCalcLayoutLargeTerminalGrowsCenterArea(t *testing.T) {
	smallL, _ := CalcLayout(MinTableWidth, MinTableHeight)
	bigL, ok := CalcLayout(MinTableWidth, MinTableHeight+10)
	require.True(t, ok)
	require.Greater(t, bigL.CenterArea.Height, smallL.CenterArea.Height)
}
