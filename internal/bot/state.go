package bot

import (
	"sort"
	"strings"
	"sync"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// BotState 保存机器人从协议事件流推导出的可见状态。
type BotState struct {
	mu              sync.RWMutex
	view            BotView
	pendingExchange []string
}

// NewState 创建机器人状态。
func NewState() *BotState {
	st := &BotState{}
	st.view.SeatIndex = -1
	st.view.ActingSeat = -1
	st.view.ClaimCandidates = make(map[int32][]string)
	st.view.DiscardsBySeat = make([][]string, 4)
	st.view.MeldsBySeat = make([][]string, 4)
	st.view.DrawnBySeat = make([][]string, 4)
	for i := range st.view.QueBySeat {
		st.view.QueBySeat[i] = -1
	}
	return st
}

// Snapshot 返回状态副本。
func (s *BotState) Snapshot() BotView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneView(s.view)
}

// Apply 应用一条服务端下行 Envelope。
func (s *BotState) Apply(env *clientv1.Envelope) {
	if env == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyEnvelope(env)
	s.view.UpdatedAt = time.Now()
}

// RememberExchange 记录本地刚提交的换三张，便于换牌完成后修正手牌。
func (s *BotState) RememberExchange(tiles []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingExchange = append([]string(nil), tiles...)
}

func (s *BotState) applyEnvelope(env *clientv1.Envelope) {
	v := &s.view
	if pu := extractPhaseUpdate(env); pu != nil {
		applyBotPhaseUpdate(v, pu)
	}
	switch body := env.GetBody().(type) {
	case *clientv1.Envelope_LoginResp:
		if body.LoginResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			v.UserID = body.LoginResp.GetUserId()
		}
	case *clientv1.Envelope_JoinRoomResp:
		if body.JoinRoomResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			v.SeatIndex = body.JoinRoomResp.GetSeatIndex()
			clearRound(v)
		}
	case *clientv1.Envelope_AutoMatchResp:
		if body.AutoMatchResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			v.RoomID = body.AutoMatchResp.GetRoomId()
			v.SeatIndex = body.AutoMatchResp.GetSeatIndex()
			clearRound(v)
		}
	case *clientv1.Envelope_CreateRoomResp:
		if body.CreateRoomResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			v.RoomID = body.CreateRoomResp.GetRoomId()
			v.SeatIndex = body.CreateRoomResp.GetSeatIndex()
			clearRound(v)
		}
	case *clientv1.Envelope_StartGame:
		v.RoomID = body.StartGame.GetRoomId()
		v.LastSettlement = nil
	case *clientv1.Envelope_InitialDeal:
		if body.InitialDeal.GetSeatIndex() == v.SeatIndex {
			v.HandTiles = sortedTiles(body.InitialDeal.GetTiles())
		}
	case *clientv1.Envelope_OpeningDone:
		applyOpeningDone(v, body.OpeningDone, s.pendingExchange)
		if body.OpeningDone.GetAction() == string(ActionExchangeThree) || body.OpeningDone.GetKind() == "exchange_done" {
			s.pendingExchange = nil
		}
	case *clientv1.Envelope_DrawTile:
		applyDraw(v, body.DrawTile)
	case *clientv1.Envelope_Action:
		applyAction(v, body.Action)
	case *clientv1.Envelope_PassResp:
		if body.PassResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			if v.WaitingAction == "tsumo_window" && body.PassResp.GetPhaseUpdate() == nil {
				v.WaitingAction = "discard"
				v.AvailableAction = []string{"discard"}
			} else if body.PassResp.GetPhaseUpdate() == nil {
				v.AvailableAction = nil
			}
			v.ClaimCandidates = map[int32][]string{}
		}
	case *clientv1.Envelope_Snapshot:
		applySnapshot(v, body.Snapshot)
	case *clientv1.Envelope_Settlement:
		v.LastSettlement = body.Settlement
		v.WaitingAction = "none"
		v.AvailableAction = nil
		v.ClaimCandidates = map[int32][]string{}
	case *clientv1.Envelope_LeaveRoomResp:
		if body.LeaveRoomResp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			v.RoomID = ""
			v.SeatIndex = -1
			clearRound(v)
		}
	}
}

func applyOpeningDone(v *BotView, done *clientv1.OpeningDoneNotify, exchanged []string) {
	switch {
	case done.GetAction() == string(ActionExchangeThree) || done.GetKind() == "exchange_done":
		applyOpeningExchangeDone(v, done, exchanged)
	case done.GetAction() == string(ActionQueMen) || done.GetKind() == "missing_suit_done":
		for seat, suit := range openingSeatInts(done, "que_suit") {
			if seat >= 0 && seat < len(v.QueBySeat) {
				v.QueBySeat[seat] = suit
			}
		}
	}
}

func applyOpeningExchangeDone(v *BotView, done *clientv1.OpeningDoneNotify, exchanged []string) {
	for _, item := range openingSeatTiles(done, "received") {
		if item.GetSeatIndex() != v.SeatIndex {
			continue
		}
		if local := openingLocalTiles(done, "exchanged_away"); len(local) > 0 {
			exchanged = local
		}
		for _, t := range exchanged {
			v.HandTiles = removeOneTile(v.HandTiles, t)
		}
		v.HandTiles = sortedTiles(append(v.HandTiles, item.GetTiles()...))
		break
	}
}

func openingSeatTiles(done *clientv1.OpeningDoneNotify, key string) []*clientv1.SeatTiles {
	if done == nil {
		return nil
	}
	for _, group := range done.GetSeatTiles() {
		if group.GetKey() == key {
			return group.GetSeats()
		}
	}
	return nil
}

func openingSeatInts(done *clientv1.OpeningDoneNotify, key string) []int32 {
	if done == nil {
		return nil
	}
	for _, group := range done.GetSeatInts() {
		if group.GetKey() == key {
			return group.GetValues()
		}
	}
	return nil
}

func openingLocalTiles(done *clientv1.OpeningDoneNotify, key string) []string {
	if done == nil {
		return nil
	}
	for _, group := range done.GetLocalTiles() {
		if group.GetKey() == key {
			return append([]string(nil), group.GetTiles()...)
		}
	}
	return nil
}

func extractPhaseUpdate(env *clientv1.Envelope) *clientv1.PhaseUpdate {
	if env == nil {
		return nil
	}
	switch body := env.GetBody().(type) {
	case *clientv1.Envelope_StartGame:
		return body.StartGame.GetPhaseUpdate()
	case *clientv1.Envelope_DrawTile:
		return body.DrawTile.GetPhaseUpdate()
	case *clientv1.Envelope_Action:
		return body.Action.GetPhaseUpdate()
	case *clientv1.Envelope_OpeningDone:
		return body.OpeningDone.GetPhaseUpdate()
	case *clientv1.Envelope_Settlement:
		return body.Settlement.GetPhaseUpdate()
	case *clientv1.Envelope_Snapshot:
		return body.Snapshot.GetPhaseUpdate()
	case *clientv1.Envelope_DiscardResp:
		return body.DiscardResp.GetPhaseUpdate()
	case *clientv1.Envelope_OpeningActionResp:
		return body.OpeningActionResp.GetPhaseUpdate()
	case *clientv1.Envelope_PongResp:
		return body.PongResp.GetPhaseUpdate()
	case *clientv1.Envelope_GangResp:
		return body.GangResp.GetPhaseUpdate()
	case *clientv1.Envelope_HuResp:
		return body.HuResp.GetPhaseUpdate()
	case *clientv1.Envelope_PassResp:
		return body.PassResp.GetPhaseUpdate()
	case *clientv1.Envelope_ReadyResp:
		return body.ReadyResp.GetPhaseUpdate()
	default:
		return nil
	}
}

func applyBotPhaseUpdate(v *BotView, pu *clientv1.PhaseUpdate) {
	if v == nil || pu == nil {
		return
	}
	actions := append([]string(nil), pu.GetAvailableActions()...)
	v.AvailableAction = actions
	if acting := pu.GetActingSeats(); len(acting) > 0 {
		v.ActingSeat = acting[0]
	} else if pu.GetReason() == clientv1.WaitingReason_WAITING_REASON_NONE {
		v.ActingSeat = -1
	}
	switch pu.GetReason() {
	case clientv1.WaitingReason_WAITING_REASON_OPENING:
		v.WaitingAction = firstAction(actions)
	case clientv1.WaitingReason_WAITING_REASON_DISCARD:
		v.WaitingAction = "discard"
	case clientv1.WaitingReason_WAITING_REASON_CLAIM_WINDOW:
		v.WaitingAction = "claim_window"
	case clientv1.WaitingReason_WAITING_REASON_TSUMO:
		v.WaitingAction = "tsumo_window"
	case clientv1.WaitingReason_WAITING_REASON_NONE:
		if len(actions) == 0 {
			v.WaitingAction = ""
		}
	}
}

func firstAction(actions []string) string {
	if len(actions) == 0 {
		return ""
	}
	return actions[0]
}

func applyDraw(v *BotView, draw *clientv1.DrawTileNotify) {
	seat := draw.GetSeatIndex()
	t := strings.TrimSpace(draw.GetTile())
	if seat >= 0 && seat < 4 && t != "" {
		v.DrawnBySeat[seat] = append(v.DrawnBySeat[seat], t)
	}
	v.ActingSeat = seat
	v.PendingTile = t
	if draw.GetPhaseUpdate() == nil {
		v.WaitingAction = "discard"
		v.AvailableAction = []string{"discard"}
	}
	if seat == v.SeatIndex && t != "" {
		v.HandTiles = sortedTiles(append(v.HandTiles, t))
	}
}

func applyAction(v *BotView, action *clientv1.ActionNotify) {
	seat := action.GetSeatIndex()
	act := action.GetAction()
	t := action.GetTile()
	if seat >= 0 && seat < 4 {
		v.ActingSeat = seat
	}
	if action.GetPhaseUpdate() != nil {
		// PhaseUpdate is authoritative for action windows; action strings are only visible projections.
		return
	}
	switch act {
	case "discard":
		recordDiscard(v, seat, t)
	case "pong":
		recordMeld(v, seat, "pong:"+t)
	case "gang":
		recordMeld(v, seat, "gang:"+t)
	case "hu", "tsumo":
		v.AvailableAction = nil
		v.ClaimCandidates = map[int32][]string{}
	case "tsumo_choice":
		v.PendingTile = t
		if seat == v.SeatIndex {
			v.WaitingAction = "tsumo_window"
			v.AvailableAction = []string{"hu", "pass"}
		}
	case "pong_choice", "gang_choice", "hu_choice", "qiang_gang_choice":
		v.PendingTile = t
		if seat == v.SeatIndex {
			v.WaitingAction = "claim_window"
			v.AvailableAction = actionsForChoice(act)
			v.ClaimCandidates = map[int32][]string{seat: append([]string(nil), v.AvailableAction...)}
		}
	}
}

func applySnapshot(v *BotView, snap *clientv1.SnapshotNotify) {
	v.RoomID = snap.GetRoomId()
	v.RoomState = snap.GetState()
	v.Closed = snap.GetState() == "closed"
	v.WaitingAction = snap.GetWaitingAction()
	v.ActingSeat = snap.GetActingSeat()
	v.PendingTile = snap.GetPendingTile()
	v.AvailableAction = append([]string(nil), snap.GetAvailableActions()...)
	v.HandTiles = sortedTiles(snap.GetYourHandTiles())
	v.ClaimCandidates = make(map[int32][]string, len(snap.GetClaimCandidates()))
	for _, candidate := range snap.GetClaimCandidates() {
		v.ClaimCandidates[candidate.GetSeatIndex()] = append([]string(nil), candidate.GetActions()...)
	}
	for seat, suit := range snap.GetQueSuitBySeat() {
		if seat >= 0 && seat < len(v.QueBySeat) {
			v.QueBySeat[seat] = suit
		}
	}
	applySeatTiles(v.DiscardsBySeat, snap.GetDiscardsBySeat())
	applySeatTiles(v.MeldsBySeat, snap.GetMeldsBySeat())
}

func recordDiscard(v *BotView, seat int32, raw string) {
	if seat < 0 || seat > 3 || raw == "" {
		return
	}
	v.DiscardsBySeat[seat] = append(v.DiscardsBySeat[seat], raw)
	if seat == v.SeatIndex {
		v.HandTiles = removeOneTile(v.HandTiles, raw)
	}
	v.WaitingAction = "none"
	v.PendingTile = ""
	v.AvailableAction = nil
	v.ClaimCandidates = map[int32][]string{}
}

func recordMeld(v *BotView, seat int32, meld string) {
	if seat < 0 || seat > 3 || meld == "" {
		return
	}
	v.MeldsBySeat[seat] = append(v.MeldsBySeat[seat], meld)
	if seat == v.SeatIndex {
		parts := strings.SplitN(meld, ":", 2)
		if len(parts) == 2 {
			removeCount := 2
			if parts[0] == "gang" {
				removeCount = 3
			}
			for i := 0; i < removeCount; i++ {
				v.HandTiles = removeOneTile(v.HandTiles, parts[1])
			}
		}
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

func applySeatTiles(dst [][]string, items []*clientv1.SeatTiles) {
	for _, item := range items {
		seat := item.GetSeatIndex()
		if seat < 0 || seat > 3 {
			continue
		}
		dst[seat] = append([]string(nil), item.GetTiles()...)
	}
}

func clearRound(v *BotView) {
	v.RoomState = ""
	v.WaitingAction = ""
	v.ActingSeat = -1
	v.PendingTile = ""
	v.AvailableAction = nil
	v.ClaimCandidates = map[int32][]string{}
	v.HandTiles = nil
	v.DiscardsBySeat = make([][]string, 4)
	v.MeldsBySeat = make([][]string, 4)
	v.DrawnBySeat = make([][]string, 4)
	v.Closed = false
}

func cloneView(in BotView) BotView {
	out := in
	out.AvailableAction = append([]string(nil), in.AvailableAction...)
	out.HandTiles = append([]string(nil), in.HandTiles...)
	out.DiscardsBySeat = cloneMatrix(in.DiscardsBySeat)
	out.MeldsBySeat = cloneMatrix(in.MeldsBySeat)
	out.DrawnBySeat = cloneMatrix(in.DrawnBySeat)
	out.ClaimCandidates = make(map[int32][]string, len(in.ClaimCandidates))
	for seat, actions := range in.ClaimCandidates {
		out.ClaimCandidates[seat] = append([]string(nil), actions...)
	}
	return out
}

func cloneMatrix(in [][]string) [][]string {
	out := make([][]string, 4)
	for i := 0; i < len(out) && i < len(in); i++ {
		out[i] = append([]string(nil), in[i]...)
	}
	return out
}

func sortedTiles(tiles []string) []string {
	out := append([]string(nil), tiles...)
	sort.SliceStable(out, func(i, j int) bool { return tileSortKey(out[i]) < tileSortKey(out[j]) })
	return out
}

func tileSortKey(raw string) int {
	if len(raw) != 2 {
		return 999
	}
	suit := 0
	switch raw[0] {
	case 'm':
		suit = 0
	case 'p':
		suit = 1
	case 's':
		suit = 2
	default:
		suit = 9
	}
	return suit*10 + int(raw[1]-'0')
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
