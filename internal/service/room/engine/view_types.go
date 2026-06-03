package engine

// Phase 表示局内阶段，与传输层协议解耦的内部枚举。
// proto 转换由 codec.PhaseToProto 完成，不在此处定义。
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
