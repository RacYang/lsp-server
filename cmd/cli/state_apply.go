package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func (s *AppState) Apply(env *clientv1.Envelope) {
	if env == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	applyEnvelopeLocked(&s.view, env)
	s.view.UpdatedAt = time.Now()
}

func applyEnvelopeLocked(v *RoomView, env *clientv1.Envelope) {
	if shouldDropPendingLeaveFrame(v, env) {
		appendLog(v, "已忽略离房后的陈旧牌桌事件")
		return
	}
	if shouldDropStaleStep(v, env) {
		appendLog(v, "已忽略快照之前的陈旧局内事件")
		return
	}
	switch body := env.GetBody().(type) {
	case *clientv1.Envelope_LoginResp:
		applyLogin(v, body.LoginResp)
	case *clientv1.Envelope_JoinRoomResp:
		v.SeatIndex = body.JoinRoomResp.GetSeatIndex()
		v.RuleID = body.JoinRoomResp.GetRuleId()
		v.DisplayName = body.JoinRoomResp.GetDisplayName()
		clearRoundFacts(v)
		v.LastSettlement = nil
		applySeatInfos(v, body.JoinRoomResp.GetSeats())
		if v.SeatIndex >= 0 && v.SeatIndex < 4 {
			v.Players[v.SeatIndex].Nickname = v.Nickname
			v.Players[v.SeatIndex].UserID = v.UserID
		}
		if body.JoinRoomResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			v.Phase = phaseTable
		}
		appendLog(v, fmt.Sprintf("已入座 %d", v.SeatIndex))
	case *clientv1.Envelope_ListRoomsResp:
		applyListRooms(v, body.ListRoomsResp)
	case *clientv1.Envelope_AutoMatchResp:
		applyAutoMatch(v, body.AutoMatchResp)
	case *clientv1.Envelope_CreateRoomResp:
		applyCreateRoom(v, body.CreateRoomResp)
	case *clientv1.Envelope_ReadyResp:
		appendResponseLog(v, "准备", body.ReadyResp.GetErrorCode(), body.ReadyResp.GetErrorMessage())
	case *clientv1.Envelope_InitialDeal:
		deal := body.InitialDeal
		v.SeatIndex = deal.GetSeatIndex()
		player := &v.Players[v.SeatIndex]
		player.Hand = sortedTiles(deal.GetTiles())
		player.HandCnt = len(player.Hand)
		appendLog(v, fmt.Sprintf("收到开局手牌 %d 张", len(player.Hand)))
	case *clientv1.Envelope_StartGame:
		v.RoomID = body.StartGame.GetRoomId()
		v.DealerSeat = body.StartGame.GetDealerSeat()
		v.WallRemaining = body.StartGame.GetWallRemaining()
		v.RoundIndex = body.StartGame.GetRoundIndex()
		v.HandIndex = body.StartGame.GetHandIndex()
		if meta := body.StartGame.GetRuleMeta(); meta != nil {
			v.RuleID = meta.GetRuleId()
			v.DisplayName = meta.GetDisplayName()
		}
		v.RoomState = "playing"
		applyPhase(v, body.StartGame.GetPhase(), body.StartGame.GetActingSeats(), body.StartGame.GetStep())
		v.LastSettlement = nil
		appendLog(v, fmt.Sprintf("开局，庄家 %d", v.DealerSeat))
	case *clientv1.Envelope_DrawTile:
		applyDraw(v, body.DrawTile)
	case *clientv1.Envelope_DiscardResp:
		appendResponseLog(v, "出牌", body.DiscardResp.GetErrorCode(), body.DiscardResp.GetErrorMessage())
	case *clientv1.Envelope_Action:
		applyAction(v, body.Action)
	case *clientv1.Envelope_Settlement:
		v.LastSettlement = body.Settlement
		v.RoundIndex = body.Settlement.GetRoundIndex()
		v.HandIndex = body.Settlement.GetHandIndex()
		v.TotalScores = cloneSeatScores(body.Settlement.GetTotalScores())
		v.RoomState = "settling"
		setWaitingAction(v, "none", nil)
		v.ClaimCandidates = map[int32][]string{}
		appendLog(v, "收到结算")
	case *clientv1.Envelope_HeartbeatResp:
		// RTT 由连接层根据本地发送时间更新，这里只记录可见事件。
	case *clientv1.Envelope_LeaveRoomResp:
		appendResponseLog(v, "离房", body.LeaveRoomResp.GetErrorCode(), body.LeaveRoomResp.GetErrorMessage())
		if body.LeaveRoomResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			resetRoomToLobby(v, true)
			v.PendingLeaveRoomID = ""
		}
	case *clientv1.Envelope_RouteRedirect:
		v.Reconnecting = true
		v.LastError = "服务端要求切换网关"
		appendLog(v, "收到路由重定向: "+body.RouteRedirect.GetWsUrl())
	case *clientv1.Envelope_ExchangeThreeResp:
		appendResponseLog(v, "换三张", body.ExchangeThreeResp.GetErrorCode(), body.ExchangeThreeResp.GetErrorMessage())
	case *clientv1.Envelope_ExchangeThreeDone:
		applyExchangeDone(v, body.ExchangeThreeDone)
	case *clientv1.Envelope_QueMenResp:
		appendResponseLog(v, "定缺", body.QueMenResp.GetErrorCode(), body.QueMenResp.GetErrorMessage())
	case *clientv1.Envelope_QueMenDone:
		for seat, suit := range body.QueMenDone.GetQueSuitBySeat() {
			if seat < len(v.QueBySeat) {
				v.QueBySeat[seat] = suit
			}
		}
		applyPhase(v, body.QueMenDone.GetPhase(), body.QueMenDone.GetActingSeats(), body.QueMenDone.GetStep())
		if body.QueMenDone.GetPhase() == clientv1.Phase_PHASE_UNSPECIFIED {
			setWaitingAction(v, "discard", []string{"discard"})
		}
		appendLog(v, "定缺完成")
	case *clientv1.Envelope_Snapshot:
		applySnapshot(v, body.Snapshot)
	case *clientv1.Envelope_PongResp:
		appendResponseLog(v, "碰", body.PongResp.GetErrorCode(), body.PongResp.GetErrorMessage())
	case *clientv1.Envelope_GangResp:
		appendResponseLog(v, "杠", body.GangResp.GetErrorCode(), body.GangResp.GetErrorMessage())
	case *clientv1.Envelope_HuResp:
		appendResponseLog(v, "胡", body.HuResp.GetErrorCode(), body.HuResp.GetErrorMessage())
	case *clientv1.Envelope_PassResp:
		appendResponseLog(v, "过", body.PassResp.GetErrorCode(), body.PassResp.GetErrorMessage())
		if body.PassResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			if v.WaitingAction == "tsumo_window" {
				setWaitingAction(v, "discard", []string{"discard"})
				v.PendingTile = ""
			} else {
				v.AvailableActions = nil
			}
			v.ClaimCandidates = map[int32][]string{}
		}
	case *clientv1.Envelope_AddBotResp:
		appendResponseLog(v, "添加机器人", body.AddBotResp.GetErrorCode(), body.AddBotResp.GetErrorMessage())
		if body.AddBotResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			applySeatInfos(v, body.AddBotResp.GetAdded())
		}
	}
}

func applyLogin(v *RoomView, resp *clientv1.LoginResponse) {
	if resp.GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		if resp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_ROUTE_REDIRECT {
			v.Reconnecting = true
			v.LastError = "服务端要求切换网关"
			appendLog(v, "服务端要求切换网关: "+resp.GetErrorMessage())
			return
		}
		v.LastError = resp.GetErrorMessage()
		appendLog(v, "登录失败: "+resp.GetErrorMessage())
		return
	}
	v.UserID = resp.GetUserId()
	v.SessionToken = resp.GetSessionToken()
	v.Phase = phaseLobby
	v.SuppressAutoResume = resp.GetResumed()
	if resp.GetResumed() {
		v.RoomID = ""
		v.DisplayName = ""
		v.SeatIndex = -1
		clearRoundFacts(v)
	}
	if !resp.GetResumed() {
		v.RoomID = ""
		v.ResumeRoomID = ""
		v.DisplayName = ""
		v.SeatIndex = -1
		v.RoomState = ""
		v.LastSettlement = nil
		clearRoundFacts(v)
	}
	appendLog(v, "登录成功: "+v.UserID)
}

func applyListRooms(v *RoomView, resp *clientv1.ListRoomsResponse) {
	if resp.GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		v.LastError = resp.GetErrorMessage()
		appendLog(v, "刷新房间列表失败: "+resp.GetErrorMessage())
		return
	}
	v.RoomList = cloneRoomMetas(resp.GetRooms())
	v.NextRoomPage = resp.GetNextPageToken()
	v.Phase = phaseLobby
	appendLog(v, fmt.Sprintf("刷新房间列表: %d 间", len(v.RoomList)))
}

func applyAutoMatch(v *RoomView, resp *clientv1.AutoMatchResponse) {
	if resp.GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		v.LastError = resp.GetErrorMessage()
		appendLog(v, "自动匹配失败: "+resp.GetErrorMessage())
		return
	}
	applyLobbySeat(v, resp.GetRoomId(), resp.GetSeatIndex(), resp.GetRuleId(), resp.GetDisplayName())
	applySeatInfos(v, resp.GetSeats())
	appendLog(v, fmt.Sprintf("自动匹配成功: %s 座位 %d", resp.GetRoomId(), resp.GetSeatIndex()))
}

func applyCreateRoom(v *RoomView, resp *clientv1.CreateRoomResponse) {
	if resp.GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		v.LastError = resp.GetErrorMessage()
		appendLog(v, "创建房间失败: "+resp.GetErrorMessage())
		return
	}
	applyLobbySeat(v, resp.GetRoomId(), resp.GetSeatIndex(), resp.GetRuleId(), resp.GetDisplayName())
	applySeatInfos(v, resp.GetSeats())
	appendLog(v, fmt.Sprintf("创建房间成功: %s 座位 %d", resp.GetRoomId(), resp.GetSeatIndex()))
}

func applyLobbySeat(v *RoomView, roomID string, seat int32, ruleID, displayName string) {
	v.RoomID = roomID
	v.PendingLeaveRoomID = ""
	v.SuppressAutoResume = false
	v.ResumeRoomID = ""
	v.RuleID = ruleID
	v.DisplayName = displayName
	v.SeatIndex = seat
	v.Phase = phaseTable
	clearRoundFacts(v)
	v.LastSettlement = nil
	if seat >= 0 && seat < 4 {
		v.Players[seat].Nickname = v.Nickname
		v.Players[seat].UserID = v.UserID
	}
}

func applyDraw(v *RoomView, draw *clientv1.DrawTileNotify) {
	seat := draw.GetSeatIndex()
	if seat < 0 || seat > 3 {
		return
	}
	t := draw.GetTile()
	v.ActingSeat = seat
	v.PendingTile = t
	v.WallRemaining = draw.GetWallRemaining()
	v.DeadlineUnixMS = draw.GetDeadlineUnixMs()
	applyPhase(v, draw.GetPhase(), draw.GetActingSeats(), draw.GetStep())
	if draw.GetPhase() == clientv1.Phase_PHASE_UNSPECIFIED {
		setWaitingAction(v, "discard", []string{"discard"})
	}
	p := &v.Players[seat]
	if seat == v.SeatIndex {
		if t != "" && (v.SnapshotStep <= 0 || len(p.Hand) < 14) {
			p.Hand = sortedTiles(append(p.Hand, t))
		}
		p.HandCnt = len(p.Hand)
	} else {
		// 对家摸牌只维护计数；服务端按座位投影后不会下发实际牌面。
		p.HandCnt++
	}
	if seat == v.SeatIndex && t != "" {
		appendLog(v, fmt.Sprintf("%s 摸 %s", nicknameForSeat(*v, seat), TileName(t)))
	} else {
		appendLog(v, fmt.Sprintf("%s 摸牌", nicknameForSeat(*v, seat)))
	}
}

func applyAction(v *RoomView, action *clientv1.ActionNotify) {
	seat := action.GetSeatIndex()
	v.WallRemaining = action.GetWallRemaining()
	v.DeadlineUnixMS = action.GetDeadlineUnixMs()
	if detail := action.GetDetail(); detail != nil {
		v.LastAction = &clientv1.LastActionInfo{
			Step:        detail.GetStep(),
			ActorSeat:   detail.GetActorSeat(),
			Action:      detail.GetAction(),
			Tile:        detail.GetTile(),
			TargetSeat:  detail.GetTargetSeat(),
			SourceSeat:  detail.GetSourceSeat(),
			CreatedAtMs: detail.GetCreatedAtMs(),
		}
	}
	applyPhase(v, action.GetPhase(), action.GetActingSeats(), action.GetStep())
	switch action.GetAction() {
	case "exchange_three":
		if action.GetPhase() == clientv1.Phase_PHASE_UNSPECIFIED {
			setWaitingAction(v, "exchange_three", []string{"exchange_three"})
		}
	case "que_men":
		if action.GetPhase() == clientv1.Phase_PHASE_UNSPECIFIED {
			setWaitingAction(v, "que_men", []string{"que_men"})
		}
	case "discard":
		applyDiscardAction(v, seat, action.GetTile())
	case "pong":
		applyPongAction(v, seat, action.GetTile())
	case "gang":
		recordMeld(v, seat, "gang:"+action.GetTile())
		if seat >= 0 && seat < 4 {
			v.ActingSeat = seat
		}
	case "hu", "tsumo":
		if seat >= 0 && seat < 4 {
			v.Players[seat].Hued = true
		}
	case "hu_choice", "qiang_gang_choice", "pong_choice", "gang_choice":
		if action.GetPhase() == clientv1.Phase_PHASE_UNSPECIFIED {
			setWaitingAction(v, "claim_window", nil)
		}
		v.PendingTile = action.GetTile()
		if seat >= 0 && seat < 4 {
			v.ActingSeat = seat
		}
		if seat == v.SeatIndex {
			v.AvailableActions = actionsForChoice(action.GetAction())
			v.ClaimCandidates = map[int32][]string{seat: append([]string(nil), v.AvailableActions...)}
		} else {
			v.AvailableActions = nil
		}
	case "tsumo_choice":
		if action.GetPhase() == clientv1.Phase_PHASE_UNSPECIFIED {
			setWaitingAction(v, "tsumo_window", nil)
		}
		v.PendingTile = action.GetTile()
		if seat >= 0 && seat < 4 {
			v.ActingSeat = seat
		}
		if seat == v.SeatIndex {
			v.AvailableActions = []string{"hu", "pass"}
		} else {
			v.AvailableActions = nil
		}
	}
	if msg := formatActionLog(*v, seat, action.GetAction(), action.GetTile()); msg != "" {
		appendLog(v, msg)
	}
}

// formatActionLog 把协议侧的 action 字符串翻译成对玩家可读的中文描述。
//
// 例如 "1 discard p5" → "1号位 出 五筒"；"2 pong p5" → "2号位 碰 五筒"。
// 选择窗口类（hu_choice / pong_choice 等）只暴露给抢答座位用，统一为 "等待应答"，
// 避免历史日志被一堆 *_choice 内部噪声淹没。
func formatActionLog(view RoomView, seat int32, action, tile string) string {
	name := nicknameForSeat(view, seat)
	switch action {
	case "discard":
		if tile != "" {
			return fmt.Sprintf("%s 出 %s", name, TileName(tile))
		}
		return name + " 出牌"
	case "pong":
		return fmt.Sprintf("%s 碰 %s", name, TileName(tile))
	case "gang":
		return fmt.Sprintf("%s 杠 %s", name, TileName(tile))
	case "hu", "tsumo":
		if tile != "" {
			return fmt.Sprintf("%s 胡 %s", name, TileName(tile))
		}
		return name + " 胡牌"
	case "exchange_three":
		return name + " 换三张"
	case "que_men":
		return name + " 定缺"
	}
	return ""
}

func applyDiscardAction(v *RoomView, seat int32, tile string) {
	if seat < 0 || seat > 3 || tile == "" {
		return
	}
	p := &v.Players[seat]
	p.Discards = append(p.Discards, tile)
	if seat == v.SeatIndex {
		p.Hand = removeOneTile(p.Hand, tile)
		p.HandCnt = len(p.Hand)
	} else if p.HandCnt > 0 {
		// 对家弃牌时手牌计数 -1；保持与 applyDraw 的 +1 对称，UI 上的 "▢×N" 才不会越涨越高。
		p.HandCnt--
	}
	v.ActingSeat = -1
	setWaitingAction(v, "none", nil)
	v.PendingTile = ""
	v.ClaimCandidates = map[int32][]string{}
}

func applyPongAction(v *RoomView, seat int32, tile string) {
	if seat < 0 || seat > 3 || tile == "" {
		return
	}
	removeClaimedDiscard(v, seat, tile)
	recordMeld(v, seat, "pong:"+tile)
	p := &v.Players[seat]
	if seat == v.SeatIndex {
		p.Hand = removeOneTile(removeOneTile(p.Hand, tile), tile)
		p.HandCnt = len(p.Hand)
	} else {
		p.HandCnt -= 2
		if p.HandCnt < 0 {
			p.HandCnt = 0
		}
	}
	v.ActingSeat = seat
	setWaitingAction(v, "discard", []string{"discard"})
	v.PendingTile = ""
	v.ClaimCandidates = map[int32][]string{}
}

func removeClaimedDiscard(v *RoomView, claimer int32, tile string) {
	for seat := int32(0); seat < 4; seat++ {
		if seat == claimer {
			continue
		}
		discards := v.Players[seat].Discards
		if len(discards) == 0 || discards[len(discards)-1] != tile {
			continue
		}
		v.Players[seat].Discards = append([]string(nil), discards[:len(discards)-1]...)
		return
	}
}

func applyExchangeDone(v *RoomView, done *clientv1.ExchangeThreeDoneNotify) {
	applyPhase(v, done.GetPhase(), done.GetActingSeats(), done.GetStep())
	if done.GetDirection() > 0 {
		v.ExchangeDirection = done.GetDirection()
	}
	for _, item := range done.GetPerSeat() {
		if item.GetSeatIndex() == v.SeatIndex {
			p := &v.Players[v.SeatIndex]
			p.Hand = sortedTiles(append(p.Hand, item.GetTiles()...))
			p.HandCnt = len(p.Hand)
			break
		}
	}
	if done.GetPhase() == clientv1.Phase_PHASE_UNSPECIFIED {
		setWaitingAction(v, "que_men", []string{"que_men"})
	}
	appendLog(v, "换三张完成")
}

func setWaitingAction(v *RoomView, action string, available []string) {
	if action == "" {
		action = "none"
	}
	if v.WaitingAction != action {
		v.ActionStartedAt = time.Now()
	}
	v.WaitingAction = action
	v.AvailableActions = append([]string(nil), available...)
}

func applyPhase(v *RoomView, phase clientv1.Phase, actingSeats []int32, step int64) {
	if phase == clientv1.Phase_PHASE_UNSPECIFIED {
		if step > v.LastStep {
			v.LastStep = step
		}
		return
	}
	if step > v.LastStep {
		v.LastStep = step
	}
	v.RoundPhase = phase
	v.ActingSeats = append([]int32(nil), actingSeats...)
	if len(actingSeats) > 0 {
		v.ActingSeat = actingSeats[0]
	}
	switch phase {
	case clientv1.Phase_PHASE_EXCHANGE:
		setWaitingAction(v, "exchange_three", []string{"exchange_three"})
	case clientv1.Phase_PHASE_QUE_MEN:
		setWaitingAction(v, "que_men", []string{"que_men"})
	case clientv1.Phase_PHASE_DISCARD:
		setWaitingAction(v, "discard", []string{"discard"})
	case clientv1.Phase_PHASE_CLAIM:
		setWaitingAction(v, "claim_window", nil)
	case clientv1.Phase_PHASE_TSUMO:
		setWaitingAction(v, "tsumo_window", nil)
	case clientv1.Phase_PHASE_SETTLE:
		setWaitingAction(v, "none", nil)
	default:
		setWaitingAction(v, "none", nil)
	}
}

func shouldDropStaleStep(v *RoomView, env *clientv1.Envelope) bool {
	step := envelopeStep(env)
	return step > 0 && v.SnapshotStep > 0 && step <= v.SnapshotStep
}

func envelopeStep(env *clientv1.Envelope) int64 {
	if env == nil {
		return 0
	}
	switch body := env.GetBody().(type) {
	case *clientv1.Envelope_InitialDeal:
		return body.InitialDeal.GetStep()
	case *clientv1.Envelope_DrawTile:
		return body.DrawTile.GetStep()
	case *clientv1.Envelope_Action:
		return body.Action.GetStep()
	case *clientv1.Envelope_ExchangeThreeDone:
		return body.ExchangeThreeDone.GetStep()
	case *clientv1.Envelope_QueMenDone:
		return body.QueMenDone.GetStep()
	case *clientv1.Envelope_StartGame:
		return body.StartGame.GetStep()
	default:
		return 0
	}
}

func applySnapshot(v *RoomView, snap *clientv1.SnapshotNotify) {
	if v.SuppressAutoResume && v.Phase == phaseLobby {
		v.ResumeRoomID = snap.GetRoomId()
		appendLog(v, "检测到可恢复房间，已停留在大厅")
		return
	}
	v.RoomID = snap.GetRoomId()
	if v.RoomID != "" {
		v.Phase = phaseTable
	}
	v.RoomState = snap.GetState()
	setWaitingAction(v, snap.GetWaitingAction(), append([]string(nil), snap.GetAvailableActions()...))
	v.ActingSeat = snap.GetActingSeat()
	v.ActingSeats = append([]int32(nil), snap.GetActingSeats()...)
	v.RoundPhase = snap.GetPhase()
	if snap.GetLastStep() > v.LastStep {
		v.LastStep = snap.GetLastStep()
	}
	if snap.GetLastStep() > v.SnapshotStep {
		v.SnapshotStep = snap.GetLastStep()
	}
	v.PendingTile = snap.GetPendingTile()
	v.LastAction = snap.GetLastAction()
	v.WallRemaining = snap.GetWallRemaining()
	v.DeadlineUnixMS = snap.GetDeadlineUnixMs()
	v.RoundIndex = snap.GetRoundIndex()
	v.HandIndex = snap.GetHandIndex()
	v.TotalScores = cloneSeatScores(snap.GetTotalScores())
	if meta := snap.GetRuleMeta(); meta != nil {
		v.RuleID = meta.GetRuleId()
		v.DisplayName = meta.GetDisplayName()
	}
	v.ClaimCandidates = make(map[int32][]string, len(snap.GetClaimCandidates()))
	for _, candidate := range snap.GetClaimCandidates() {
		v.ClaimCandidates[candidate.GetSeatIndex()] = append([]string(nil), candidate.GetActions()...)
	}
	for seat, userID := range snap.GetPlayerIds() { //nolint:staticcheck // 兼容旧服务端的 deprecated player_ids 兜底。
		if seat < len(v.Players) {
			v.Players[seat].UserID = userID
		}
	}
	applySeatInfos(v, snap.GetSeats())
	for seat, suit := range snap.GetQueSuitBySeat() {
		if seat < len(v.QueBySeat) {
			v.QueBySeat[seat] = suit
		}
	}
	if v.SeatIndex >= 0 && v.SeatIndex < 4 {
		p := &v.Players[v.SeatIndex]
		p.Hand = sortedTiles(snap.GetYourHandTiles())
		p.HandCnt = len(p.Hand)
	}
	applySeatTiles(v, snap.GetDiscardsBySeat(), func(p *PlayerView, tiles []string) { p.Discards = tiles })
	applySeatTiles(v, snap.GetMeldsBySeat(), func(p *PlayerView, tiles []string) { p.Melds = tiles })
	applyMeldInfos(v, snap.GetMeldInfosBySeat())
	appendLog(v, "快照已恢复")
}

func applySeatInfos(v *RoomView, seats []*clientv1.SeatInfo) {
	if len(seats) == 0 {
		return
	}
	for _, seat := range seats {
		idx := seat.GetSeatIndex()
		if idx < 0 || idx > 3 {
			continue
		}
		p := &v.Players[idx]
		p.UserID = seat.GetUserId()
		p.Nickname = seat.GetNickname()
		p.IsBot = seat.GetIsBot()
		p.Surrendered = seat.GetSurrendered()
		p.Online = seat.GetOnline()
		p.AutoPlay = seat.GetAutoPlay()
		p.Status = seat.GetStatus()
		p.TotalScore = seat.GetTotalScore()
		if count := seat.GetHandCount(); count > 0 {
			p.HandCnt = int(count)
		}
	}
}

func clearRoundFacts(v *RoomView) {
	v.RoomState = ""
	v.WaitingAction = ""
	v.RoundPhase = clientv1.Phase_PHASE_UNSPECIFIED
	v.ActingSeat = -1
	v.ActingSeats = nil
	v.PendingTile = ""
	v.AvailableActions = nil
	v.ClaimCandidates = map[int32][]string{}
}

func resetRoomToLobby(v *RoomView, keepSettlement bool) {
	v.Phase = phaseLobby
	v.RoomID = ""
	v.RuleID = ""
	v.DisplayName = ""
	v.SeatIndex = -1
	v.RoomState = ""
	v.LastStep = 0
	v.SnapshotStep = 0
	v.DeadlineUnixMS = 0
	v.WallRemaining = 0
	v.RoundIndex = 0
	v.HandIndex = 0
	v.TotalScores = nil
	v.LastAction = nil
	v.ExchangeDirection = -1
	clearRoundFacts(v)
	for i := range v.Players {
		v.Players[i] = PlayerView{}
	}
	for i := range v.QueBySeat {
		v.QueBySeat[i] = -1
	}
	if !keepSettlement {
		v.LastSettlement = nil
	}
}

func shouldDropPendingLeaveFrame(v *RoomView, env *clientv1.Envelope) bool {
	if v.PendingLeaveRoomID == "" || v.Phase != phaseLobby || env == nil {
		return false
	}
	if _, ok := env.GetBody().(*clientv1.Envelope_LeaveRoomResp); ok {
		return false
	}
	roomID := envelopeRoomID(env)
	return roomID != "" && roomID == v.PendingLeaveRoomID
}

func envelopeRoomID(env *clientv1.Envelope) string {
	switch body := env.GetBody().(type) {
	case *clientv1.Envelope_AutoMatchResp:
		return body.AutoMatchResp.GetRoomId()
	case *clientv1.Envelope_CreateRoomResp:
		return body.CreateRoomResp.GetRoomId()
	case *clientv1.Envelope_StartGame:
		return body.StartGame.GetRoomId()
	case *clientv1.Envelope_Settlement:
		return body.Settlement.GetRoomId()
	case *clientv1.Envelope_Snapshot:
		return body.Snapshot.GetRoomId()
	case *clientv1.Envelope_RouteRedirect:
		return body.RouteRedirect.GetRoomId()
	default:
		return ""
	}
}

func actionsForChoice(action string) []string {
	switch action {
	case "hu_choice", "qiang_gang_choice":
		return []string{"hu", "pass"}
	case "gang_choice":
		return []string{"gang", "pass"}
	case "pong_choice":
		return []string{"pong", "pass"}
	default:
		return []string{"pass"}
	}
}

func applySeatTiles(v *RoomView, items []*clientv1.SeatTiles, fn func(*PlayerView, []string)) {
	for _, item := range items {
		seat := item.GetSeatIndex()
		if seat < 0 || seat > 3 {
			continue
		}
		fn(&v.Players[seat], append([]string(nil), item.GetTiles()...))
	}
}

func applyMeldInfos(v *RoomView, items []*clientv1.SeatMelds) {
	for _, item := range items {
		seat := item.GetSeatIndex()
		if seat < 0 || seat > 3 {
			continue
		}
		melds := make([]string, 0, len(item.GetMelds()))
		for _, meld := range item.GetMelds() {
			tiles := meld.GetTiles()
			if len(tiles) == 0 {
				continue
			}
			melds = append(melds, meld.GetKind()+":"+strings.Join(tiles, ","))
		}
		if len(melds) > 0 {
			v.Players[seat].Melds = melds
		}
	}
}

func appendResponseLog(v *RoomView, label string, code clientv1.ErrorCode, message string) {
	if code == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		appendLog(v, label+"成功")
		return
	}
	if message == "" {
		message = code.String()
	}
	v.LastError = message
	appendLog(v, label+"失败: "+message)
}

func appendLog(v *RoomView, text string) {
	v.Log = append(v.Log, LogEntry{At: time.Now(), Text: strings.TrimSpace(text)})
	if len(v.Log) > 100 {
		v.Log = append([]LogEntry(nil), v.Log[len(v.Log)-100:]...)
	}
}

func sortedTiles(tiles []string) []string {
	out := append([]string(nil), tiles...)
	sort.SliceStable(out, func(i, j int) bool { return tileSortKey(out[i]) < tileSortKey(out[j]) })
	return out
}

func removeOneTile(tiles []string, target string) []string {
	out := append([]string(nil), tiles...)
	for i, t := range out {
		if t == target {
			return append(out[:i], out[i+1:]...)
		}
	}
	return out
}

func recordMeld(v *RoomView, seat int32, meld string) {
	if seat < 0 || seat > 3 || meld == "" {
		return
	}
	v.Players[seat].Melds = append(v.Players[seat].Melds, meld)
}
