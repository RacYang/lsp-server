package main

import (
	"fmt"
	"strings"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// TablePhase 是 TUI 可渲染和可输入的局内阶段。
type TablePhase int

const (
	PhaseUnknown TablePhase = iota
	PhaseLogin
	PhaseLobby
	PhaseWaiting
	PhaseOpening
	PhaseDiscard
	PhaseClaim
	PhaseTsumo
	PhaseSettlement
	PhaseMyTurnIdle
	PhaseMyTurnSelected
	PhaseOtherTurn
)

// PlayerAction 是客户端可发给服务端的玩家动作集合。
type PlayerAction string

const (
	ActionDiscard       PlayerAction = "discard"
	ActionExchangeThree PlayerAction = "exchange_three"
	ActionQueMen        PlayerAction = "que_men"
	ActionPong          PlayerAction = "pong"
	ActionChi           PlayerAction = "chi"
	ActionGang          PlayerAction = "gang"
	ActionHu            PlayerAction = "hu"
	ActionPass          PlayerAction = "pass"
)

// ClaimSpec 是 InteractionModel 暴露给浮窗层的结构化抢答信息。
type ClaimSpec struct {
	Dialog *ClaimDialogState
}

// InteractionModel 是从 RoomView 协议事实纯派生出来的 TUI 模型。
type InteractionModel struct {
	Phase       TablePhase
	SelfSeat    int32
	ActingSeat  int32
	Allowed     []PlayerAction
	PendingTile string
	Claim       *ClaimSpec
	Settlement  *SettlementSummary
	Hint        string
}

// SeatDisplay 是牌桌渲染层消费的座位展示模型。
type SeatDisplay struct {
	Seat          int32
	Position      SeatPosition
	RelativeLabel string
	Name          string
	Status        string
	HandCount     int
	Que           string
	Melds         string
	IsSelf        bool
	IsFocus       bool
}

// TableUXModel 是牌桌 UI/输入共享的唯一玩家体验模型。
//
// 它只从 RoomView、光标和当前时间派生，不写状态、不做 IO；渲染与输入都应优先消费
// 这里的阶段、提示和动作许可，避免各自重新解释 RoomView 字段。
type TableUXModel struct {
	Phase            TablePhase
	SelfSeat         int32
	FocusSeat        int32
	CountdownSeconds int
	HasCountdown     bool
	PrimaryPrompt    string
	KeyHint          string
	AllowedActions   []PlayerAction
	DisabledReason   string
	PendingFeedback  string
	Claim            *ClaimSpec
	Settlement       *SettlementSummary
	Seats            [4]SeatDisplay
}

// DeriveInteractionModel 不读写锁、不做 IO，只把 RoomView 翻译成 UI 可消费状态。
func DeriveInteractionModel(view RoomView) InteractionModel {
	model := InteractionModel{
		SelfSeat:    view.SeatIndex,
		ActingSeat:  view.ActingSeat,
		PendingTile: view.PendingTile,
	}
	if view.LastSettlement != nil {
		model.Phase = PhaseSettlement
		model.Settlement = snapshotSettlementSummary(view)
		model.Hint = "本局结束"
		return model
	}
	switch view.Phase {
	case phaseLogin:
		model.Phase = PhaseLogin
		model.Hint = "正在登录"
		return model
	case phaseLobby:
		model.Phase = PhaseLobby
		model.Hint = "大厅"
		return model
	}
	if view.RoundPhase == clientv1.Phase_PHASE_DRAW {
		model.Phase = PhaseOtherTurn
		model.Hint = waitingDrawHint(view)
		return model
	}
	switch view.WaitingAction {
	case "exchange_three":
		model.Phase = PhaseOpening
		// 川麻血战：换三张是 4 家并发，不存在"轮到谁"的概念。只要本地 SeatIndex 合法
		// 就给 ActionExchangeThree，让任何座位都能 Space 标记 / Enter 提交。
		if view.SeatIndex >= 0 && view.SeatIndex < 4 {
			model.Allowed = []PlayerAction{ActionExchangeThree}
			model.Hint = "请选三张换三张"
		} else {
			model.Hint = waitingHint(view)
		}
	case "que_men":
		model.Phase = PhaseOpening
		// 定缺同理：4 家并发选缺一门，不依赖 ActingSeat。
		if view.SeatIndex >= 0 && view.SeatIndex < 4 {
			model.Allowed = []PlayerAction{ActionQueMen}
			// [Q1.1]/[G2] 定缺键位为 m / p / s；[Q2.1] 必须提示「选定后不可更改」。
			model.Hint = "请定缺：m 缺万 / p 缺筒 / s 缺条（选定后不可更改）"
		} else {
			model.Hint = waitingHint(view)
		}
	case "discard":
		model.Phase = PhaseDiscard
		if view.SeatIndex == view.ActingSeat {
			model.Allowed = actionsFromStrings(view.AvailableActions)
			if len(model.Allowed) == 0 {
				model.Allowed = []PlayerAction{ActionDiscard}
			}
			model.Hint = "该你打牌"
		} else {
			model.Hint = waitingHint(view)
		}
	case "claim_window":
		model.Phase = PhaseClaim
		actions := claimActionsForSeat(view, view.SeatIndex)
		model.Allowed = actionsFromStrings(actions)
		if len(model.Allowed) > 0 {
			model.Claim = &ClaimSpec{Dialog: buildClaimDialog(view, model.Allowed)}
			model.Hint = "请决定"
		} else {
			model.Hint = waitingHint(view)
		}
	case "tsumo_window":
		model.Phase = PhaseTsumo
		if view.SeatIndex == view.ActingSeat {
			model.Allowed = []PlayerAction{ActionHu, ActionPass}
			model.Claim = &ClaimSpec{Dialog: buildClaimDialog(view, model.Allowed)}
			model.Hint = "自摸：胡或过"
		} else {
			model.Hint = waitingHint(view)
		}
	default:
		model.Phase = PhaseWaiting
		model.Hint = "等待开始"
	}
	return model
}

// DerivePhase 把协议事实、交互模型与本地光标状态收敛成渲染层唯一消费的阶段。
func DerivePhase(view RoomView, cursor *HandCursor) TablePhase {
	model := DeriveInteractionModel(view)
	switch model.Phase {
	case PhaseSettlement:
		return PhaseSettlement
	case PhaseClaim, PhaseTsumo:
		if model.Claim != nil {
			return PhaseClaim
		}
		return PhaseOtherTurn
	case PhaseOpening:
		if containsAction(model.Allowed, ActionExchangeThree) || containsAction(model.Allowed, ActionQueMen) {
			return PhaseOpening
		}
		return PhaseOtherTurn
	case PhaseDiscard:
		if view.SeatIndex == view.ActingSeat {
			if cursor != nil && cursor.Mode == CursorModeSingle && cursor.Index >= 0 {
				return PhaseMyTurnSelected
			}
			return PhaseMyTurnIdle
		}
		return PhaseOtherTurn
	case PhaseWaiting:
		return PhaseWaiting
	default:
		return model.Phase
	}
}

// DeriveTableUXModel 把协议投影、光标和时间收敛成渲染/输入共享的玩家体验模型。
func DeriveTableUXModel(view RoomView, cursor *HandCursor, now time.Time) TableUXModel {
	model := DeriveInteractionModel(view)
	phase := DerivePhase(view, cursor)
	ux := TableUXModel{
		Phase:          phase,
		SelfSeat:       view.SeatIndex,
		FocusSeat:      view.ActingSeat,
		AllowedActions: append([]PlayerAction(nil), model.Allowed...),
		Claim:          model.Claim,
		Settlement:     model.Settlement,
	}
	if left, ok := actionCountdown(view, now); ok {
		ux.HasCountdown = true
		ux.CountdownSeconds = left
	}
	for seat := int32(0); seat < 4; seat++ {
		pos := RelativeSeat(view.SeatIndex, seat)
		ux.Seats[seat] = SeatDisplay{
			Seat:          seat,
			Position:      pos,
			RelativeLabel: relativeSeatLabel(pos, seat == view.SeatIndex),
			Name:          playerDisplayName(view, seat),
			Status:        compactSeatState(view, seat),
			HandCount:     handCountForSeat(view, seat),
			Que:           queLabel(view.QueBySeat[seat]),
			Melds:         formatMeldGlyphs(view.Players[seat].Melds),
			IsSelf:        seat == view.SeatIndex,
			IsFocus:       focusOnSeat(view, cursor, seat),
		}
	}
	ux.PendingFeedback = pendingFeedback(view, cursor)
	ux.DisabledReason = disabledReason(view, cursor, model)
	ux.PrimaryPrompt = primaryPrompt(view, cursor, model, ux, now)
	ux.KeyHint = keyHintForUX(view, cursor, ux)
	return ux
}

func primaryPrompt(view RoomView, cursor *HandCursor, model InteractionModel, ux TableUXModel, now time.Time) string {
	if noticeActive(view, now) {
		return view.UXNotice
	}
	if ux.PendingFeedback != "" {
		return ux.PendingFeedback
	}
	if phase := ux.Phase; phase == PhaseOpening && cursor != nil && cursor.Mode == CursorModeMulti3 && view.SeatIndex >= 0 {
		// [E2.1] 底栏必须实时显示「已选 N/3」字面格式；满 3 张时切到提交提示。
		marked := len(cursor.Marked)
		if marked < 3 {
			return fmt.Sprintf("换三张：已选 %d/3，须同花色", marked)
		}
		return "换三张：已选 3/3，按 Enter 交换"
	}
	if ux.Phase == PhaseOpening && cursor != nil && cursor.Mode == CursorModeQueMen {
		return fmt.Sprintf("定缺：已选缺%s，Enter 确认（选定后不可更改）", queChoiceLabel(cursor.Index))
	}
	if ux.Phase == PhaseMyTurnSelected && cursor != nil && cursor.Mode == CursorModeSingle && cursor.Index >= 0 && view.SeatIndex >= 0 {
		hand := view.Players[view.SeatIndex].Hand
		if cursor.Index < len(hand) {
			return fmt.Sprintf("轮到你：已选 %s，Enter 打出", TileName(hand[cursor.Index]))
		}
	}
	switch model.Phase {
	case PhaseOpening:
		switch {
		case containsAction(model.Allowed, ActionExchangeThree):
			return "换三张：选 3 张同花色"
		case containsAction(model.Allowed, ActionQueMen):
			return "定缺：选一门不要，选定后不可更改"
		default:
			return waitingUXHint(view)
		}
	case PhaseDiscard:
		if containsAction(model.Allowed, ActionDiscard) {
			return "轮到你：选择一张牌打出"
		}
		return waitingUXHint(view)
	case PhaseClaim, PhaseTsumo:
		if model.Claim != nil {
			return "请决定：" + playerActionList(model.Allowed)
		}
		return waitingUXHint(view)
	case PhaseSettlement:
		return "本局结束"
	case PhaseWaiting:
		if emptySeatCount(view) > 0 {
			return "座位未满 - b 补一个 / B 补满"
		}
		return "人已坐齐：按 Enter 准备开局"
	default:
		if model.Hint != "" {
			return model.Hint
		}
		return "等待开始"
	}
}

func keyHintForUX(view RoomView, cursor *HandCursor, ux TableUXModel) string {
	if ux.DisabledReason != "" && len(ux.AllowedActions) == 0 {
		return ux.DisabledReason + "　? 帮助"
	}
	switch ux.Phase {
	case PhaseOpening:
		if containsAction(ux.AllowedActions, ActionQueMen) {
			return "定缺：←→ 选择缺门　Enter 确认　m/p/s 快捷"
		}
		marked := 0
		if cursor != nil {
			marked = len(cursor.Marked)
		}
		return fmt.Sprintf("换三张：已选 %d/3　←→ 选牌　Space 标记　Enter 确认", marked)
	case PhaseMyTurnIdle, PhaseMyTurnSelected:
		if containsAction(ux.AllowedActions, ActionGang) {
			return "轮到你：←→ 选牌　Enter 打出　g 杠"
		}
		return "轮到你：←→ 选牌　Enter 打出"
	case PhaseClaim:
		return claimKeyHint(ux)
	case PhaseSettlement:
		return "本局结束：r 再开一桌　l 离桌　Enter 停留"
	case PhaseWaiting:
		if emptySeatCount(view) > 0 {
			return "等人入座：b 补一个机器人　B 补满"
		}
		return "准备开局：Enter 确认　? 帮助"
	}
	return "等待：? 帮助　i 房间信息　Tab 玩家"
}

func queChoiceLabel(idx int) string {
	switch idx {
	case 0:
		return "万"
	case 1:
		return "筒"
	case 2:
		return "条"
	default:
		return "万"
	}
}

func disabledReason(view RoomView, cursor *HandCursor, model InteractionModel) string {
	if view.SeatIndex < 0 || view.SeatIndex > 3 {
		return "尚未入座"
	}
	if cursor != nil && cursor.Pending {
		return "正在等待服务端确认"
	}
	switch model.Phase {
	case PhaseDiscard:
		if !containsAction(model.Allowed, ActionDiscard) {
			return "还没轮到你：" + waitingUXHint(view)
		}
	case PhaseClaim, PhaseTsumo:
		if model.Claim == nil {
			return "还没轮到你：" + waitingUXHint(view)
		}
	case PhaseOpening:
		if len(model.Allowed) == 0 {
			return "当前不能操作：" + waitingUXHint(view)
		}
	}
	return ""
}

func playerActionList(actions []PlayerAction) string {
	if len(actions) == 0 {
		return "过"
	}
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		switch action {
		case ActionHu:
			labels = append(labels, "胡")
		case ActionGang:
			labels = append(labels, "杠")
		case ActionPong:
			labels = append(labels, "碰")
		case ActionChi:
			labels = append(labels, "吃")
		case ActionPass:
			labels = append(labels, "过")
		case ActionDiscard:
			labels = append(labels, "出牌")
		case ActionExchangeThree:
			labels = append(labels, "换三张")
		case ActionQueMen:
			labels = append(labels, "定缺")
		}
	}
	return strings.Join(labels, " / ")
}

func claimKeyHint(ux TableUXModel) string {
	shortcuts := make([]string, 0, len(ux.AllowedActions))
	for _, action := range ux.AllowedActions {
		switch action {
		case ActionHu:
			shortcuts = append(shortcuts, "h 胡")
		case ActionGang:
			shortcuts = append(shortcuts, "g 杠")
		case ActionPong:
			shortcuts = append(shortcuts, "p 碰")
		case ActionChi:
			shortcuts = append(shortcuts, "c 吃")
		case ActionPass:
			shortcuts = append(shortcuts, "n 过")
		}
	}
	if len(shortcuts) == 0 {
		shortcuts = append(shortcuts, "n 过")
	}
	return "请决定：←→ 选择　Enter 确认　" + strings.Join(shortcuts, "　")
}

func pendingFeedback(view RoomView, cursor *HandCursor) string {
	if cursor == nil || !cursor.Pending {
		return ""
	}
	switch cursor.Mode {
	case CursorModeSingle:
		if view.SeatIndex >= 0 && view.SeatIndex < 4 && cursor.Index >= 0 {
			hand := view.Players[view.SeatIndex].Hand
			if cursor.Index < len(hand) {
				return "出牌中... " + TileName(hand[cursor.Index])
			}
		}
		return "出牌中..."
	case CursorModeMulti3:
		return "换三张提交中..."
	default:
		return "提交中..."
	}
}

func noticeActive(view RoomView, now time.Time) bool {
	return view.UXNotice != "" && !view.UXNoticeUntil.IsZero() && now.Before(view.UXNoticeUntil)
}

func relativeSeatLabel(pos SeatPosition, self bool) string {
	if self {
		return "我"
	}
	switch pos {
	case SeatPosLeft:
		return "下家"
	case SeatPosTop:
		return "对家"
	case SeatPosRight:
		return "上家"
	default:
		return "玩家"
	}
}

func actionsFromStrings(actions []string) []PlayerAction {
	out := make([]PlayerAction, 0, len(actions))
	for _, action := range actions {
		switch action {
		case "discard":
			out = append(out, ActionDiscard)
		case "exchange_three":
			out = append(out, ActionExchangeThree)
		case "que_men":
			out = append(out, ActionQueMen)
		case "pong":
			out = append(out, ActionPong)
		case "chi":
			out = append(out, ActionChi)
		case "gang":
			out = append(out, ActionGang)
		case "hu":
			out = append(out, ActionHu)
		case "pass":
			out = append(out, ActionPass)
		}
	}
	return out
}

func claimActionsForSeat(view RoomView, seat int32) []string {
	if seat < 0 {
		return nil
	}
	if actions, ok := view.ClaimCandidates[seat]; ok {
		return append([]string(nil), actions...)
	}
	if seat == view.ActingSeat {
		return append([]string(nil), view.AvailableActions...)
	}
	return nil
}

func buildClaimDialog(view RoomView, actions []PlayerAction) *ClaimDialogState {
	trigger := ClaimTriggerPong
	switch {
	case view.WaitingAction == "tsumo_window":
		trigger = ClaimTriggerSelfDraw
	case containsAction(actions, ActionHu) && containsAction(actions, ActionPong):
		trigger = ClaimTriggerPongOrHu
	case containsAction(actions, ActionHu):
		trigger = ClaimTriggerRon
	case containsAction(actions, ActionGang):
		trigger = ClaimTriggerGang
	}
	claimActions := make([]ClaimAction, 0, len(actions)+1)
	for _, action := range actions {
		switch action {
		case ActionHu:
			claimActions = append(claimActions, ClaimActionHu)
		case ActionPong:
			claimActions = append(claimActions, ClaimActionPong)
		case ActionChi:
			claimActions = append(claimActions, ClaimActionChow)
		case ActionGang:
			claimActions = append(claimActions, ClaimActionGang)
		case ActionPass:
			claimActions = append(claimActions, ClaimActionPass)
		}
	}
	if !containsClaimAction(claimActions, ClaimActionPass) {
		claimActions = append(claimActions, ClaimActionPass)
	}
	triggerSeat := claimTriggerSeat(view)
	return &ClaimDialogState{
		Trigger:     trigger,
		TriggerSeat: triggerSeat,
		TriggerName: nicknameForSeat(view, triggerSeat),
		Tile:        view.PendingTile,
		Actions:     claimActions,
	}
}

func claimTriggerSeat(view RoomView) int32 {
	if view.WaitingAction == "tsumo_window" {
		return view.SeatIndex
	}
	if action := view.LastAction; action != nil && action.GetAction() == "discard" {
		if view.PendingTile == "" || action.GetTile() == "" || action.GetTile() == view.PendingTile {
			return action.GetActorSeat()
		}
	}
	return -1
}

func containsAction(actions []PlayerAction, target PlayerAction) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}

func containsClaimAction(actions []ClaimAction, target ClaimAction) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}

func waitingHint(view RoomView) string {
	if view.ActingSeat >= 0 && view.ActingSeat < 4 {
		return "等待 " + nicknameForSeat(view, view.ActingSeat)
	}
	return "等待开始"
}

func waitingDrawHint(view RoomView) string {
	if view.ActingSeat >= 0 && view.ActingSeat < 4 && gameStarted(view) {
		return "等待" + sideLabel(view, view.ActingSeat) + "摸牌"
	}
	return waitingHint(view)
}

func waitingUXHint(view RoomView) string {
	if view.ActingSeat >= 0 && view.ActingSeat < 4 && gameStarted(view) {
		return "等待" + sideLabel(view, view.ActingSeat) + "操作"
	}
	return "等待开始"
}
