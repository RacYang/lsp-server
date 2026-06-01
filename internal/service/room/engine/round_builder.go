package engine

import (
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/wall"
)

// RoundStateConfig 是测试专用的 RoundState 构造配置；
// 仅供 service/room 包白盒测试使用，禁止在生产代码路径中引用。
type RoundStateConfig struct {
	RoomID          string
	RuleID          string
	Rule            rules.Rule
	Caps            rules.CapabilitySet
	PlayerIDs       [4]string
	Wall            *wall.Wall
	Hands           []*hand.Hand
	RuleState       rules.RuleState
	WaitingDiscard  bool
	WaitingTsumo    bool
	Turn            Seat
	LastDiscardSeat Seat
	Surrendered     []bool
}

// NewRoundStateFromConfig 通过 RoundStateConfig 构造测试用 RoundState。
// 禁止在生产代码路径调用；仅用于需要预置引擎状态的白盒测试。
func NewRoundStateFromConfig(cfg RoundStateConfig) *RoundState {
	rs := &RoundState{
		roomID:          cfg.RoomID,
		ruleID:          cfg.RuleID,
		rule:            cfg.Rule,
		caps:            cfg.Caps,
		playerIDs:       cfg.PlayerIDs,
		wall:            cfg.Wall,
		hands:           cfg.Hands,
		ruleState:       cfg.RuleState,
		waitingDiscard:  cfg.WaitingDiscard,
		waitingTsumo:    cfg.WaitingTsumo,
		turn:            cfg.Turn,
		lastDiscardSeat: cfg.LastDiscardSeat,
		surrendered:     cfg.Surrendered,
	}
	if rs.rule != nil && cfg.Caps.Turn == nil {
		rs.caps = rules.CapabilitiesOf(rs.rule)
	}
	return rs
}
