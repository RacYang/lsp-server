// Package rules 定义可插拔麻将规则接口与注册表；具体变体放在子目录中实现。
package rules

import (
	"context"
	"fmt"
	"sort"
	"sync"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

// HuSource 表示和牌来源，供规则实现区分自摸、点炮与抢杠等场况。
type HuSource string

const (
	HuSourceUnspecified HuSource = ""
	HuSourceTsumo       HuSource = "tsumo"
	HuSourceDiscard     HuSource = "discard"
	HuSourceQiangGang   HuSource = "qiang_gang"
	HuSourceBuGang      HuSource = "bu_gang"
)

// GangKind 表示杠牌类型，供结算阶段计算刮风下雨与退税。
type GangKind string

const (
	GangKindUnspecified GangKind = ""
	GangKindMing        GangKind = "ming"
	GangKindAn          GangKind = "an"
	GangKindBu          GangKind = "bu"
)

// GangRecord 记录一笔杠牌流水，后续用于抢杠、退税与责任方判定。
type GangRecord struct {
	Seat            domainroom.Seat
	Kind            GangKind
	Tile            tile.Tile
	FromSeat        domainroom.Seat
	ResponsibleSeat domainroom.Seat
	Step            int
}

// HuContext 为和牌判定上下文；规则实现可按需消费场况字段。
type HuContext struct {
	Seat            domainroom.Seat
	Source          HuSource
	PendingTile     tile.Tile
	Que             []tile.Suit
	HasQueSuit      bool
	QueSuit         tile.Suit
	Discarder       domainroom.Seat
	IsHaiDi         bool
	IsGangShangHua  bool
	ResponsibleSeat domainroom.Seat
	GangHistory     []GangRecord
	Melds           []MeldContext
	WallRemaining   int
}

// HuResult 保存和牌后的 14 张计数快照，供计分使用。
type HuResult struct {
	Win       hu.Counts
	Closed    hu.Counts
	OpenMelds int
	Melds     []MeldContext
}

// ScoreContext 为计分上下文；Phase 5 规则 PR 会逐步消费这些字段。
type ScoreContext struct {
	HuSeat               domainroom.Seat
	DealerSeat           domainroom.Seat
	SeatGenTiles         [][]tile.Tile
	GangRecords          []GangRecord
	Melds                []MeldContext
	IsTsumo              bool
	IsOpeningDraw        bool
	IsDealerFirstDiscard bool
	IsHaiDi              bool
	IsGangShangHua       bool
	IsGangShangPao       bool
	Que                  []tile.Suit
	ResponsibleSeat      domainroom.Seat
	WallRemaining        int
}

// MeldContext 是规则层计算胡牌与番种所需的副露事实。
type MeldContext struct {
	Kind      string
	Tiles     []tile.Tile
	Concealed bool
}

// GameState 描述血战到底结束条件所需的最小信息。
type GameState struct {
	// WallRemaining 牌墙剩余张数（摸牌堆）。
	WallRemaining int
	// HuedPlayers 已经和牌的人数。
	HuedPlayers int
}

// ActionName 是局内动作的稳定内部名称；对外协议仍以字符串追加兼容。
type ActionName string

const (
	ActionHu   ActionName = "hu"
	ActionGang ActionName = "gang"
	ActionPong ActionName = "pong"
	ActionChi  ActionName = "chi"
)

// RuleMetadata 描述客户端与大厅可读的规则能力摘要。
type RuleMetadata struct {
	DisplayName     string
	ShortDesc       string
	EnabledFeatures []string
	MaxHands        int32
}

// CapabilitySet 是规则门面暴露给 room engine 的能力组合。
type CapabilitySet struct {
	Metadata    RuleMetadata
	Opening     OpeningFlow
	Claims      ClaimPolicy
	Turn        TurnFlow
	Scoring     ScoringPolicy
	Settlement  SettlementPolicy
	Termination TerminationPolicy
	Projection  RoundProjection
}

// OpeningFlow 描述规则开局子流程，如换三张、定缺与选庄。
type OpeningFlow interface {
	Steps() []string
}

// TurnFlow 描述摸牌、自摸窗口、杠后补牌与海底处理等轮转能力。
type TurnFlow interface {
	FeatureFlags() []string
}

// ScoringPolicy 是规则计分能力的标记接口；具体计分仍由 Rule.ScoreFans 承担。
type ScoringPolicy interface {
	FeatureFlags() []string
}

// SettlementPolicy 是规则结算能力的标记接口；房间层不得依赖具体规则包类型。
type SettlementPolicy interface {
	FeatureFlags() []string
}

// TerminationPolicy 是终局能力的标记接口；具体终局仍由 Rule.GameOver 承担。
type TerminationPolicy interface {
	FeatureFlags() []string
}

// RoundProjection 是规则投影能力的标记接口，供快照与 TUI 契约演进时接入。
type RoundProjection interface {
	FeatureFlags() []string
}

// ClaimAction 描述一个抢答动作颗粒及其优先级。
type ClaimAction struct {
	Name         ActionName
	ChoiceAction string
	Priority     int
}

// ClaimContext 是规则判断吃碰杠胡候选时可见的确定性上下文。
type ClaimContext struct {
	Seat            domainroom.Seat
	SourceSeat      domainroom.Seat
	Tile            tile.Tile
	Hand            *hand.Hand
	QiangGangWindow bool
	Hued            bool
	HuContext       HuContext
	CheckHu         func(*hand.Hand, tile.Tile, HuContext) (HuResult, bool)
}

// ClaimPolicy 决定吃碰杠胡等动作颗粒是否可用及其优先级。
type ClaimPolicy interface {
	Candidates(ctx ClaimContext) []ClaimAction
}

// RuleCapabilitiesProvider 由具备组合能力声明的规则实现。
type RuleCapabilitiesProvider interface {
	Capabilities() CapabilitySet
}

// Rule 为玩法变体接口；房间层只应依赖本接口而非具体实现包。
type Rule interface {
	ID() string
	Name() string
	BuildWall(ctx context.Context, seed int64) *wall.Wall
	CheckHu(h *hand.Hand, target tile.Tile, hc HuContext) (HuResult, bool)
	ScoreFans(result HuResult, sc ScoreContext) fan.Breakdown
	GameOver(state GameState) bool
}

// CapabilitiesOf 返回规则能力集合；未显式声明的旧规则使用保守默认能力。
func CapabilitiesOf(r Rule) CapabilitySet {
	if provider, ok := r.(RuleCapabilitiesProvider); ok {
		caps := provider.Capabilities()
		if caps.Claims == nil {
			caps.Claims = NoEatingClaimPolicy{}
		}
		return caps
	}
	return CapabilitySet{
		Metadata: RuleMetadata{
			DisplayName: "未命名麻将规则",
			ShortDesc:   "默认无吃牌抢答能力",
		},
		Claims: NoEatingClaimPolicy{},
	}
}

// StaticOpeningFlow 是固定开局步骤的简单实现。
type StaticOpeningFlow []string

// Steps 返回开局步骤副本。
func (f StaticOpeningFlow) Steps() []string {
	return append([]string(nil), f...)
}

// FeatureSet 是仅声明特性开关的简单能力实现。
type FeatureSet []string

// FeatureFlags 返回特性开关副本。
func (f FeatureSet) FeatureFlags() []string {
	return append([]string(nil), f...)
}

// NoEatingClaimPolicy 实现四川血战默认抢答：胡优先于杠，杠优先于碰，不开放吃。
type NoEatingClaimPolicy struct{}

// Candidates 返回当前座位可用抢答动作。
func (NoEatingClaimPolicy) Candidates(ctx ClaimContext) []ClaimAction {
	if ctx.Hued || ctx.Hand == nil || ctx.Tile == 0 || ctx.Seat == ctx.SourceSeat {
		return nil
	}
	out := make([]ClaimAction, 0, 3)
	if ctx.CheckHu != nil {
		if _, ok := ctx.CheckHu(ctx.Hand, ctx.Tile, ctx.HuContext); ok {
			action := ClaimAction{Name: ActionHu, ChoiceAction: "hu_choice", Priority: 3}
			if ctx.QiangGangWindow {
				action.ChoiceAction = "qiang_gang_choice"
			}
			out = append(out, action)
		}
	}
	if ctx.QiangGangWindow {
		return out
	}
	count := 0
	for _, current := range ctx.Hand.Tiles() {
		if current == ctx.Tile {
			count++
		}
	}
	if count >= 3 {
		out = append(out, ClaimAction{Name: ActionGang, ChoiceAction: "gang_choice", Priority: 2})
	}
	if count >= 2 {
		out = append(out, ClaimAction{Name: ActionPong, ChoiceAction: "pong_choice", Priority: 1})
	}
	return out
}

var (
	regMu sync.RWMutex
	reg   = map[string]Rule{}
)

// Register 注册规则实现；重复 ID 将 panic，避免静默覆盖。
func Register(r Rule) {
	if r == nil {
		panic("nil rule")
	}
	id := r.ID()
	if id == "" {
		panic("empty rule id")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := reg[id]; ok {
		panic(fmt.Sprintf("duplicate rule id: %s", id))
	}
	reg[id] = r
}

// MustGet 按 ID 获取规则；不存在则 panic（装配期错误应尽早暴露）。
func MustGet(id string) Rule {
	regMu.RLock()
	defer regMu.RUnlock()
	r, ok := reg[id]
	if !ok {
		panic(fmt.Sprintf("unknown rule id: %s", id))
	}
	return r
}

// List 返回当前已注册规则的稳定快照，按规则 ID 升序排列。
func List() []Rule {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Rule, 0, len(reg))
	for _, r := range reg {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID() < out[j].ID()
	})
	return out
}
