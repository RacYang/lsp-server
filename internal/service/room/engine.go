package room

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"

	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

// ErrRoundPersistUnsupportedSchema 表示持久化局面版本超过当前服务可理解范围。
var ErrRoundPersistUnsupportedSchema = errors.New("unsupported round persist schema")

// Kind 表示 room 服务产出的局内通知种类，由 handler 适配为具体 msg_id。
type Kind string

const (
	KindInitialDeal       Kind = "initial_deal"
	KindExchangeThreeDone Kind = "exchange_three_done"
	KindQueMenDone        Kind = "que_men_done"
	KindStartGame         Kind = "start_game"
	KindDrawTile          Kind = "draw_tile"
	KindAction            Kind = "action"
	KindSettlement        Kind = "settlement"
)

const defaultExchangeDirection int32 = 3

// Notification 为 room 服务产出的通知载荷；payload 已是 client.v1.Envelope 的序列化结果。
type Notification struct {
	Kind       Kind
	Payload    []byte
	TargetSeat Seat
	Privacy    NotificationPrivacy
	Project    func(Seat) []byte
}

// NotificationPrivacy 标识通知是否需要按座位投影后下发。
type NotificationPrivacy int

const (
	// PrivacyPublic 表示 payload 可原样广播给全桌。
	PrivacyPublic NotificationPrivacy = iota
	// PrivacyPerSeat 表示 payload 必须经 Project 按目标座位改写。
	PrivacyPerSeat
)

// Engine 负责在单房上下文内生成确定性的血战流程通知。
type Engine struct {
	ruleID string
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
	queBySeat   []int32
	discards    [][]tile.Tile
	melds       [][]string
	lastAction  *clientv1.LastActionInfo

	waitingExchange        bool
	exchangeDirection      int32
	waitingQueMen          bool
	exchangeSubmitted      []bool
	exchangeSelection      [][]tile.Tile
	queSubmitted           []bool
	waitingDiscard         bool
	waitingTsumo           bool
	pendingDraw            tile.Tile
	currentDraw            tile.Tile
	lastDiscard            tile.Tile
	lastDiscardSeat        Seat
	claimWindowOpen        bool
	claimCandidates        []claimCandidate
	qiangGangWindow        bool
	turn                   Seat
	step                   int
	dealerSeat             Seat
	openingDrawSeat        Seat
	dealerFirstDiscardOpen bool
	huedSeats              []bool
	winnerSeats            []Seat
	ledger                 []xuezhandaodi.ScoreEntry
	gangRecords            []rules.GangRecord
	lastGangFollowUp       bool
	lastDiscardAfterGang   bool
	closed                 bool
	deadlineUnixMs         int64
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
	SchemaVersion          int                       `json:"schema_version,omitempty"`
	RuleID                 string                    `json:"rule_id"`
	PlayerIDs              [4]string                 `json:"player_ids"`
	QueBySeat              []int32                   `json:"que_by_seat"`
	WaitingExchange        bool                      `json:"waiting_exchange"`
	ExchangeDir            int32                     `json:"exchange_dir,omitempty"`
	WaitingQueMen          bool                      `json:"waiting_que_men"`
	ExchangeDone           []bool                    `json:"exchange_done,omitempty"`
	ExchangeTiles          [][]string                `json:"exchange_tiles,omitempty"`
	QueDone                []bool                    `json:"que_done,omitempty"`
	Turn                   int                       `json:"turn"`
	Step                   int                       `json:"step"`
	DealerSeat             int                       `json:"dealer_seat,omitempty"`
	OpeningDrawSeat        int                       `json:"opening_draw_seat"`
	DealerFirstDiscardOpen bool                      `json:"dealer_first_discard_open,omitempty"`
	WaitingDiscard         bool                      `json:"waiting_discard"`
	WaitingTsumo           bool                      `json:"waiting_tsumo"`
	PendingDraw            string                    `json:"pending_draw,omitempty"`
	CurrentDraw            string                    `json:"current_draw,omitempty"`
	LastDiscard            string                    `json:"last_discard,omitempty"`
	LastDiscardSeat        int                       `json:"last_discard_seat"`
	ClaimWindowOpen        bool                      `json:"claim_window_open,omitempty"`
	ClaimCandidates        []claimCandidatePersist   `json:"claim_candidates,omitempty"`
	QiangGangWindow        bool                      `json:"qiang_gang_window,omitempty"`
	WinnerSeat             int                       `json:"winner_seat,omitempty"` // 兼容 schema_version=0/1 的单赢家快照
	WinnerSeats            []int                     `json:"winner_seats,omitempty"`
	HuedSeats              []bool                    `json:"hued_seats,omitempty"`
	TotalFanBySeat         []int32                   `json:"total_fan_by_seat,omitempty"` // 兼容 schema_version=0/1
	Ledger                 []xuezhandaodi.ScoreEntry `json:"ledger,omitempty"`
	GangRecords            []rules.GangRecord        `json:"gang_records,omitempty"`
	LastGangFollowUp       bool                      `json:"last_gang_follow_up,omitempty"`
	LastDiscardAfterGang   bool                      `json:"last_discard_after_gang,omitempty"`
	Hands                  [][]string                `json:"hands"`
	Discards               [][]string                `json:"discards,omitempty"`
	Melds                  [][]string                `json:"melds,omitempty"`
	WallRemaining          []string                  `json:"wall_remaining"`
}

// RoundView 描述客户端恢复时所需的最小等待态摘要。
type RoundView struct {
	ActingSeat       int32
	ActingSeats      []int32
	WaitingAction    string
	Phase            clientv1.Phase
	LastStep         int64
	PendingTile      string
	AvailableActions []string
	ClaimCandidates  []RoundClaimCandidate
	HandsBySeat      [][]string
	DiscardsBySeat   [][]string
	MeldsBySeat      [][]string
	MeldInfosBySeat  []*clientv1.SeatMelds
	// 以下字段供 BotSupervisor 与重连快照重建 bot 视图使用，对老调用方可零值兼容。
	QueBySeat         []int32
	PlayerIDs         [4]string
	HuedSeats         []bool
	Closed            bool
	ExchangeSubmitted []bool
	QueSubmitted      []bool
	LastAction        *clientv1.LastActionInfo
	WallRemaining     int32
	DeadlineUnixMs    int64
	RoundIndex        int32
	HandIndex         int32
	TotalScores       []*clientv1.SeatScore
	RuleMeta          *clientv1.RuleMeta
}

// RoundClaimCandidate 描述恢复快照中仍有效的抢答候选。
type RoundClaimCandidate struct {
	Seat    int32
	Actions []string
}

// NewEngine 创建牌局引擎；ruleID 为空时回退到四川血战到底默认规则。
func NewEngine(ruleID string) *Engine {
	if ruleID == "" {
		ruleID = "sichuan_xuezhandaodi_huansanzhang"
	}
	return &Engine{ruleID: ruleID}
}

// StartRound 初始化交互式牌局，并推进到首个等待出牌的状态。
func (e *Engine) StartRound(ctx context.Context, roomID string, playerIDs [4]string) (*RoundState, []Notification, error) {
	if e == nil {
		return nil, nil, fmt.Errorf("nil engine")
	}
	rule := rules.MustGet(e.ruleID)
	caps := rules.CapabilitiesOf(rule)
	rs := &RoundState{
		roomID:            roomID,
		ruleID:            e.ruleID,
		playerIDs:         playerIDs,
		surrendered:       make([]bool, 4),
		rule:              rule,
		caps:              caps,
		wall:              rule.BuildWall(ctx, int64(seedFromRoomID(roomID)&0x7fff_ffff_ffff_ffff)), //nolint:gosec // 已清零最高位
		hands:             make([]*hand.Hand, 4),
		discards:          make([][]tile.Tile, 4),
		melds:             make([][]string, 4),
		queBySeat:         make([]int32, 4),
		exchangeSubmitted: make([]bool, 4),
		exchangeDirection: -1,
		exchangeSelection: make([][]tile.Tile, 4),
		queSubmitted:      make([]bool, 4),
		lastDiscardSeat:   -1,
		dealerSeat:        0,
		openingDrawSeat:   0,
		huedSeats:         make([]bool, 4),
		winnerSeats:       make([]Seat, 0, 3),
		ledger:            make([]xuezhandaodi.ScoreEntry, 0, 16),
	}
	for i := range rs.hands {
		rs.hands[i] = hand.New()
	}
	for round := 0; round < 13; round++ {
		for seat := 0; seat < 4; seat++ {
			t, err := rs.wall.Draw()
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
	out, err := rs.initRoundNotifications()
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
