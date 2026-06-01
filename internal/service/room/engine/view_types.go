package engine

import clientv1 "racoo.cn/lsp/api/gen/go/client/v1"

// Phase 表示局内阶段，与传输层协议解耦的内部枚举。
type Phase int

const (
	PhaseUnspecified Phase = iota
	PhaseOpening
	PhaseClaim
	PhaseTsumo
	PhaseDiscard
	PhaseClosed
	PhaseDraw
)

// Proto 返回对应的 client.v1.Phase 枚举值，仅在序列化边界调用。
func (p Phase) Proto() clientv1.Phase {
	switch p {
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

// RuleMeta 是规则元数据的内部投影，与传输层协议解耦。
type RuleMeta struct {
	RuleID          string
	DisplayName     string
	ShortDesc       string
	EnabledFeatures []string
	MaxHands        int32
}

// LastActionInfo 是最近一次玩家动作的内部记录，与传输层协议解耦。
type LastActionInfo struct {
	Step        int64
	ActorSeat   int32
	Action      string
	Tile        string
	TargetSeat  int32
	SourceSeat  int32
	CreatedAtMs int64
}

// MeldInfo 是单条副露的内部投影。
type MeldInfo struct {
	SeatIndex       int32
	Kind            string
	Tiles           []string
	ClaimedFromSeat int32
	Concealed       bool
	Step            int64
}

// SeatMelds 是单座位所有副露的内部投影。
type SeatMelds struct {
	SeatIndex int32
	Melds     []*MeldInfo
}
