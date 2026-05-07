package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLastPlayerVisibleEventFiltersInternalNoise 防回归：HUD"最近"行不应显示
// "已入座 N"、"准备成功"、"离房失败：尚未进入房间" 这类协议握手 / 错误日志，
// 否则玩家会以为系统出了问题。
func TestLastPlayerVisibleEventFiltersInternalNoise(t *testing.T) {
	now := time.Now()
	view := RoomView{Log: []LogEntry{
		{At: now.Add(-10 * time.Second), Text: "客户端已启动"},
		{At: now.Add(-9 * time.Second), Text: "已入座 0"},
		{At: now.Add(-8 * time.Second), Text: "准备成功"},
		{At: now.Add(-7 * time.Second), Text: "开局，庄家 0"},
		{At: now.Add(-6 * time.Second), Text: "alice 出 5万"},
		{At: now.Add(-5 * time.Second), Text: "离房失败：尚未进入房间"},
	}}
	require.Equal(t, "alice 出 5万", lastPlayerVisibleEvent(view))
}

func TestVisibleLogEntriesPreservesChronology(t *testing.T) {
	now := time.Now()
	view := RoomView{Log: []LogEntry{
		{At: now.Add(-3 * time.Second), Text: "alice 出 5万"},
		{At: now.Add(-2 * time.Second), Text: "已入座 1"},
		{At: now.Add(-1 * time.Second), Text: "bob 碰 5万"},
	}}
	got := visibleLogEntries(view.Log, 5)
	require.Equal(t, []string{"alice 出 5万", "bob 碰 5万"}, got)
}

func TestBreadcrumbHUDDoesNotIncludeDealerBeforeStart(t *testing.T) {
	view := RoomView{
		RuleID:    "blood",
		RoomID:    "T6S4HS",
		Phase:     phaseTable,
		RoomState: "waiting",
	}
	out := breadcrumbHUD(view, PhaseWaiting)
	require.Contains(t, out, "川麻血战")
	require.Contains(t, out, "T6S4HS")
	require.NotContains(t, out, "庄", "未开局时面包屑不应暴露庄家信息，避免玩家以为已经开局")
}

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

func TestBreadcrumbHUDShowsRuleAndRoomDistinctly(t *testing.T) {
	view := RoomView{
		RuleID:      "blood",
		RoomID:      "VMMEZ6",
		DisplayName: "VMMEZ6",
		Phase:       phaseTable,
		RoomState:   "playing",
	}
	out := breadcrumbHUD(view, PhaseMyTurnIdle)
	require.Equal(t, "川麻血战 ▸ VMMEZ6 ▸ 你的回合", out)
}
