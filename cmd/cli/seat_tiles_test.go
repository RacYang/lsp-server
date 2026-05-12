package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlayerJourney_Q2_2_QueSuitTileDecodingAndCursorOverlay 锁定 [Q2.2] 缺门灰显规则。
//
// 玩家旅程 [Q2.2] 要求自家手牌区按 view.QueBySeat[seat] 把缺门花色灰显，
// 帮玩家优先打缺门；光标 / Marked 高亮必须覆盖灰显，避免「正在操作的牌被
// 灰吃掉」。本用例对 isQueSuitTile + cursorHighlightedAt 两个判定函数做
// 单元级断言：m/p/s 三花色与 que=0/1/2/-1 的所有组合 + 光标/标记位的避让。
func TestPlayerJourney_Q2_2_QueSuitTileDecodingAndCursorOverlay(t *testing.T) {
	cases := []struct {
		name string
		tile string
		que  int32
		want bool
	}{
		{"m_match_wan", "m3", 0, true},
		{"p_match_tong", "p7", 1, true},
		{"s_match_tiao", "s9", 2, true},
		{"m_not_match_tong", "m3", 1, false},
		{"p_not_match_tiao", "p7", 2, false},
		{"s_not_match_wan", "s9", 0, false},
		{"que_unset", "m1", -1, false},
		{"que_out_of_range", "m1", 5, false},
		{"non_suit_tile", "z3", 0, false},
		{"empty_tile", "", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isQueSuitTile(tc.tile, tc.que))
		})
	}

	cursor := &HandCursor{Mode: CursorModeSingle, Index: 2, Marked: []int{0}}
	require.True(t, cursorHighlightedAt(cursor, 0), "Marked 索引必须被识别为高亮位")
	require.True(t, cursorHighlightedAt(cursor, 2), "Index 必须被识别为高亮位")
	require.False(t, cursorHighlightedAt(cursor, 1), "未高亮位返回 false")
	require.False(t, cursorHighlightedAt(nil, 0), "nil cursor 一律返回 false")
}
