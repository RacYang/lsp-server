package room

import (
	"strings"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

func (rs *RoundState) wallRemaining() int32 {
	if rs == nil || rs.wall == nil {
		return 0
	}
	return int32(rs.wall.Remaining()) //nolint:gosec // 四川牌墙剩余张数小于 int32 上限
}

func (rs *RoundState) ruleMeta() *clientv1.RuleMeta {
	if rs == nil {
		return nil
	}
	meta := rules.CapabilitiesOf(rs.rule).Metadata
	if meta.DisplayName == "" && rs.rule != nil {
		meta.DisplayName = rs.rule.Name()
	}
	return &clientv1.RuleMeta{
		RuleId:          rs.ruleID,
		DisplayName:     meta.DisplayName,
		ShortDesc:       meta.ShortDesc,
		EnabledFeatures: append([]string(nil), meta.EnabledFeatures...),
		MaxHands:        meta.MaxHands,
	}
}

func (rs *RoundState) totalScores() []*clientv1.SeatScore {
	if rs == nil {
		return nil
	}
	balances := seatBalancesFromLedger(rs.ledger)
	out := make([]*clientv1.SeatScore, 0, 4)
	for seat := 0; seat < 4; seat++ {
		userID := ""
		if seat < len(rs.playerIDs) {
			userID = rs.playerIDs[seat]
		}
		out = append(out, &clientv1.SeatScore{
			SeatIndex: int32(seat), //nolint:gosec // 固定座位范围 0..3
			UserId:    userID,
			TotalFan:  balances[seat],
		})
	}
	return out
}

func (rs *RoundState) actionDetail(actor Seat, action string, t tile.Tile, target Seat, source Seat) *clientv1.ActionDetail {
	if rs == nil {
		return nil
	}
	tileText := ""
	if t != 0 {
		tileText = t.String()
	}
	return &clientv1.ActionDetail{
		Step:       int64(rs.step),
		ActorSeat:  actor.Proto(),
		Action:     action,
		Tile:       tileText,
		TargetSeat: target.Proto(),
		SourceSeat: source.Proto(),
	}
}

func (rs *RoundState) rememberLastAction(detail *clientv1.ActionDetail) {
	if rs == nil || detail == nil {
		return
	}
	rs.lastAction = &clientv1.LastActionInfo{
		Step:        detail.GetStep(),
		ActorSeat:   detail.GetActorSeat(),
		Action:      detail.GetAction(),
		Tile:        detail.GetTile(),
		TargetSeat:  detail.GetTargetSeat(),
		SourceSeat:  detail.GetSourceSeat(),
		CreatedAtMs: detail.GetCreatedAtMs(),
	}
}

func (rs *RoundState) meldInfosBySeat() []*clientv1.SeatMelds {
	out := make([]*clientv1.SeatMelds, 0, 4)
	if rs == nil {
		return out
	}
	for seat := 0; seat < 4; seat++ {
		seatMelds := &clientv1.SeatMelds{SeatIndex: int32(seat)} //nolint:gosec // 固定座位范围 0..3
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

func meldInfoFromEncoded(seat Seat, encoded string, step int) *clientv1.MeldInfo {
	kind, raw, ok := strings.Cut(encoded, ":")
	if !ok || kind == "" || raw == "" {
		return nil
	}
	var count int
	concealed := false
	switch kind {
	case "pong":
		count = 3
	case "gang":
		count = 4
	case "an_gang":
		count = 4
		concealed = true
	case "bu_gang":
		count = 4
	default:
		return nil
	}
	tiles := make([]string, 0, count)
	for i := 0; i < count; i++ {
		tiles = append(tiles, raw)
	}
	return &clientv1.MeldInfo{
		SeatIndex:       seat.Proto(),
		Kind:            kind,
		Tiles:           tiles,
		ClaimedFromSeat: SeatInvalid.Proto(),
		Concealed:       concealed,
		Step:            int64(step),
	}
}
