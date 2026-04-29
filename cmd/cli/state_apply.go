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
	switch body := env.GetBody().(type) {
	case *clientv1.Envelope_LoginResp:
		applyLogin(v, body.LoginResp)
	case *clientv1.Envelope_JoinRoomResp:
		v.SeatIndex = body.JoinRoomResp.GetSeatIndex()
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
		v.Stage = stageExchange
		appendLog(v, fmt.Sprintf("开局，庄家 %d", v.DealerSeat))
	case *clientv1.Envelope_DrawTile:
		applyDraw(v, body.DrawTile)
	case *clientv1.Envelope_DiscardResp:
		appendResponseLog(v, "出牌", body.DiscardResp.GetErrorCode(), body.DiscardResp.GetErrorMessage())
	case *clientv1.Envelope_Action:
		applyAction(v, body.Action)
	case *clientv1.Envelope_Settlement:
		v.Stage = stageSettlement
		v.LastSettlement = body.Settlement
		appendLog(v, "收到结算")
	case *clientv1.Envelope_HeartbeatResp:
		// RTT 由连接层根据本地发送时间更新，这里只记录可见事件。
	case *clientv1.Envelope_LeaveRoomResp:
		appendResponseLog(v, "离房", body.LeaveRoomResp.GetErrorCode(), body.LeaveRoomResp.GetErrorMessage())
		if body.LeaveRoomResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			v.Phase = phaseLobby
			v.RoomID = ""
			v.SeatIndex = -1
		}
	case *clientv1.Envelope_RouteRedirect:
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
		v.Stage = stageDiscard
		appendLog(v, "定缺完成")
	case *clientv1.Envelope_Snapshot:
		applySnapshot(v, body.Snapshot)
	case *clientv1.Envelope_PongResp:
		appendResponseLog(v, "碰", body.PongResp.GetErrorCode(), body.PongResp.GetErrorMessage())
	case *clientv1.Envelope_GangResp:
		appendResponseLog(v, "杠", body.GangResp.GetErrorCode(), body.GangResp.GetErrorMessage())
	case *clientv1.Envelope_HuResp:
		appendResponseLog(v, "胡", body.HuResp.GetErrorCode(), body.HuResp.GetErrorMessage())
	}
}

func applyLogin(v *RoomView, resp *clientv1.LoginResponse) {
	if resp.GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		v.LastError = resp.GetErrorMessage()
		appendLog(v, "登录失败: "+resp.GetErrorMessage())
		return
	}
	v.UserID = resp.GetUserId()
	v.SessionToken = resp.GetSessionToken()
	v.Phase = phaseLobby
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
	applyLobbySeat(v, resp.GetRoomId(), resp.GetSeatIndex())
	appendLog(v, fmt.Sprintf("自动匹配成功: %s 座位 %d", resp.GetRoomId(), resp.GetSeatIndex()))
}

func applyCreateRoom(v *RoomView, resp *clientv1.CreateRoomResponse) {
	if resp.GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		v.LastError = resp.GetErrorMessage()
		appendLog(v, "创建房间失败: "+resp.GetErrorMessage())
		return
	}
	applyLobbySeat(v, resp.GetRoomId(), resp.GetSeatIndex())
	appendLog(v, fmt.Sprintf("创建房间成功: %s 座位 %d", resp.GetRoomId(), resp.GetSeatIndex()))
}

func applyLobbySeat(v *RoomView, roomID string, seat int32) {
	v.RoomID = roomID
	v.SeatIndex = seat
	v.Phase = phaseTable
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
	if seat == v.SeatIndex {
		p := &v.Players[seat]
		p.Hand = sortedTiles(append(p.Hand, t))
		p.HandCnt = len(p.Hand)
	}
	appendLog(v, fmt.Sprintf("%d 摸牌 %s", seat, t))
}

func applyAction(v *RoomView, action *clientv1.ActionNotify) {
	seat := action.GetSeatIndex()
	if seat >= 0 && seat < 4 {
		v.ActingSeat = seat
	}
	switch action.GetAction() {
	case "exchange_three":
		v.Stage = stageExchange
	case "que_men":
		v.Stage = stageQueMen
	case "discard":
		applyDiscardAction(v, seat, action.GetTile())
	case "pong":
		recordMeld(v, seat, "pong:"+action.GetTile())
	case "gang":
		recordMeld(v, seat, "gang:"+action.GetTile())
	case "hu", "tsumo":
		if seat >= 0 && seat < 4 {
			v.Players[seat].Hued = true
		}
	case "hu_choice", "qiang_gang_choice", "pong_choice", "gang_choice":
		v.Stage = stageClaim
		v.PendingTile = action.GetTile()
	}
	appendLog(v, fmt.Sprintf("%d %s %s", seat, action.GetAction(), action.GetTile()))
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
	}
	v.Stage = stageDiscard
}

func applyExchangeDone(v *RoomView, done *clientv1.ExchangeThreeDoneNotify) {
	for _, item := range done.GetPerSeat() {
		if item.GetSeatIndex() == v.SeatIndex {
			p := &v.Players[v.SeatIndex]
			p.Hand = sortedTiles(append(p.Hand, item.GetTiles()...))
			p.HandCnt = len(p.Hand)
			break
		}
	}
	v.Stage = stageQueMen
	appendLog(v, "换三张完成")
}

func applySnapshot(v *RoomView, snap *clientv1.SnapshotNotify) {
	v.RoomID = snap.GetRoomId()
	v.Stage = snap.GetState()
	v.ActingSeat = snap.GetActingSeat()
	v.PendingTile = snap.GetPendingTile()
	v.AvailableAction = append([]string(nil), snap.GetAvailableActions()...)
	v.ClaimCandidates = make(map[int32][]string, len(snap.GetClaimCandidates()))
	for _, candidate := range snap.GetClaimCandidates() {
		v.ClaimCandidates[candidate.GetSeatIndex()] = append([]string(nil), candidate.GetActions()...)
	}
	for seat, userID := range snap.GetPlayerIds() {
		if seat < len(v.Players) {
			v.Players[seat].UserID = userID
		}
	}
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
	appendLog(v, "快照已恢复")
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
