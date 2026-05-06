// Package bot 实现独立的麻将机器人客户端与内置规则策略。
package bot

import (
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// ActionKind 是机器人可提交给服务端的动作类型。
type ActionKind string

const (
	ActionReady         ActionKind = "ready"
	ActionExchangeThree ActionKind = "exchange_three"
	ActionQueMen        ActionKind = "que_men"
	ActionDiscard       ActionKind = "discard"
	ActionPong          ActionKind = "pong"
	ActionGang          ActionKind = "gang"
	ActionHu            ActionKind = "hu"
	ActionPass          ActionKind = "pass"
	ActionNone          ActionKind = "none"
)

// Action 是策略输出的结构化动作。
type Action struct {
	Kind   ActionKind
	Tile   string
	Tiles  []string
	Suit   int32
	Reason string
}

// Strategy 根据机器人可见视图选择下一步动作。
type Strategy interface {
	Decide(ctx Context, view BotView) (Action, error)
}

// Context 是策略可依赖的最小上下文接口，便于测试注入。
type Context interface {
	Done() <-chan struct{}
	Err() error
}

// BotView 是机器人策略可见的牌桌视图。
type BotView struct {
	UserID          string
	RoomID          string
	SeatIndex       int32
	RoomState       string
	WaitingAction   string
	ActingSeat      int32
	PendingTile     string
	AvailableAction []string
	ClaimCandidates map[int32][]string
	QueBySeat       [4]int32
	HandTiles       []string
	DiscardsBySeat  [][]string
	MeldsBySeat     [][]string
	DrawnBySeat     [][]string
	LastSettlement  *clientv1.SettlementNotify
	Closed          bool
	UpdatedAt       time.Time
}

// RunnerConfig 定义单个机器人运行参数。
type RunnerConfig struct {
	Name               string
	RoomID             string
	WSURL              string
	Origin             string
	TokenFile          string
	InsecureSkipVerify bool
	ThinkMin           time.Duration
	ThinkMax           time.Duration
	Strategy           Strategy
}
