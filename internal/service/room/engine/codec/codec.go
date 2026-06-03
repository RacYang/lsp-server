package codec

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// Phase 常量与 engine.Phase 枚举值一一对应，数值顺序不得擅自修改。
// 用于将 engine 内部阶段枚举转换为 proto 枚举，而无需在 engine 主包引入 proto 依赖。
const (
	PhaseUnspecified = 0 // 未指定
	PhaseOpening     = 1 // 开局阶段
	PhaseClaim       = 2 // 抢答窗口
	PhaseTsumo       = 3 // 自摸待决
	PhaseDiscard     = 4 // 等待弃牌
	PhaseClosed      = 5 // 已结束
	PhaseDraw        = 6 // 摸牌过渡
)

// Reason 常量与 engine.WaitingReason 枚举值一一对应，数值顺序不得擅自修改。
// 用于将 engine 内部等待原因枚举转换为 proto 枚举，而无需在 engine 主包引入 proto 依赖。
const (
	ReasonNone        = 0 // 无等待
	ReasonOpening     = 1 // 开局等待
	ReasonClaimWindow = 2 // 抢答窗口
	ReasonTsumo       = 3 // 自摸待决
	ReasonDiscard     = 4 // 等待弃牌
	ReasonSurrender   = 5 // 投降缩短
)

// ProgressData 是 engine.RoundProgress 的基础类型镜像，用于无循环依赖地传递进度数据。
type ProgressData struct {
	ActingSeat       int32
	ActingSeats      []int32
	WaitingAction    string
	Phase            int
	Step             int64
	PendingTile      string
	AvailableActions []string
	ClaimCandidates  []ClaimCandidateData
	WallRemaining    int32
	DeadlineUnixMs   int64
	Reason           int
	ServerNowUnixMs  int64
}

// ClaimCandidateData 是 engine.RoundClaimCandidate 的基础类型镜像。
type ClaimCandidateData struct {
	Seat    int32
	Actions []string
}

// ActionDetail 是 engine.LastActionInfo 的基础类型镜像。
type ActionDetail struct {
	Step        int64
	ActorSeat   int32
	Action      string
	Tile        string
	TargetSeat  int32
	SourceSeat  int32
	CreatedAtMs int64
}

// RuleMetaData 是 engine.RuleMeta 的基础类型镜像。
type RuleMetaData struct {
	RuleID          string
	DisplayName     string
	ShortDesc       string
	EnabledFeatures []string
	MaxHands        int32
}

// SeatScoreData 是 rules.SeatScore 的基础类型镜像。
type SeatScoreData struct {
	SeatIndex int32
	UserID    string
	TotalFan  int32
	Skipped   bool
}

// PenaltyData 是 rules.PenaltyItem 的基础类型镜像。
type PenaltyData struct {
	Reason   string
	FromSeat int32
	ToSeat   int32
	Amount   int32
}

// WinnerBreakdownData 是 rules.WinnerBreakdown 的基础类型镜像。
type WinnerBreakdownData struct {
	SeatIndex int32
	UserID    string
	Fan       int32
	FanNames  []string
}

// SettlementData 汇聚结算通知所需的全部字段。
type SettlementData struct {
	WinnerUserIDs       []string
	TotalFan            int32
	SeatScores          []SeatScoreData
	Penalties           []PenaltyData
	PerWinnerBreakdowns []WinnerBreakdownData
	DetailText          string
}

// SeatTilesItemData 是单座位牌列数据。
type SeatTilesItemData struct {
	Seat  int32
	Tiles []string
}

// SeatTilesData 是命名的多座位牌列数据。
type SeatTilesData struct {
	Key   string
	Seats []SeatTilesItemData
}

// SeatIntsData 是命名的多座位整数数据。
type SeatIntsData struct {
	Key    string
	Values []int32
}

// LocalTilesData 是目标座位专属的本地牌列数据。
type LocalTilesData struct {
	Key   string
	Tiles []string
}

// OpeningDoneData 汇聚开局结束通知所需的全部字段；LocalTiles 由调用方按目标座位筛选后传入。
type OpeningDoneData struct {
	Action     string
	StepID     string
	Kind       string
	Params     map[string]string
	SeatTiles  []SeatTilesData
	SeatInts   []SeatIntsData
	LocalTiles []LocalTilesData
}

// PhaseTokenData 是 engine.PhaseToken 的基础类型镜像，供适配层双向转换使用。
type PhaseTokenData struct {
	Step   int64
	Reason int
}

// PhaseTokenFromProto 将 proto PhaseToken 转换为 PhaseTokenData；nil 输入返回 nil。
func PhaseTokenFromProto(p *clientv1.PhaseToken) *PhaseTokenData {
	if p == nil {
		return nil
	}
	return &PhaseTokenData{Step: p.GetStep(), Reason: WaitingReasonFromProto(p.GetReason())}
}

// PhaseToProto 将 engine.Phase（以 int 传入）转换为 proto Phase 枚举。
func PhaseToProto(phase int) clientv1.Phase {
	switch phase {
	case PhaseOpening:
		return clientv1.Phase_PHASE_OPENING
	case PhaseClaim:
		return clientv1.Phase_PHASE_CLAIM
	case PhaseTsumo:
		return clientv1.Phase_PHASE_TSUMO
	case PhaseDiscard:
		return clientv1.Phase_PHASE_DISCARD
	case PhaseClosed:
		return clientv1.Phase_PHASE_CLOSED
	case PhaseDraw:
		return clientv1.Phase_PHASE_DRAW
	default:
		return clientv1.Phase_PHASE_UNSPECIFIED
	}
}

// WaitingReasonToProto 将 engine.WaitingReason（以 int 传入）转换为 proto WaitingReason 枚举。
func WaitingReasonToProto(reason int) clientv1.WaitingReason {
	switch reason {
	case ReasonOpening:
		return clientv1.WaitingReason_WAITING_REASON_OPENING
	case ReasonClaimWindow:
		return clientv1.WaitingReason_WAITING_REASON_CLAIM_WINDOW
	case ReasonTsumo:
		return clientv1.WaitingReason_WAITING_REASON_TSUMO
	case ReasonDiscard:
		return clientv1.WaitingReason_WAITING_REASON_DISCARD
	case ReasonSurrender:
		return clientv1.WaitingReason_WAITING_REASON_SURRENDER
	default:
		return clientv1.WaitingReason_WAITING_REASON_NONE
	}
}

// WaitingReasonFromProto 将 proto WaitingReason 枚举转换为 engine.WaitingReason 的 int 值。
func WaitingReasonFromProto(r clientv1.WaitingReason) int {
	switch r {
	case clientv1.WaitingReason_WAITING_REASON_OPENING:
		return ReasonOpening
	case clientv1.WaitingReason_WAITING_REASON_CLAIM_WINDOW:
		return ReasonClaimWindow
	case clientv1.WaitingReason_WAITING_REASON_TSUMO:
		return ReasonTsumo
	case clientv1.WaitingReason_WAITING_REASON_DISCARD:
		return ReasonDiscard
	case clientv1.WaitingReason_WAITING_REASON_SURRENDER:
		return ReasonSurrender
	default:
		return ReasonNone
	}
}

// PhaseUpdateFromProgress 将进度数据构造为 proto PhaseUpdate。
func PhaseUpdateFromProgress(p ProgressData) *clientv1.PhaseUpdate {
	return &clientv1.PhaseUpdate{
		Phase:            PhaseToProto(p.Phase),
		Step:             p.Step,
		Reason:           WaitingReasonToProto(p.Reason),
		DeadlineUnixMs:   p.DeadlineUnixMs,
		ServerNowUnixMs:  p.ServerNowUnixMs,
		ActingSeats:      append([]int32(nil), p.ActingSeats...),
		AvailableActions: append([]string(nil), p.AvailableActions...),
	}
}

// PhaseUpdateForDrift 构造阶段漂移错误响应中的 PhaseUpdate（仅含 step 和 reason）。
func PhaseUpdateForDrift(step int64, reason int) *clientv1.PhaseUpdate {
	return &clientv1.PhaseUpdate{
		Step:   step,
		Reason: WaitingReasonToProto(reason),
	}
}

// BuildInitialDeal 构造并序列化发牌通知；每个座位收到一条专属通知，牌面对其他座位不可见。
func BuildInitialDeal(reqID string, seatIndex int32, tiles []string, step int64) ([]byte, error) {
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: reqID,
		Body: &clientv1.Envelope_InitialDeal{
			InitialDeal: &clientv1.InitialDealNotify{
				SeatIndex: seatIndex,
				Tiles:     append([]string(nil), tiles...),
				Step:      step,
			},
		},
	})
}

// BuildStartGame 构造并序列化开局通知，嵌入阶段进度与规则元数据。
func BuildStartGame(reqID, roomID string, dealerSeat, roundIdx, handIdx int32, meta *RuleMetaData, progress ProgressData) ([]byte, error) {
	start := &clientv1.StartGameNotify{
		RoomId:     roomID,
		DealerSeat: dealerSeat,
		RoundIndex: roundIdx,
		HandIndex:  handIdx,
	}
	if meta != nil {
		start.RuleMeta = &clientv1.RuleMeta{
			RuleId:          meta.RuleID,
			DisplayName:     meta.DisplayName,
			ShortDesc:       meta.ShortDesc,
			EnabledFeatures: append([]string(nil), meta.EnabledFeatures...),
			MaxHands:        meta.MaxHands,
		}
	}
	applyProgressToStart(start, progress)
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: reqID,
		Body:  &clientv1.Envelope_StartGame{StartGame: start},
	})
}

// BuildDrawTile 构造并序列化 DrawTile 通知；tileText 为空时为隐藏牌面（其他玩家视角）。
func BuildDrawTile(reqID string, seatIndex int32, tileText string, progress ProgressData) ([]byte, error) {
	draw := &clientv1.DrawTileNotify{
		SeatIndex: seatIndex,
		Tile:      tileText,
	}
	applyProgressToDraw(draw, progress)
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: reqID,
		Body:  &clientv1.Envelope_DrawTile{DrawTile: draw},
	})
}

// BuildAction 构造并序列化动作通知，嵌入动作详情与当前阶段进度。
func BuildAction(reqID string, seatIndex int32, action, tile string, detail ActionDetail, progress ProgressData) ([]byte, error) {
	act := &clientv1.ActionNotify{
		SeatIndex: seatIndex,
		Action:    action,
		Tile:      tile,
		Detail:    toProtoActionDetail(detail),
	}
	applyProgressToAction(act, progress)
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: reqID,
		Body:  &clientv1.Envelope_Action{Action: act},
	})
}

// BuildHiddenAction 构造并序列化隐藏牌面的 Action 通知（暗杠专用）。
func BuildHiddenAction(reqID string, seatIndex int32, action string, detail ActionDetail, progress ProgressData) ([]byte, error) {
	d := toProtoActionDetail(detail)
	if d != nil {
		d.Tile = ""
	}
	act := &clientv1.ActionNotify{
		SeatIndex: seatIndex,
		Action:    action,
		Tile:      "",
		Detail:    d,
	}
	applyProgressToAction(act, progress)
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: reqID,
		Body:  &clientv1.Envelope_Action{Action: act},
	})
}

// BuildOpeningDone 构造并序列化 OpeningDone 通知。
// LocalTiles 由调用方按目标座位筛选后传入（广播时为 nil）。
func BuildOpeningDone(reqID string, done OpeningDoneData, progress ProgressData) ([]byte, error) {
	if done.Action == "" || done.Kind == "" {
		return nil, fmt.Errorf("不支持的开局投影")
	}
	payload := &clientv1.OpeningDoneNotify{
		Action:     done.Action,
		StepId:     done.StepID,
		Kind:       done.Kind,
		Params:     cloneStringMap(done.Params),
		SeatTiles:  toProtoSeatTiles(done.SeatTiles),
		SeatInts:   toProtoSeatInts(done.SeatInts),
		LocalTiles: toProtoLocalTiles(done.LocalTiles),
	}
	applyProgressToOpeningDone(payload, progress)
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: reqID,
		Body:  &clientv1.Envelope_OpeningDone{OpeningDone: payload},
	})
}

// BuildSettlement 构造并序列化结算通知，包含每座位得分、惩罚与胡牌番种明细。
func BuildSettlement(reqID, roomID string, data SettlementData) ([]byte, error) {
	return marshalEnvelope(&clientv1.Envelope{
		ReqId: reqID,
		Body: &clientv1.Envelope_Settlement{
			Settlement: &clientv1.SettlementNotify{
				RoomId:             roomID,
				WinnerUserIds:      append([]string(nil), data.WinnerUserIDs...),
				TotalFan:           data.TotalFan,
				SeatScores:         toProtoSeatScores(data.SeatScores),
				Penalties:          toProtoPenalties(data.Penalties),
				DetailText:         data.DetailText,
				PerWinnerBreakdown: toProtoWinnerBreakdowns(data.PerWinnerBreakdowns),
			},
		},
	})
}

func marshalEnvelope(env *clientv1.Envelope) ([]byte, error) {
	if env == nil {
		return nil, fmt.Errorf("nil envelope")
	}
	return proto.Marshal(env)
}

func toProtoActionDetail(d ActionDetail) *clientv1.ActionDetail {
	return &clientv1.ActionDetail{
		Step:        d.Step,
		ActorSeat:   d.ActorSeat,
		Action:      d.Action,
		Tile:        d.Tile,
		TargetSeat:  d.TargetSeat,
		SourceSeat:  d.SourceSeat,
		CreatedAtMs: d.CreatedAtMs,
	}
}

func applyProgressToAction(act *clientv1.ActionNotify, p ProgressData) {
	if act == nil {
		return
	}
	act.Phase = PhaseToProto(p.Phase)
	act.Step = p.Step
	act.ActingSeats = append([]int32(nil), p.ActingSeats...)
	act.WallRemaining = p.WallRemaining
	act.DeadlineUnixMs = p.DeadlineUnixMs
	act.PhaseUpdate = PhaseUpdateFromProgress(p)
}

func applyProgressToDraw(draw *clientv1.DrawTileNotify, p ProgressData) {
	if draw == nil {
		return
	}
	draw.Phase = PhaseToProto(p.Phase)
	draw.Step = p.Step
	draw.ActingSeats = append([]int32(nil), p.ActingSeats...)
	draw.WallRemaining = p.WallRemaining
	draw.DeadlineUnixMs = p.DeadlineUnixMs
	draw.PhaseUpdate = PhaseUpdateFromProgress(p)
}

func applyProgressToStart(start *clientv1.StartGameNotify, p ProgressData) {
	if start == nil {
		return
	}
	start.Phase = PhaseToProto(p.Phase)
	start.Step = p.Step
	start.ActingSeats = append([]int32(nil), p.ActingSeats...)
	start.WallRemaining = p.WallRemaining
	start.PhaseUpdate = PhaseUpdateFromProgress(p)
}

func applyProgressToOpeningDone(done *clientv1.OpeningDoneNotify, p ProgressData) {
	if done == nil {
		return
	}
	done.Phase = PhaseToProto(p.Phase)
	done.Step = p.Step
	done.ActingSeats = append([]int32(nil), p.ActingSeats...)
	done.PhaseUpdate = PhaseUpdateFromProgress(p)
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func toProtoSeatTiles(in []SeatTilesData) []*clientv1.OpeningSeatTiles {
	out := make([]*clientv1.OpeningSeatTiles, 0, len(in))
	for _, st := range in {
		seats := make([]*clientv1.SeatTiles, 0, len(st.Seats))
		for _, item := range st.Seats {
			seats = append(seats, &clientv1.SeatTiles{
				SeatIndex: item.Seat,
				Tiles:     append([]string(nil), item.Tiles...),
			})
		}
		out = append(out, &clientv1.OpeningSeatTiles{Key: st.Key, Seats: seats})
	}
	return out
}

func toProtoSeatInts(in []SeatIntsData) []*clientv1.OpeningSeatInts {
	out := make([]*clientv1.OpeningSeatInts, 0, len(in))
	for _, si := range in {
		out = append(out, &clientv1.OpeningSeatInts{Key: si.Key, Values: append([]int32(nil), si.Values...)})
	}
	return out
}

func toProtoLocalTiles(in []LocalTilesData) []*clientv1.OpeningLocalTiles {
	out := make([]*clientv1.OpeningLocalTiles, 0, len(in))
	for _, lt := range in {
		out = append(out, &clientv1.OpeningLocalTiles{Key: lt.Key, Tiles: append([]string(nil), lt.Tiles...)})
	}
	return out
}

func toProtoSeatScores(scores []SeatScoreData) []*clientv1.SeatScore {
	out := make([]*clientv1.SeatScore, 0, len(scores))
	for _, s := range scores {
		out = append(out, &clientv1.SeatScore{
			SeatIndex: s.SeatIndex,
			UserId:    s.UserID,
			TotalFan:  s.TotalFan,
			Skipped:   s.Skipped,
		})
	}
	return out
}

func toProtoPenalties(penalties []PenaltyData) []*clientv1.PenaltyItem {
	out := make([]*clientv1.PenaltyItem, 0, len(penalties))
	for _, p := range penalties {
		out = append(out, &clientv1.PenaltyItem{
			Reason:   p.Reason,
			FromSeat: p.FromSeat,
			ToSeat:   p.ToSeat,
			Amount:   p.Amount,
		})
	}
	return out
}

func toProtoWinnerBreakdowns(breakdowns []WinnerBreakdownData) []*clientv1.WinnerBreakdown {
	out := make([]*clientv1.WinnerBreakdown, 0, len(breakdowns))
	for _, b := range breakdowns {
		out = append(out, &clientv1.WinnerBreakdown{
			SeatIndex: b.SeatIndex,
			UserId:    b.UserID,
			Fan:       b.Fan,
			FanNames:  append([]string(nil), b.FanNames...),
		})
	}
	return out
}
