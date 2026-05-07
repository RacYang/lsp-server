package main

import "strings"

// ruleDisplayNames 把协议侧的 ruleID 映射到玩家可读的中文名。
//
// 与 internal/rules 注册保持一致；新增规则时同步在这里登记，避免 HUD 出现
// "default" / "blood" 这类英文 ID。
var ruleDisplayNames = map[string]string{
	"default":       "默认麻将",
	"blood":         "川麻血战",
	"sichuan":       "川麻血战",
	"sichuan_xz":    "川麻血战",
	"sichuan_xzdd":  "川麻血战 (血战到底)",
	"sichuan_dx":    "川麻倒下",
	"changsha":      "长沙麻将",
	"japanese":      "日麻立直",
	"international": "国标麻将",
}

// ruleLabel 仅根据 RuleID 得到玩家可读的规则名；不会再误用 DisplayName。
//
// 注意：协议层的 DisplayName 是「房间标题」（例如玩家自定义的桌名 / RoomID 兜底），
// 与"规则"是两个维度。早期实现把 DisplayName 当成 ruleLabel 的优先来源，
// 导致面包屑出现 "VMMEZ6 ▸ VMMEZ6 ▸ ..."（房间名顶替了规则名）。
func ruleLabel(view RoomView) string {
	if name, ok := ruleDisplayNames[view.RuleID]; ok {
		return name
	}
	if view.RuleID != "" {
		return view.RuleID
	}
	return "麻将"
}

// roomLabel 是面包屑里"房间"那一格的展示文案：DisplayName 优先（玩家可读名），
// 没有则回退到 RoomID；都没有时显示 "--"。
func roomLabel(view RoomView) string {
	if view.DisplayName != "" {
		return view.DisplayName
	}
	if view.RoomID != "" {
		return view.RoomID
	}
	return "--"
}

// breadcrumbHUD 拼装顶部"规则 ▸ 房间 ▸ 阶段"面包屑。
//
// 阶段固定靠 phase 派生，不再混入庄家信息——庄家只在游戏真正进入打牌阶段
// 才由 statusbar 追加 " ▸ 庄 X" 段落。
func breadcrumbHUD(view RoomView, phase TablePhase) string {
	return strings.Join([]string{ruleLabel(view), roomLabel(view), phaseLabel(phase)}, " ▸ ")
}

// gameStarted 判定本局是否已经发牌；HUD 用它来决定是否暴露庄家、剩余牌等局况。
func gameStarted(view RoomView) bool {
	switch view.RoomState {
	case "playing", "settling":
		return true
	}
	return view.LastSettlement != nil
}

// 历史得分 sparkline 已在 statusbar 移除：协议层尚未提供按局得分序列，先撤掉
// 占位实现，后续接入真实数据（ADR-0038 规划中的 RoundEnd notify 累积）后再加回。
