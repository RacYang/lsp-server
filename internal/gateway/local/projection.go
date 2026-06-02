package local

import (
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/rules"
	lobbysvc "racoo.cn/lsp/internal/service/lobby"
	reconnectsvc "racoo.cn/lsp/internal/service/reconnect"
	eng "racoo.cn/lsp/internal/service/room/engine"
)

// buildSnapshotFromReconnect 将重连服务层结果投影为 SnapshotNotify proto。
func buildSnapshotFromReconnect(r *reconnectsvc.ReconnectResult) *clientv1.SnapshotNotify {
	view := r.View
	mySeat := seatIndexForUser(r.Players, r.UserID)
	snap := &clientv1.SnapshotNotify{
		RoomId:           r.RoomID,
		PlayerIds:        append([]string(nil), r.Players...),
		Seats:            clientSeatsFromPlayerIDs(r.Players),
		QueSuitBySeat:    append([]int32(nil), view.QueBySeat...),
		Cursor:           r.LastCursor,
		State:            r.State,
		ActingSeat:       view.ActingSeat,
		ActingSeats:      append([]int32(nil), view.ActingSeats...),
		WaitingAction:    view.WaitingAction,
		Phase:            view.Phase.Proto(),
		LastStep:         view.LastStep,
		PendingTile:      view.PendingTile,
		AvailableActions: append([]string(nil), view.AvailableActions...),
		ClaimCandidates:  roomClaimCandidatesToClient(view.ClaimCandidates),
		YourHandTiles:    handForSeat(view.HandsBySeat, mySeat),
		DiscardsBySeat:   stringMatrixToClientSeatTiles(view.DiscardsBySeat),
		MeldsBySeat:      stringMatrixToClientSeatTiles(view.MeldsBySeat),
		MeldInfosBySeat:  roomViewMeldInfosBySeat(view.MeldInfosBySeat),
		LastAction:       roomViewLastAction(view.LastAction),
		WallRemaining:    view.WallRemaining,
		DeadlineUnixMs:   view.DeadlineUnixMs,
		RoundIndex:       view.RoundIndex,
		HandIndex:        view.HandIndex,
		TotalScores:      roomViewTotalScores(view.TotalScores),
		RuleMeta:         roomViewRuleMeta(view.RuleMeta),
	}
	for seat := 0; seat < len(snap.Seats) && seat < len(view.HandsBySeat); seat++ {
		snap.Seats[seat].HandCount = int32(len(view.HandsBySeat[seat])) //nolint:gosec // 座位手牌数量小于 20
	}
	return snap
}

// clientSeatsFromPlayerIDs 将玩家 ID 列表投影为客户端座位信息数组（固定 4 个座位）。
func clientSeatsFromPlayerIDs(players []string) []*clientv1.SeatInfo {
	seats := make([]*clientv1.SeatInfo, 0, 4)
	for i := 0; i < 4; i++ {
		info := &clientv1.SeatInfo{SeatIndex: int32(i), Status: "empty"} //nolint:gosec // 固定座位范围 0..3
		if i < len(players) {
			info.UserId = players[i]
			if players[i] != "" {
				info.Online = true
				info.Status = "online"
			}
		}
		seats = append(seats, info)
	}
	return seats
}

// seatIndexForUser 返回用户在玩家列表中的座位下标；未找到时返回 -1。
func seatIndexForUser(players []string, userID string) int {
	for seat, current := range players {
		if current == userID {
			return seat
		}
	}
	return -1
}

// handForSeat 返回指定座位手牌的副本；座位越界时返回 nil。
func handForSeat(hands [][]string, seat int) []string {
	if seat < 0 || seat >= len(hands) {
		return nil
	}
	return append([]string(nil), hands[seat]...)
}

// stringMatrixToClientSeatTiles 将二维字符串数组按座位组织为客户端牌列表。
func stringMatrixToClientSeatTiles(items [][]string) []*clientv1.SeatTiles {
	out := make([]*clientv1.SeatTiles, 0, 4)
	for seat := 0; seat < 4; seat++ {
		var tiles []string
		if seat < len(items) {
			tiles = append([]string(nil), items[seat]...)
		}
		out = append(out, &clientv1.SeatTiles{
			SeatIndex: int32(seat), //nolint:gosec // 座位范围固定
			Tiles:     tiles,
		})
	}
	return out
}

// roomClaimCandidatesToClient 将抢答候选列表投影为客户端 proto。
func roomClaimCandidatesToClient(candidates []eng.RoundClaimCandidate) []*clientv1.ClaimCandidate {
	out := make([]*clientv1.ClaimCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, &clientv1.ClaimCandidate{
			SeatIndex: candidate.Seat,
			Actions:   append([]string(nil), candidate.Actions...),
		})
	}
	return out
}

// lobbyRoomMetasToClient 将大厅房间摘要列表投影为客户端 proto。
func lobbyRoomMetasToClient(rooms []lobbysvc.RoomMeta) []*clientv1.RoomMeta {
	out := make([]*clientv1.RoomMeta, 0, len(rooms))
	for _, room := range rooms {
		out = append(out, &clientv1.RoomMeta{
			RoomId:      room.RoomID,
			RuleId:      room.RuleID,
			DisplayName: room.DisplayName,
			SeatCount:   room.SeatCount,
			MaxSeats:    room.MaxSeats,
			CreatedAtMs: room.CreatedAtMs,
			Stage:       room.Stage,
			RuleMeta:    lobbyRuleMetaToClient(room.RuleMeta),
		})
	}
	return out
}

// lobbyRuleMetasToClient 将规则摘要列表投影为客户端 proto。
func lobbyRuleMetasToClient(rules []lobbysvc.RuleMeta) []*clientv1.RuleMeta {
	out := make([]*clientv1.RuleMeta, 0, len(rules))
	for _, rule := range rules {
		out = append(out, lobbyRuleMetaToClient(rule))
	}
	return out
}

// roomViewRuleMeta 将局面规则摘要投影为客户端 proto；nil 时返回 nil。
func roomViewRuleMeta(m *eng.RuleMeta) *clientv1.RuleMeta {
	if m == nil {
		return nil
	}
	return &clientv1.RuleMeta{
		RuleId:          m.RuleID,
		DisplayName:     m.DisplayName,
		ShortDesc:       m.ShortDesc,
		EnabledFeatures: append([]string(nil), m.EnabledFeatures...),
		MaxHands:        m.MaxHands,
	}
}

// roomViewLastAction 将最后操作信息投影为客户端 proto；nil 时返回 nil。
func roomViewLastAction(a *eng.LastActionInfo) *clientv1.LastActionInfo {
	if a == nil {
		return nil
	}
	return &clientv1.LastActionInfo{
		Step:        a.Step,
		ActorSeat:   a.ActorSeat,
		Action:      a.Action,
		Tile:        a.Tile,
		TargetSeat:  a.TargetSeat,
		SourceSeat:  a.SourceSeat,
		CreatedAtMs: a.CreatedAtMs,
	}
}

// roomViewMeldInfosBySeat 将按座位组织的面子信息投影为客户端 proto。
func roomViewMeldInfosBySeat(seats []*eng.SeatMelds) []*clientv1.SeatMelds {
	out := make([]*clientv1.SeatMelds, 0, len(seats))
	for _, s := range seats {
		pm := &clientv1.SeatMelds{SeatIndex: s.SeatIndex}
		for _, m := range s.Melds {
			pm.Melds = append(pm.Melds, &clientv1.MeldInfo{
				SeatIndex:       m.SeatIndex,
				Kind:            m.Kind,
				Tiles:           append([]string(nil), m.Tiles...),
				ClaimedFromSeat: m.ClaimedFromSeat,
				Concealed:       m.Concealed,
				Step:            m.Step,
			})
		}
		out = append(out, pm)
	}
	return out
}

// roomViewTotalScores 将总分列表投影为客户端 proto。
func roomViewTotalScores(scores []*rules.SeatScore) []*clientv1.SeatScore {
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

// lobbyRuleMetaToClient 将单条规则摘要投影为客户端 proto；空摘要返回 nil。
func lobbyRuleMetaToClient(meta lobbysvc.RuleMeta) *clientv1.RuleMeta {
	if meta.RuleID == "" && meta.DisplayName == "" {
		return nil
	}
	return &clientv1.RuleMeta{
		RuleId:          meta.RuleID,
		DisplayName:     meta.DisplayName,
		ShortDesc:       meta.ShortDesc,
		EnabledFeatures: append([]string(nil), meta.EnabledFeatures...),
		MaxHands:        meta.MaxHands,
	}
}
