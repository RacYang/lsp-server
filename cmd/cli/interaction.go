package main

// TablePhase 是 TUI 可渲染和可输入的局内阶段。
type TablePhase int

const (
	PhaseUnknown TablePhase = iota
	PhaseLogin
	PhaseLobby
	PhaseWaiting
	PhaseExchange
	PhaseQueMen
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
	switch view.WaitingAction {
	case "exchange_three":
		model.Phase = PhaseExchange
		// 川麻血战：换三张是 4 家并发，不存在"轮到谁"的概念。只要本地 SeatIndex 合法
		// 就给 ActionExchangeThree，让任何座位都能 Space 标记 / Enter 提交。
		if view.SeatIndex >= 0 && view.SeatIndex < 4 {
			model.Allowed = []PlayerAction{ActionExchangeThree}
			model.Hint = "请选三张换三张"
		} else {
			model.Hint = waitingHint(view)
		}
	case "que_men":
		model.Phase = PhaseQueMen
		// 定缺同理：4 家并发选缺一门，不依赖 ActingSeat。
		if view.SeatIndex >= 0 && view.SeatIndex < 4 {
			model.Allowed = []PlayerAction{ActionQueMen}
			model.Hint = "请定缺 (m / p / s)"
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
	case PhaseExchange:
		return PhaseExchange
	case PhaseDiscard:
		if view.SeatIndex == view.ActingSeat {
			if cursor != nil && cursor.Mode == CursorModeSingle && cursor.Index >= 0 {
				return PhaseMyTurnSelected
			}
			return PhaseMyTurnIdle
		}
		return PhaseOtherTurn
	case PhaseQueMen:
		if view.SeatIndex == view.ActingSeat {
			return PhaseMyTurnIdle
		}
		return PhaseOtherTurn
	case PhaseWaiting:
		return PhaseWaiting
	default:
		return model.Phase
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
		case ActionGang:
			claimActions = append(claimActions, ClaimActionGang)
		case ActionPass:
			claimActions = append(claimActions, ClaimActionPass)
		}
	}
	if !containsClaimAction(claimActions, ClaimActionPass) {
		claimActions = append(claimActions, ClaimActionPass)
	}
	return &ClaimDialogState{
		Trigger:     trigger,
		TriggerSeat: view.ActingSeat,
		TriggerName: nicknameForSeat(view, view.ActingSeat),
		Tile:        view.PendingTile,
		Actions:     claimActions,
	}
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
