package room

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"

	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/clock"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

// ErrRoundPersistUnsupportedSchema 表示持久化局面版本超过当前服务可理解范围。
var ErrRoundPersistUnsupportedSchema = errors.New("unsupported round persist schema")

// Kind 表示 room 服务产出的局内通知种类，由 handler 适配为具体 msg_id。
type Kind string

const (
	KindInitialDeal Kind = "initial_deal"
	KindOpeningDone Kind = "opening_done"
	KindStartGame   Kind = "start_game"
	KindDrawTile    Kind = "draw_tile"
	KindAction      Kind = "action"
	KindSettlement  Kind = "settlement"
)

const defaultExchangeDirection int32 = 3

// Notification 为 room 服务产出的通知载荷；payload 已是 client.v1.Envelope 的序列化结果。
// Notification 是房间引擎向外投递的单条通知；TargetSeat 为 BroadcastSeat 时广播全桌。
// 每条 Notification 携带完整、独立的 Payload，不再含 Project 函数或 Privacy 字段——
// 需要差异化投影（如摸牌私密牌面、暗杠）时，引擎在生成时直接展开为每座位一条通知。
type Notification struct {
	Kind       Kind
	Payload    []byte
	TargetSeat Seat
}

// Engine 负责在单房上下文内生成确定性的血战流程通知。
//
// clk 与 tmo 被 engine 在 StartRound / RecoverRoom 时注入 RoundState，
// 供 RoundState.enterPhase 派生 deadlineUnixMs（详见 ADR-0045）。零值时回退到 clock.NewReal()
// 与 DefaultTimeoutConfig()，确保未通过 Service 装配的测试调用仍可工作。
type Engine struct {
	ruleID string
	clk    clock.Clock
	tmo    TimeoutConfig
}

// RoundState 保存交互式单局运行态，仅在 room actor 内被串行访问。
type RoundState struct {
	roomID      string
	ruleID      string
	playerIDs   [4]string
	surrendered []bool
	rule        rules.Rule
	caps        rules.CapabilitySet
	wall        *wall.Wall
	hands       []*hand.Hand
	ruleState   rules.RuleState
	discards    [][]tile.Tile
	flowers     [][]tile.Tile
	melds       [][]string
	lastAction  *LastActionInfo

	waitingOpening         bool
	waitingDiscard         bool
	waitingTsumo           bool
	pendingDraw            tile.Tile
	currentDraw            tile.Tile
	lastDiscard            tile.Tile
	lastDiscardSeat        Seat
	claimWindowOpen        bool
	claimCandidates        []claimCandidate
	qiangGangWindow        bool
	pendingGangSeat        Seat
	pendingGangTile        tile.Tile
	turn                   Seat
	step                   int
	dealerSeat             Seat
	openingDrawSeat        Seat
	dealerFirstDiscardOpen bool
	winEvents              []rules.WinEvent
	scoreEvents            []rules.ScoreEvent
	gangRecords            []rules.GangRecord
	lastGangFollowUp       bool
	lastDiscardAfterGang   bool
	closed                 bool
	deadlineUnixMs         int64
	// phaseReason 与 phaseStartUnixMs 由 enterPhase 一并写入，是 deadlineUnixMs 的派生来源。
	// 详见 ADR-0045 与 phase.go。
	phaseReason      WaitingReason
	phaseStartUnixMs int64
	// clk / tmo 由 engine 在 StartRound / RecoverRoom 时注入，使 enterPhase 能直接计算 deadline；
	// 不参与持久化（重启时由 service 重新注入）。
	clk clock.Clock
	tmo TimeoutConfig
}

type claimCandidate struct {
	seat         Seat
	actions      []string
	priority     int
	choiceAction string
}

type claimCandidatePersist struct {
	Seat    int      `json:"seat"`
	Actions []string `json:"actions"`
}

type roundPersist struct {
	SchemaVersion          int                     `json:"schema_version,omitempty"`
	RuleID                 string                  `json:"rule_id"`
	PlayerIDs              [4]string               `json:"player_ids"`
	WaitingOpening         bool                    `json:"waiting_opening,omitempty"`
	Turn                   int                     `json:"turn"`
	Step                   int                     `json:"step"`
	DealerSeat             int                     `json:"dealer_seat,omitempty"`
	OpeningDrawSeat        int                     `json:"opening_draw_seat"`
	DealerFirstDiscardOpen bool                    `json:"dealer_first_discard_open,omitempty"`
	WaitingDiscard         bool                    `json:"waiting_discard"`
	WaitingTsumo           bool                    `json:"waiting_tsumo"`
	PendingDraw            string                  `json:"pending_draw,omitempty"`
	CurrentDraw            string                  `json:"current_draw,omitempty"`
	LastDiscard            string                  `json:"last_discard,omitempty"`
	LastDiscardSeat        int                     `json:"last_discard_seat"`
	ClaimWindowOpen        bool                    `json:"claim_window_open,omitempty"`
	ClaimCandidates        []claimCandidatePersist `json:"claim_candidates,omitempty"`
	QiangGangWindow        bool                    `json:"qiang_gang_window,omitempty"`
	PendingGangSeat        int                     `json:"pending_gang_seat,omitempty"`
	PendingGangTile        string                  `json:"pending_gang_tile,omitempty"`
	SurrenderedSeats       []bool                  `json:"surrendered_seats,omitempty"`
	RuleState              rules.RuleState         `json:"rule_state,omitempty"`
	WinEvents              []rules.WinEvent        `json:"win_events,omitempty"`
	ScoreEvents            []rules.ScoreEvent      `json:"score_events,omitempty"`
	GangRecords            []rules.GangRecord      `json:"gang_records,omitempty"`
	LastGangFollowUp       bool                    `json:"last_gang_follow_up,omitempty"`
	LastDiscardAfterGang   bool                    `json:"last_discard_after_gang,omitempty"`
	Hands                  [][]string              `json:"hands"`
	Discards               [][]string              `json:"discards,omitempty"`
	Flowers                [][]string              `json:"flowers,omitempty"`
	Melds                  [][]string              `json:"melds,omitempty"`
	WallRemaining          []string                `json:"wall_remaining"`
	// PhaseReason 与 PhaseStartUnixMs 由 ADR-0045 引入；为 schema v3 兼容增量字段。
	// 老快照该两字段为零；恢复路径在 finalizeRoundInvariants 中由 waiting flags 派生 PhaseReason，
	// PhaseStartUnixMs=0 表示无锚点，scheduler.armUntil 触发立即超时（cmdAutoTimeout）。
	PhaseReason      int   `json:"phase_reason,omitempty"`
	PhaseStartUnixMs int64 `json:"phase_start_unix_ms,omitempty"`
}

// RoundView 描述客户端恢复时所需的最小等待态摘要。
type RoundView struct {
	ActingSeat       int32
	ActingSeats      []int32
	WaitingAction    string
	Phase            Phase
	LastStep         int64
	PendingTile      string
	AvailableActions []string
	ClaimCandidates  []RoundClaimCandidate
	HandsBySeat      [][]string
	DiscardsBySeat   [][]string
	MeldsBySeat      [][]string
	MeldInfosBySeat  []*SeatMelds
	// 以下字段供重连快照重建视图使用，对老调用方可零值兼容。
	QueBySeat        []int32
	PlayerIDs        [4]string
	HuedSeats        []bool
	Closed           bool
	OpeningSubmitted map[string][]bool
	LastAction       *LastActionInfo
	WallRemaining    int32
	DeadlineUnixMs   int64
	RoundIndex       int32
	HandIndex        int32
	TotalScores      []*rules.SeatScore
	RuleMeta         *RuleMeta
}

// RoundProgress 是局内进度的权威投影，不承载房间生命周期或 UI 文案。
// Reason 与 ServerNowUnixMs 是 ADR-0045 引入的派生字段，供 toPhaseUpdate 构建 PhaseUpdate；
// 老调用方读取 DeadlineUnixMs 等字段不受影响。
type RoundProgress struct {
	ActingSeat       int32
	ActingSeats      []int32
	WaitingAction    string
	Phase            Phase
	Step             int64
	PendingTile      string
	AvailableActions []string
	ClaimCandidates  []RoundClaimCandidate
	WallRemaining    int32
	DeadlineUnixMs   int64
	Reason           WaitingReason
	ServerNowUnixMs  int64
}

// toPhaseUpdate 构造嵌入到 Notify / Response 的 PhaseUpdate；详见 ADR-0045。
func (p RoundProgress) toPhaseUpdate() *clientv1.PhaseUpdate {
	return &clientv1.PhaseUpdate{
		Phase:            p.Phase.Proto(),
		Step:             p.Step,
		Reason:           p.Reason.Proto(),
		DeadlineUnixMs:   p.DeadlineUnixMs,
		ServerNowUnixMs:  p.ServerNowUnixMs,
		ActingSeats:      append([]int32(nil), p.ActingSeats...),
		AvailableActions: append([]string(nil), p.AvailableActions...),
	}
}

// RoundFacts 是局内可见事实投影；调用方不得从 UI 日志反推这些字段。
type RoundFacts struct {
	HandsBySeat     [][]string
	DiscardsBySeat  [][]string
	MeldsBySeat     [][]string
	MeldInfosBySeat []*SeatMelds
	QueBySeat       []int32
	PlayerIDs       [4]string
	HuedSeats       []bool
	Closed          bool
	LastAction      *LastActionInfo
	RoundIndex      int32
	HandIndex       int32
	TotalScores     []*rules.SeatScore
	RuleMeta        *RuleMeta
}

// RoundProjection 聚合 room service 对外暴露的局内事实。
type RoundProjection struct {
	Progress RoundProgress
	Facts    RoundFacts
}

// RoundClaimCandidate 描述恢复快照中仍有效的抢答候选。
type RoundClaimCandidate struct {
	Seat    int32
	Actions []string
}

// NewEngine 创建牌局引擎；ruleID 为空时回退到四川血战到底默认规则。
// 时钟与 timeout 默认为真实时钟与 DefaultTimeoutConfig；Service 装配时会通过
// SetClock / SetTimeoutConfig 注入正确值，再传入 RoundState。
func NewEngine(ruleID string) *Engine {
	if ruleID == "" {
		ruleID = "sichuan_xuezhandaodi_huansanzhang"
	}
	return &Engine{ruleID: ruleID, clk: clock.NewReal(), tmo: DefaultTimeoutConfig()}
}

func (rs *RoundState) ensureRuleRuntime() {
	if rs == nil {
		return
	}
	if rs.rule == nil {
		ruleID := rs.ruleID
		if ruleID == "" {
			ruleID = "sichuan_xuezhandaodi_huansanzhang"
		}
		rs.rule = rules.MustGet(ruleID)
		rs.ruleID = ruleID
	}
	if rs.caps.TileSet == nil && rs.caps.Claims == nil && rs.caps.Win == nil && rs.caps.Scoring == nil && rs.caps.Termination == nil {
		rs.caps = rules.CapabilitiesOf(rs.rule)
	}
	rs.normalizeRuleState()
}

// SetClock 注入引擎使用的时钟源，影响 enterPhase 写入的 phaseStartUnixMs。
func (e *Engine) SetClock(clk clock.Clock) {
	if e == nil || clk == nil {
		return
	}
	e.clk = clk
}

// SetTimeoutConfig 注入引擎使用的等待时长配置，影响 enterPhase 派生的 deadlineUnixMs。
func (e *Engine) SetTimeoutConfig(cfg TimeoutConfig) {
	if e == nil {
		return
	}
	e.tmo = cfg.withDefaults()
}

// StartRound 初始化交互式牌局，并推进到首个等待出牌的状态。
func (e *Engine) StartRound(ctx context.Context, roomID string, playerIDs [4]string) (*RoundState, []Notification, error) {
	if e == nil {
		return nil, nil, fmt.Errorf("nil engine")
	}
	rule := rules.MustGet(e.ruleID)
	caps := rules.CapabilitiesOf(rule)
	rs := &RoundState{
		roomID:          roomID,
		ruleID:          e.ruleID,
		playerIDs:       playerIDs,
		surrendered:     make([]bool, 4),
		rule:            rule,
		caps:            caps,
		wall:            caps.TileSet.BuildWall(ctx, int64(seedFromRoomID(roomID)&0x7fff_ffff_ffff_ffff)), //nolint:gosec // 已清零最高位
		hands:           make([]*hand.Hand, 4),
		ruleState:       newInitialRuleState(caps),
		discards:        make([][]tile.Tile, 4),
		flowers:         make([][]tile.Tile, 4),
		melds:           make([][]string, 4),
		lastDiscardSeat: -1,
		dealerSeat:      0,
		openingDrawSeat: 0,
		winEvents:       make([]rules.WinEvent, 0, 3),
		scoreEvents:     make([]rules.ScoreEvent, 0, 16),
		clk:             e.clk,
		tmo:             e.tmo,
		phaseReason:     ReasonNone,
	}
	for i := range rs.hands {
		rs.hands[i] = hand.New()
	}
	for round := 0; round < 13; round++ {
		for seat := 0; seat < 4; seat++ {
			t, err := e.drawNextPlayableTile(rs, Seat(seat))
			if err != nil {
				return nil, nil, err
			}
			rs.hands[seat].Add(t)
		}
	}

	initial, err := rs.initialDealNotifications()
	if err != nil {
		return nil, nil, err
	}
	out, err := e.initRoundNotifications(ctx, rs)
	if err != nil {
		return nil, nil, err
	}
	return rs, append(initial, out...), nil
}

func (rs *RoundState) initialDealNotifications() ([]Notification, error) {
	if rs == nil {
		return nil, fmt.Errorf("nil round state")
	}
	out := make([]Notification, 0, 4)
	for seat := 0; seat < 4; seat++ {
		seatID := Seat(seat)
		seatIndex := seatID.Proto()
		payload, err := marshalEnvelope(&clientv1.Envelope{
			ReqId: fmt.Sprintf("initial-deal-%d", seat),
			Body: &clientv1.Envelope_InitialDeal{
				InitialDeal: &clientv1.InitialDealNotify{
					SeatIndex: seatIndex,
					Tiles:     tilesToStrings(rs.hands[seat].Tiles()),
					Step:      int64(rs.step),
				},
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, Notification{Kind: KindInitialDeal, Payload: payload, TargetSeat: seatID})
	}
	return out, nil
}

func marshalEnvelope(env *clientv1.Envelope) ([]byte, error) {
	if env == nil {
		return nil, fmt.Errorf("nil envelope")
	}
	return proto.Marshal(env)
}

func seedFromRoomID(roomID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(roomID))
	return h.Sum64()
}
