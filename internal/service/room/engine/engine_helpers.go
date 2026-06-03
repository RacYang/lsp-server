package engine

import (
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	codec "racoo.cn/lsp/internal/service/room/engine/codec"
)

func (rs *RoundState) wallRemaining() int32 {
	if rs == nil || rs.wall == nil {
		return 0
	}
	return int32(rs.wall.Remaining()) //nolint:gosec // 四川牌墙剩余张数小于 int32 上限
}

func (rs *RoundState) ruleMeta() *RuleMeta {
	if rs == nil {
		return nil
	}
	rs.ensureRuleRuntime()
	meta := rs.caps.Metadata
	if meta.DisplayName == "" && rs.rule != nil {
		meta.DisplayName = rs.rule.Name()
	}
	return &RuleMeta{
		RuleID:          rs.ruleID,
		DisplayName:     meta.DisplayName,
		ShortDesc:       meta.ShortDesc,
		EnabledFeatures: append([]string(nil), meta.EnabledFeatures...),
		MaxHands:        meta.MaxHands,
	}
}

// ToCodecData 将 RuleMeta 转换为 codec.RuleMetaData；供序列化边界使用。
func (m *RuleMeta) ToCodecData() *codec.RuleMetaData {
	if m == nil {
		return nil
	}
	return &codec.RuleMetaData{
		RuleID:          m.RuleID,
		DisplayName:     m.DisplayName,
		ShortDesc:       m.ShortDesc,
		EnabledFeatures: append([]string(nil), m.EnabledFeatures...),
		MaxHands:        m.MaxHands,
	}
}

func (rs *RoundState) totalScores() []*rules.SeatScore {
	if rs == nil {
		return nil
	}
	balances := seatBalancesFromScoreEvents(rs.scoreEvents)
	out := make([]*rules.SeatScore, 0, 4)
	for seat := 0; seat < 4; seat++ {
		userID := ""
		if seat < len(rs.playerIDs) {
			userID = rs.playerIDs[seat]
		}
		out = append(out, &rules.SeatScore{
			SeatIndex: int32(seat), //nolint:gosec // 固定座位范围 0..3
			UserID:    userID,
			TotalFan:  balances[seat],
		})
	}
	return out
}

// perSeatNotifications 将一个"按座位差异化"的通知展开为每座位一条。
// payloadForSeat 接收目标座位，返回该座位应收到的 Payload；
// 相邻座位若 Payload 相同，仍各发独立通知以维持统一的下游模型。
func perSeatNotifications(kind Kind, payloadForSeat func(Seat) []byte) []Notification {
	out := make([]Notification, 4)
	for seat := Seat(0); seat < 4; seat++ {
		out[seat] = Notification{Kind: kind, Payload: payloadForSeat(seat), TargetSeat: seat}
	}
	return out
}

func (rs *RoundState) roundProgress() RoundProgress {
	return ProjectRoundState(rs).Progress
}

func (rs *RoundState) drawTransitionProgress() RoundProgress {
	progress := rs.roundProgress()
	progress.Phase = PhaseDraw
	progress.WaitingAction = "none"
	if rs != nil {
		progress.ActingSeat = rs.turn.Proto()
		progress.ActingSeats = []int32{rs.turn.Proto()}
		progress.Step = int64(rs.step)
		progress.WallRemaining = rs.wallRemaining()
		progress.DeadlineUnixMs = rs.deadlineUnixMs
		progress.Reason = rs.phaseReason
	}
	progress.AvailableActions = nil
	progress.ClaimCandidates = nil
	progress.PendingTile = ""
	return progress
}

// makeCodecDetail 构造 codec.ActionDetail 并同步更新 lastAction；
// 合并了原 actionDetail() 与 rememberLastAction() 的职责。
func (rs *RoundState) makeCodecDetail(actor Seat, action string, t tile.Tile, target, source Seat) codec.ActionDetail {
	tileText := ""
	if t != 0 {
		tileText = t.String()
	}
	detail := codec.ActionDetail{
		Step:       int64(rs.step),
		ActorSeat:  int32(actor),
		Action:     action,
		Tile:       tileText,
		TargetSeat: int32(target),
		SourceSeat: int32(source),
	}
	rs.lastAction = &LastActionInfo{
		Step:       detail.Step,
		ActorSeat:  detail.ActorSeat,
		Action:     detail.Action,
		Tile:       detail.Tile,
		TargetSeat: detail.TargetSeat,
		SourceSeat: detail.SourceSeat,
	}
	return detail
}

func (rs *RoundState) meldInfosBySeat() []*SeatMelds {
	out := make([]*SeatMelds, 0, 4)
	if rs == nil {
		return out
	}
	for seat := 0; seat < 4; seat++ {
		seatMelds := &SeatMelds{SeatIndex: int32(seat)} //nolint:gosec // 固定座位范围 0..3
		if seat < len(rs.melds) {
			for _, encoded := range rs.melds[seat] {
				if info := meldInfoFromEncoded(Seat(seat), encoded, rs.step); info != nil {
					seatMelds.Melds = append(seatMelds.Melds, info)
				}
			}
		}
		out = append(out, seatMelds)
	}
	return out
}

func meldInfoFromEncoded(seat Seat, encoded string, step int) *MeldInfo {
	fact, ok := parseMeldFact(encoded)
	if !ok || fact.Kind == "" {
		return nil
	}
	var count int
	switch fact.Kind {
	case "pong":
		count = 3
	case "gang", "zhi_gang", "an_gang", "bu_gang":
		count = 4
	case "chi", "chow":
		count = 3
	default:
		return nil
	}
	tiles := make([]string, 0, count)
	if len(fact.Tiles) == 1 && count > 1 {
		for i := 0; i < count; i++ {
			tiles = append(tiles, fact.Tiles[0].String())
		}
	} else {
		for _, t := range fact.Tiles {
			tiles = append(tiles, t.String())
		}
	}
	return &MeldInfo{
		SeatIndex:       seat.Proto(),
		Kind:            fact.Kind,
		Tiles:           tiles,
		ClaimedFromSeat: fact.ClaimedFrom.Proto(),
		Concealed:       fact.Concealed,
		Step:            int64(step),
	}
}
