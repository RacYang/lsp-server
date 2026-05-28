// Package rules 定义可插拔麻将规则接口与注册表；具体变体放在子目录中实现。
package rules

import (
	"context"
	"encoding/json"
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
	RuleState       RuleState
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
	WinningTile          tile.Tile
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
	ActiveSeats          []domainroom.Seat
	Step                 int
}

// RuleState 保存规则私有运行态的 opaque 载体。room 只能持久化和转交，不得解释 Data。
type RuleState struct {
	SchemaVersion int             `json:"schema_version,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
}

// Normalize 保留现有调用点；opaque RuleState 不在 room 侧补齐任何玩法字段。
func (s *RuleState) Normalize(int) {}

// MarshalJSON 只写 opaque schema/data。
func (s RuleState) MarshalJSON() ([]byte, error) {
	type wire RuleState
	return json.Marshal(wire(s))
}

// UnmarshalJSON 只接受当前 opaque schema/data 形态。
func (s *RuleState) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key := range raw {
		switch key {
		case "schema_version", "data":
		default:
			return fmt.Errorf("unsupported rule_state field %q", key)
		}
	}
	type wire struct {
		SchemaVersion int             `json:"schema_version,omitempty"`
		Data          json.RawMessage `json:"data,omitempty"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	s.SchemaVersion = w.SchemaVersion
	s.Data = append(json.RawMessage(nil), w.Data...)
	return nil
}

// RuleStateProjection 是规则状态给 room/协议兼容层的只读投影。
type RuleStateProjection struct {
	SeatInts                 map[string][]int32
	OpeningSubmittedByAction map[string][]bool
}

// RuleStateCodec 由规则包负责初始化和规范化当前私有状态。
type RuleStateCodec interface {
	InitialRuleState() RuleState
	NormalizeRuleState(RuleState) RuleState
}

// RuleStateProjector 由规则包把私有状态投影为协议兼容事实。
type RuleStateProjector interface {
	ProjectRuleState(RuleState) RuleStateProjection
}

// WinEvent 是通用胡牌事实；终局、轮转与结算都从该事实派生。
type WinEvent struct {
	Seat     domainroom.Seat `json:"seat"`
	Source   HuSource        `json:"source"`
	Tile     tile.Tile       `json:"tile"`
	FromSeat domainroom.Seat `json:"from_seat"`
	Step     int             `json:"step"`
	TotalFan int32           `json:"total_fan,omitempty"`
	FanNames []string        `json:"fan_names,omitempty"`
}

// ScoreEvent 是通用局内计分流水；FromSeat/ToSeat 为 -1 时表示系统池。
// 具体玩法可以用 Reason 和 FanNames 承载规则内语义，但 room 只把它当成可折叠流水。
type ScoreEvent struct {
	Reason     string          `json:"reason"`
	FromSeat   domainroom.Seat `json:"from_seat"`
	ToSeat     domainroom.Seat `json:"to_seat"`
	Amount     int32           `json:"amount"`
	Step       int             `json:"step"`
	WinnerSeat domainroom.Seat `json:"winner_seat"`
	WinnerFan  int32           `json:"winner_fan,omitempty"`
	FanNames   []string        `json:"fan_names,omitempty"`
}

// SettlementContext 是规则策略生成局末结算时可见的通用事实。
type SettlementContext struct {
	PlayerIDs   [4]string
	Hands       []*hand.Hand
	RuleState   RuleState
	WinEvents   []WinEvent
	ScoreEvents []ScoreEvent
}

// SeatScore 是单座位的结算得分，与传输层解耦的内部类型。
type SeatScore struct {
	SeatIndex int32
	UserID    string
	TotalFan  int32
	Skipped   bool
}

// PenaltyItem 是单条罚分或退税记录，与传输层解耦的内部类型。
type PenaltyItem struct {
	Reason   string
	FromSeat int32
	ToSeat   int32
	Amount   int32
}

// WinnerBreakdown 是胡牌玩家的番种分解，与传输层解耦的内部类型。
type WinnerBreakdown struct {
	SeatIndex int32
	UserID    string
	Fan       int32
	FanNames  []string
}

// SettlementResult 是 room 折叠成 SettlementNotify 所需的规则无关结算投影。
type SettlementResult struct {
	WinnerUserIDs      []string
	SeatScores         []*SeatScore
	Penalties          []*PenaltyItem
	PerWinnerBreakdown []*WinnerBreakdown
	DetailText         string
}

// MeldContext 是规则层计算胡牌与番种所需的副露事实。
type MeldContext struct {
	Kind      string
	Tiles     []tile.Tile
	Concealed bool
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
	TileSet     TileSetPolicy
	Wall        WallPolicy
	Opening     OpeningPolicy
	Claims      ClaimPolicy
	SelfActions SelfActionPolicy
	Win         WinPolicy
	State       RuleStateCodec
	StateView   RuleStateProjector
	Turn        TurnFlow
	Scoring     ScoringPolicy
	Settlement  SettlementPolicy
	Termination TerminationPolicy
	Projection  RoundProjection
}

// TileSetPolicy 构建规则使用的牌墙。
type TileSetPolicy interface {
	BuildWall(ctx context.Context, seed int64) *wall.Wall
}

// WallPolicy 描述摸牌墙相关能力，例如花牌补摸、王牌和岭上牌。
type WallPolicy interface {
	FeatureFlags() []string
}

// OpeningActionName 是开局动作稳定名称；具体玩法动作名由规则包声明。
type OpeningActionName string

// OpeningStep 描述当前开局阶段的协议动作与完成通知投影类型。
type OpeningStep struct {
	ID           string
	Action       string
	Reason       string
	CompleteKind string
}

// OpeningContext 是开局策略可见的局面事实。
type OpeningContext struct {
	RuleState RuleState
	Hands     []*hand.Hand
}

// OpeningActionContext 是开局动作策略可见的输入。
type OpeningActionContext struct {
	OpeningContext
	Seat      domainroom.Seat
	Action    OpeningActionName
	Tiles     []string
	Suit      int32
	Direction int32
	Params    map[string]string
	Timeout   bool
	Surrender bool
}

// OpeningResult 是开局策略动作后的通用结果。
type OpeningResult struct {
	RuleState          RuleState
	Hands              []*hand.Hand
	CompletedStep      *OpeningStep
	NextStep           *OpeningStep
	AllOpeningComplete bool
	Notifications      []OpeningNotification
}

// OpeningSeatTilesProjection 描述一个 seat-indexed tile 投影项。
type OpeningSeatTilesProjection struct {
	Seat  domainroom.Seat
	Tiles []string
}

// OpeningDoneProjection 是开局策略生成的通用完成投影。
type OpeningDoneProjection struct {
	Action     string
	StepID     string
	Kind       string
	Params     map[string]string
	SeatTiles  map[string][]OpeningSeatTilesProjection
	SeatInts   map[string][]int32
	LocalTiles map[domainroom.Seat]map[string][]string
}

// OpeningNotification 是开局策略生成的通用协议投影。room 只负责补进度并发送。
type OpeningNotification struct {
	Done OpeningDoneProjection
}

// OpeningPolicy 驱动规则开局流程；room 只转发动作并投影结果。
type OpeningPolicy interface {
	Steps() []string
	InitialState() RuleState
	CurrentStep(OpeningContext) (*OpeningStep, bool)
	Apply(OpeningActionContext) (OpeningResult, error)
}

// TurnFlow 描述摸牌、自摸窗口、杠后补牌与海底处理等轮转能力。
type TurnFlow interface {
	FeatureFlags() []string
	HuedSeatContinues() bool
}

// WinPolicy 判断胡牌合法性。
type WinPolicy interface {
	CheckHu(h *hand.Hand, target tile.Tile, hc HuContext) (HuResult, bool)
}

// ScoringPolicy 生成规则计分结果与分数事件。
type ScoringPolicy interface {
	FeatureFlags() []string
	ScoreWin(result HuResult, sc ScoreContext) (fan.Breakdown, []ScoreEvent, bool)
	ScoreGang(sc GangScoreContext) ([]ScoreEvent, GangRecord)
}

// GangScoreContext 是规则计算杠分流水时可见的事实。
type GangScoreContext struct {
	Seat        domainroom.Seat
	Kind        GangKind
	Tile        tile.Tile
	FromSeat    domainroom.Seat
	ActiveSeats []domainroom.Seat
	Step        int
}

// SettlementPolicy 是规则结算能力；房间层不得依赖具体规则包类型。
type SettlementPolicy interface {
	FeatureFlags() []string
	BuildSettlement(SettlementContext) SettlementResult
}

// TerminationContext 是终局策略可见的局面事实。
type TerminationContext struct {
	WallRemaining int
	WinEvents     []WinEvent
	ActiveSeats   []domainroom.Seat
	RuleState     RuleState
}

// TerminationPolicy 是终局能力。
type TerminationPolicy interface {
	FeatureFlags() []string
	GameOver(TerminationContext) bool
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
	RuleState       RuleState
	Melds           []MeldContext
}

// ClaimPolicy 决定吃碰杠胡等动作颗粒是否可用及其优先级。
type ClaimPolicy interface {
	Candidates(ctx ClaimContext) []ClaimAction
}

// SelfActionContext 是摸牌后自行动作策略可见的上下文。
type SelfActionContext struct {
	Seat      domainroom.Seat
	Tile      tile.Tile
	Hand      *hand.Hand
	RuleState RuleState
	Melds     []MeldContext
}

// SelfActionPolicy 决定暗杠、补杠等当前座位自行动作是否合法。
type SelfActionPolicy interface {
	CanAnGang(SelfActionContext) bool
	CanBuGang(SelfActionContext) bool
}

// RuleCapabilitiesProvider 由具备组合能力声明的规则实现。
type RuleCapabilitiesProvider interface {
	Capabilities() CapabilitySet
}

// Rule 为玩法变体注册入口；运行能力必须通过 RuleCapabilitiesProvider 显式声明。
type Rule interface {
	ID() string
	Name() string
}

// CapabilitiesOf 返回规则能力集合；规则必须显式声明运行策略。
func CapabilitiesOf(r Rule) CapabilitySet {
	if provider, ok := r.(RuleCapabilitiesProvider); ok {
		caps := provider.Capabilities()
		mustHaveRuntimeCapabilities(r.ID(), caps)
		return caps
	}
	panic(fmt.Sprintf("rule %q must implement RuleCapabilitiesProvider", r.ID()))
}

func mustHaveRuntimeCapabilities(ruleID string, caps CapabilitySet) {
	required := []struct {
		name string
		ok   bool
	}{
		{name: "TileSetPolicy", ok: caps.TileSet != nil},
		{name: "ClaimPolicy", ok: caps.Claims != nil},
		{name: "SelfActionPolicy", ok: caps.SelfActions != nil},
		{name: "WinPolicy", ok: caps.Win != nil},
		{name: "RuleStateCodec", ok: caps.State != nil},
		{name: "RuleStateProjector", ok: caps.StateView != nil},
		{name: "TurnFlow", ok: caps.Turn != nil},
		{name: "ScoringPolicy", ok: caps.Scoring != nil},
		{name: "SettlementPolicy", ok: caps.Settlement != nil},
		{name: "TerminationPolicy", ok: caps.Termination != nil},
		{name: "RoundProjection", ok: caps.Projection != nil},
	}
	for _, item := range required {
		if !item.ok {
			panic(fmt.Sprintf("rule %q must declare %s", ruleID, item.name))
		}
	}
}

// EmptyRuleStatePolicy 是无规则私有状态玩法的默认 opaque state 实现。
type EmptyRuleStatePolicy struct{}

func (EmptyRuleStatePolicy) InitialRuleState() RuleState                  { return RuleState{} }
func (EmptyRuleStatePolicy) NormalizeRuleState(state RuleState) RuleState { return state }
func (EmptyRuleStatePolicy) ProjectRuleState(RuleState) RuleStateProjection {
	return RuleStateProjection{}
}

// StandardSelfActionPolicy 实现通用暗杠、补杠合法性，不包含任何地方玩法限制。
type StandardSelfActionPolicy struct{}

func (StandardSelfActionPolicy) CanAnGang(ctx SelfActionContext) bool {
	if ctx.Hand == nil || ctx.Tile == 0 {
		return false
	}
	count := 0
	for _, t := range ctx.Hand.Tiles() {
		if t == ctx.Tile {
			count++
		}
	}
	return count >= 4
}

func (StandardSelfActionPolicy) CanBuGang(ctx SelfActionContext) bool {
	if ctx.Hand == nil || ctx.Tile == 0 {
		return false
	}
	hasTile := false
	for _, t := range ctx.Hand.Tiles() {
		if t == ctx.Tile {
			hasTile = true
			break
		}
	}
	if !hasTile {
		return false
	}
	for _, meld := range ctx.Melds {
		if meld.Kind != "pong" || len(meld.Tiles) < 3 {
			continue
		}
		ok := true
		for _, t := range meld.Tiles[:3] {
			if t != ctx.Tile {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// StaticOpeningFlow 是固定开局步骤的简单实现。
type StaticOpeningFlow []string

type staticOpeningState struct {
	Submitted map[string][]bool `json:"submitted,omitempty"`
}

// Steps 返回开局步骤副本。
func (f StaticOpeningFlow) Steps() []string {
	return append([]string(nil), f...)
}

func (f StaticOpeningFlow) InitialState() RuleState {
	state := staticOpeningState{Submitted: map[string][]bool{}}
	for _, step := range f {
		state.Submitted[step] = make([]bool, 4)
	}
	return mustRuleStateJSON(state)
}

func (f StaticOpeningFlow) CurrentStep(ctx OpeningContext) (*OpeningStep, bool) {
	state := decodeStaticOpeningState(ctx.RuleState)
	for _, step := range f {
		done := state.Submitted[step]
		if len(done) < 4 {
			return &OpeningStep{ID: step, Action: step, Reason: step}, true
		}
		for _, submitted := range done[:4] {
			if !submitted {
				return &OpeningStep{ID: step, Action: step, Reason: step}, true
			}
		}
	}
	return nil, false
}

func (f StaticOpeningFlow) Apply(ctx OpeningActionContext) (OpeningResult, error) {
	state := decodeStaticOpeningState(ctx.RuleState)
	current := mustRuleStateJSON(state)
	step, ok := f.CurrentStep(OpeningContext{RuleState: current, Hands: ctx.Hands})
	if !ok {
		return OpeningResult{RuleState: current, Hands: ctx.Hands, AllOpeningComplete: true}, nil
	}
	done := state.Submitted[step.ID]
	for len(done) < 4 {
		done = append(done, false)
	}
	if ctx.Seat >= 0 && int(ctx.Seat) < len(done) {
		done[ctx.Seat] = true
	}
	state.Submitted[step.ID] = done
	nextState := mustRuleStateJSON(state)
	result := OpeningResult{RuleState: nextState, Hands: ctx.Hands}
	if next, ok := f.CurrentStep(OpeningContext{RuleState: nextState, Hands: ctx.Hands}); ok {
		result.NextStep = next
		return result, nil
	}
	result.AllOpeningComplete = true
	return result, nil
}

func decodeStaticOpeningState(state RuleState) staticOpeningState {
	var out staticOpeningState
	_ = json.Unmarshal(state.Data, &out)
	if out.Submitted == nil {
		out.Submitted = map[string][]bool{}
	}
	return out
}

// FeatureSet 是仅声明特性开关的简单能力实现。
type FeatureSet []string

// FeatureFlags 返回特性开关副本。
func (f FeatureSet) FeatureFlags() []string {
	return append([]string(nil), f...)
}

// HuedSeatContinues 返回标准胡后退出语义；需要血流等玩法时使用专门 TurnFlow。
func (f FeatureSet) HuedSeatContinues() bool { return false }

// BuildSettlement 提供规则无私有结算需求时的默认零和流水折叠。
func (f FeatureSet) BuildSettlement(ctx SettlementContext) SettlementResult {
	winners := make([]domainroom.Seat, 0, len(ctx.WinEvents))
	seenWinners := map[domainroom.Seat]struct{}{}
	for _, event := range ctx.WinEvents {
		if _, ok := seenWinners[event.Seat]; ok {
			continue
		}
		seenWinners[event.Seat] = struct{}{}
		winners = append(winners, event.Seat)
	}
	winnerIDs := make([]string, 0, len(winners))
	for _, seat := range winners {
		if seat >= 0 && int(seat) < len(ctx.PlayerIDs) {
			winnerIDs = append(winnerIDs, ctx.PlayerIDs[seat])
		}
	}
	balances := [4]int32{}
	for _, event := range ctx.ScoreEvents {
		if event.FromSeat >= 0 && int(event.FromSeat) < len(balances) {
			balances[event.FromSeat] -= event.Amount
		}
		if event.ToSeat >= 0 && int(event.ToSeat) < len(balances) {
			balances[event.ToSeat] += event.Amount
		}
	}
	scores := make([]*SeatScore, 0, len(balances))
	for seat, total := range balances {
		scores = append(scores, &SeatScore{
			SeatIndex: int32(seat), //nolint:gosec // 固定座位范围 0..3
			UserID:    ctx.PlayerIDs[seat],
			TotalFan:  total,
		})
	}
	detail := "本局结束"
	if len(winnerIDs) == 0 {
		detail = "荒牌"
	}
	return SettlementResult{WinnerUserIDs: winnerIDs, SeatScores: scores, DetailText: detail}
}

func mustRuleStateJSON(v any) RuleState {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return RuleState{Data: data}
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

// StandardClaimPolicy 实现通用麻将抢答：胡 > 杠 > 碰 > 吃，吃只允许下家且只限数牌顺子。
type StandardClaimPolicy struct{}

// Candidates 返回标准吃碰杠胡候选动作。
func (StandardClaimPolicy) Candidates(ctx ClaimContext) []ClaimAction {
	if ctx.Hued || ctx.Hand == nil || ctx.Tile == 0 || ctx.Seat == ctx.SourceSeat {
		return nil
	}
	out := make([]ClaimAction, 0, 4)
	if ctx.CheckHu != nil {
		if _, ok := ctx.CheckHu(ctx.Hand, ctx.Tile, ctx.HuContext); ok {
			action := ClaimAction{Name: ActionHu, ChoiceAction: "hu_choice", Priority: 4}
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
		out = append(out, ClaimAction{Name: ActionGang, ChoiceAction: "gang_choice", Priority: 3})
	}
	if count >= 2 {
		out = append(out, ClaimAction{Name: ActionPong, ChoiceAction: "pong_choice", Priority: 2})
	}
	if canStandardChi(ctx) {
		out = append(out, ClaimAction{Name: ActionChi, ChoiceAction: "chi_choice", Priority: 1})
	}
	return out
}

func canStandardChi(ctx ClaimContext) bool {
	if !ctx.Tile.IsSuited() {
		return false
	}
	if ctx.SourceSeat < 0 || ctx.Seat != domainroom.Seat((int(ctx.SourceSeat)+1)%4) {
		return false
	}
	counts := map[tile.Tile]int{}
	for _, current := range ctx.Hand.Tiles() {
		counts[current]++
	}
	suit := ctx.Tile.Suit()
	rank := ctx.Tile.Rank()
	for start := rank - 2; start <= rank; start++ {
		if start < 1 || start > 7 {
			continue
		}
		need := []int{start, start + 1, start + 2}
		ok := true
		for _, r := range need {
			t := tile.Must(suit, r)
			required := 1
			if t == ctx.Tile {
				required = 0
			}
			if counts[t] < required {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
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
