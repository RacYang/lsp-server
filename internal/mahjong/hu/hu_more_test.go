package hu

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsWinningWithOpenMelds(t *testing.T) {
	tests := []struct {
		desc      string
		counts    func() Counts
		openMelds int
		want      bool
	}{
		{
			desc:      "openMelds 负数返回 false",
			counts:    func() Counts { return Counts{} },
			openMelds: -1,
			want:      false,
		},
		{
			desc:      "openMelds 超过 4 返回 false",
			counts:    func() Counts { return Counts{} },
			openMelds: 5,
			want:      false,
		},
		{
			desc: "无副露等同 IsWinning",
			counts: func() Counts {
				// 四组刻子加一对将
				var c Counts
				c[0] = 3
				c[1] = 3
				c[2] = 3
				c[3] = 3
				c[4] = 2
				return c
			},
			openMelds: 0,
			want:      true,
		},
		{
			desc: "张数不符合 14-openMelds*3 返回 false",
			counts: func() Counts {
				var c Counts
				c[0] = 2
				return c
			},
			openMelds: 1,
			want:      false,
		},
		{
			desc: "单张超过 4 返回 false",
			counts: func() Counts {
				var c Counts
				c[0] = 5
				return c
			},
			openMelds: 3,
			want:      false,
		},
		{
			desc: "三副露只剩 5 张：两刻+将",
			counts: func() Counts {
				// 三副露只剩五张牌
				var c Counts
				c[0] = 3
				c[1] = 2
				return c
			},
			openMelds: 3,
			want:      true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			require.Equal(t, tc.want, IsWinningWithOpenMelds(tc.counts(), tc.openMelds))
		})
	}
}

func TestStandardFormWithOpenMelds(t *testing.T) {
	tests := []struct {
		desc      string
		counts    func() Counts
		openMelds int
		want      bool
	}{
		{
			desc:      "openMelds 负数返回 false",
			counts:    func() Counts { return Counts{} },
			openMelds: -1,
			want:      false,
		},
		{
			desc:      "openMelds 超过 4 返回 false",
			counts:    func() Counts { return Counts{} },
			openMelds: 5,
			want:      false,
		},
		{
			desc: "张数不符合 14-openMelds*3 返回 false",
			counts: func() Counts {
				var c Counts
				c[0] = 3
				return c
			},
			openMelds: 2,
			want:      false,
		},
		{
			desc: "两副露暗手 8 张：顺子+刻子+将",
			counts: func() Counts {
				// 两副露暗手八张
				var c Counts
				c[0] = 1
				c[1] = 1
				c[2] = 1
				c[3] = 3
				c[4] = 2
				return c
			},
			openMelds: 2,
			want:      true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			require.Equal(t, tc.want, StandardFormWithOpenMelds(tc.counts(), tc.openMelds))
		})
	}
}

func TestSevenPairsRejectOddCount(t *testing.T) {
	var c Counts
	c[0] = 3
	c[1] = 11
	if SevenPairs(c) {
		t.Fatal("expected false")
	}
}

func TestStandardFormRequiresFourteen(t *testing.T) {
	var c Counts
	c[0] = 2
	if StandardForm(c) {
		t.Fatal("expected false when总张数不是 14")
	}
}

func TestIsWinningRejectsNonFourteen(t *testing.T) {
	var c Counts
	c[0] = 2
	if IsWinning(c) {
		t.Fatal("expected false")
	}
}

func TestIsWinningRejectsMoreThanFour(t *testing.T) {
	var c Counts
	c[0] = 14
	if IsWinning(c) {
		t.Fatal("expected false when单张计数超过 4")
	}
}
