package main

import (
	"fmt"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

type TableScreenPhase string

const (
	TableScreenRoomPrep   TableScreenPhase = "room_prep"
	TableScreenPlaying    TableScreenPhase = "playing"
	TableScreenSettlement TableScreenPhase = "settlement"
	TableScreenLobby      TableScreenPhase = "lobby"
)

type ActionWindowKind string

const (
	ActionWindowNone  ActionWindowKind = ""
	ActionWindowClaim ActionWindowKind = "claim"
	ActionWindowTsumo ActionWindowKind = "tsumo"
)

type ActionWindowID struct {
	Step   int64
	Reason clientv1.WaitingReason
	Seat   int32
	Tile   string
}

type ActionWindowModel struct {
	ID        ActionWindowID
	Kind      ActionWindowKind
	Trigger   ClaimTrigger
	Title     string
	Tile      string
	Actions   []ClaimAction
	Selected  int
	Pending   bool
	Deadline  time.Time
	OpenedAt  time.Time
	HasWindow bool
}

type RelativeSeatView struct {
	AbsSeat   int32
	Position  SeatPosition
	Label     string
	Name      string
	Status    string
	Que       string
	Score     string
	Hand      []string
	HandCount int
	Melds     string
	Discards  []string
	IsSelf    bool
	IsFocus   bool
	Hued      bool
}

type TableLocalUI struct {
	Cursor             HandCursor
	ActionSelected     int
	ActionPending      bool
	ActionOpenedAt     time.Time
	OverlayOpen        bool
	SettlementRevealed bool
}

type TableFrontendModel struct {
	ScreenPhase      TableScreenPhase
	SelfSeat         int32
	FocusSeat        int32
	Seats            [4]RelativeSeatView
	SelfHand         []string
	AllowedActions   []PlayerAction
	ActionWindow     *ActionWindowModel
	Prompt           string
	KeyHint          string
	DisabledReason   string
	PendingFeedback  string
	CountdownSeconds int
	HasCountdown     bool
	RoomLabel        string
	RuleLabel        string
	WallRemaining    int32
	RoundIndex       int32
	HandIndex        int32
	Cursor           HandCursor
	Settlement       *SettlementSummary
	Events           []string
}

func BuildTableFrontendModel(view RoomView, local TableLocalUI, now time.Time) TableFrontendModel {
	m := TableFrontendModel{
		ScreenPhase:   screenPhaseFromView(view),
		SelfSeat:      view.SeatIndex,
		FocusSeat:     view.ActingSeat,
		RoomLabel:     roomLabel(view),
		RuleLabel:     ruleLabel(view),
		WallRemaining: view.WallRemaining,
		RoundIndex:    view.RoundIndex,
		HandIndex:     view.HandIndex,
		Cursor:        local.Cursor,
	}
	if view.SeatIndex >= 0 && view.SeatIndex < 4 {
		m.SelfHand = append([]string(nil), view.Players[view.SeatIndex].Hand...)
	}
	if left, ok := actionCountdown(view, now); ok {
		m.HasCountdown = true
		m.CountdownSeconds = left
	}
	m.AllowedActions = allowedActionsFromFacts(view)
	m.ActionWindow = actionWindowFromFacts(view, local, now, m.AllowedActions)
	m.PendingFeedback = pendingFeedback(view, &local.Cursor)
	m.DisabledReason = disabledReasonFromModel(view, local, m)
	m.Prompt = promptFromModel(view, local, m, now)
	m.KeyHint = keyHintFromModel(view, local, m)
	if m.ScreenPhase == TableScreenSettlement {
		m.Settlement = snapshotSettlementSummary(view)
	}
	for rel := int32(0); rel < 4; rel++ {
		seat := relativeToAbsoluteSeat(view.SeatIndex, rel)
		p := view.Players[seat]
		m.Seats[rel] = RelativeSeatView{
			AbsSeat:   seat,
			Position:  RelativeSeat(view.SeatIndex, seat),
			Label:     relativeSeatLabel(RelativeSeat(view.SeatIndex, seat), seat == view.SeatIndex),
			Name:      playerDisplayName(view, seat),
			Status:    seatStatusMark(p),
			Que:       queLabel(view.QueBySeat[seat]),
			Score:     fmt.Sprintf("%+d", p.TotalScore),
			Hand:      append([]string(nil), p.Hand...),
			HandCount: handCountForSeat(view, seat),
			Melds:     formatMeldGlyphs(p.Melds),
			Discards:  append([]string(nil), p.Discards...),
			IsSelf:    seat == view.SeatIndex,
			IsFocus:   focusOnSeat(view, &local.Cursor, seat),
			Hued:      p.Hued,
		}
	}
	for i := len(view.Log) - 1; i >= 0 && len(m.Events) < 8; i-- {
		entry := view.Log[i]
		m.Events = append(m.Events, entry.At.Format("15:04:05")+" "+entry.Text)
	}
	return m
}

func screenPhaseFromView(view RoomView) TableScreenPhase {
	if view.LastSettlement != nil || view.RoomState == "settling" {
		return TableScreenSettlement
	}
	if view.Phase == phaseLobby {
		return TableScreenLobby
	}
	if view.RoomState == "playing" {
		return TableScreenPlaying
	}
	return TableScreenRoomPrep
}

func relativeToAbsoluteSeat(selfSeat, rel int32) int32 {
	if selfSeat < 0 || selfSeat > 3 {
		return rel
	}
	return (selfSeat + rel) % 4
}

func allowedActionsFromFacts(view RoomView) []PlayerAction {
	if view.SeatIndex < 0 || view.SeatIndex > 3 {
		return nil
	}
	self := view.Players[view.SeatIndex]
	if self.Hued || self.Surrendered {
		return nil
	}
	switch view.WaitingAction {
	case "exchange_three":
		return []PlayerAction{ActionExchangeThree}
	case "que_men":
		return []PlayerAction{ActionQueMen}
	case "discard":
		if seatCanAct(view, view.SeatIndex) {
			actions := actionsFromStrings(view.AvailableActions)
			if len(actions) == 0 {
				return []PlayerAction{ActionDiscard}
			}
			return actions
		}
	case "claim_window":
		return actionsFromStrings(claimActionsForSeat(view, view.SeatIndex))
	case "tsumo_window":
		if seatCanAct(view, view.SeatIndex) || view.SeatIndex == view.ActingSeat {
			actions := actionsFromStrings(view.AvailableActions)
			if len(actions) == 0 {
				actions = []PlayerAction{ActionHu, ActionPass}
			}
			return actions
		}
	}
	return nil
}

func seatCanAct(view RoomView, seat int32) bool {
	if seat < 0 || seat > 3 {
		return false
	}
	if view.Players[seat].Hued || view.Players[seat].Surrendered {
		return false
	}
	for _, acting := range view.ActingSeats {
		if acting == seat {
			return true
		}
	}
	return view.ActingSeat == seat
}

func actionWindowFromFacts(view RoomView, local TableLocalUI, now time.Time, allowed []PlayerAction) *ActionWindowModel {
	if view.WaitingAction != "claim_window" && view.WaitingAction != "tsumo_window" {
		return nil
	}
	if len(allowed) == 0 {
		return nil
	}
	dialog := buildClaimDialog(view, allowed)
	if dialog == nil {
		return nil
	}
	selected := local.ActionSelected
	if selected < 0 || selected >= len(dialog.Actions) {
		selected = 0
	}
	openedAt := local.ActionOpenedAt
	if openedAt.IsZero() {
		openedAt = now
	}
	var deadline time.Time
	if view.DeadlineUnixMS > 0 {
		deadline = time.UnixMilli(view.DeadlineUnixMS - view.ServerClockOffsetMS)
	} else {
		deadline = openedAt.Add(14 * time.Second)
	}
	dialog.SelectedIndex = selected
	dialog.Pending = local.ActionPending
	dialog.OpenedAt = openedAt
	dialog.Deadline = deadline
	kind := ActionWindowClaim
	if view.WaitingAction == "tsumo_window" {
		kind = ActionWindowTsumo
	}
	return &ActionWindowModel{
		ID: ActionWindowID{
			Step:   view.LastStep,
			Reason: waitingReasonForRoundState(view.RoundPhase, view.WaitingAction),
			Seat:   view.ActingSeat,
			Tile:   view.PendingTile,
		},
		Kind:      kind,
		Trigger:   dialog.Trigger,
		Title:     dialog.title(),
		Tile:      dialog.Tile,
		Actions:   append([]ClaimAction(nil), dialog.Actions...),
		Selected:  selected,
		Pending:   local.ActionPending,
		Deadline:  deadline,
		OpenedAt:  openedAt,
		HasWindow: true,
	}
}

func disabledReasonFromModel(view RoomView, local TableLocalUI, model TableFrontendModel) string {
	if view.SeatIndex < 0 || view.SeatIndex > 3 {
		return "尚未入座"
	}
	if local.Cursor.Pending || local.ActionPending {
		return "正在等待服务端确认"
	}
	self := view.Players[view.SeatIndex]
	if self.Hued {
		return "你已胡牌，等待本局结束"
	}
	if self.Surrendered {
		return "你已弃局，等待本局结束"
	}
	if model.ScreenPhase == TableScreenPlaying && len(model.AllowedActions) == 0 {
		return waitingUXHint(view)
	}
	return ""
}

func promptFromModel(view RoomView, local TableLocalUI, model TableFrontendModel, now time.Time) string {
	if noticeActive(view, now) {
		return view.UXNotice
	}
	if model.PendingFeedback != "" {
		return model.PendingFeedback
	}
	if model.ActionWindow != nil {
		return "请决定：" + playerActionList(model.AllowedActions)
	}
	if model.DisabledReason != "" && len(model.AllowedActions) == 0 {
		return model.DisabledReason
	}
	switch model.ScreenPhase {
	case TableScreenRoomPrep:
		if emptySeatCount(view) > 0 {
			return "座位未满 - b 补一个 / B 补满"
		}
		return "人已坐齐：按 Enter 准备开局"
	case TableScreenSettlement:
		return "本局结束"
	}
	switch view.WaitingAction {
	case "exchange_three":
		return fmt.Sprintf("换三张：已选 %d/3，须同花色", len(local.Cursor.Marked))
	case "que_men":
		return "定缺：选一门不要，选定后不可更改"
	case "discard":
		if containsAction(model.AllowedActions, ActionDiscard) {
			return "轮到你：选择一张牌打出"
		}
		return waitingUXHint(view)
	case "claim_window", "tsumo_window":
		return waitingUXHint(view)
	default:
		if view.RoundPhase == clientv1.Phase_PHASE_DRAW {
			return waitingDrawHint(view)
		}
	}
	return "等待开始"
}

func keyHintFromModel(view RoomView, local TableLocalUI, model TableFrontendModel) string {
	if model.DisabledReason != "" && len(model.AllowedActions) == 0 {
		return model.DisabledReason + "　? 帮助"
	}
	if model.ActionWindow != nil {
		shortcuts := make([]PlayerAction, 0, len(model.AllowedActions))
		shortcuts = append(shortcuts, model.AllowedActions...)
		return claimKeyHint(TableUXModel{AllowedActions: shortcuts})
	}
	switch {
	case containsAction(model.AllowedActions, ActionExchangeThree):
		return fmt.Sprintf("换三张：已选 %d/3　←→ 选牌　Space 标记　Enter 确认", len(local.Cursor.Marked))
	case containsAction(model.AllowedActions, ActionQueMen):
		return "定缺：←→ 选择缺门　Enter 确认　m/p/s 快捷"
	case containsAction(model.AllowedActions, ActionDiscard):
		if containsAction(model.AllowedActions, ActionGang) {
			return "轮到你：←→ 选牌　Enter 打出　g 杠"
		}
		return "轮到你：←→ 选牌　Enter 打出"
	case model.ScreenPhase == TableScreenSettlement:
		return "本局结束：r 再开一桌　l 离桌　Enter 停留"
	case model.ScreenPhase == TableScreenRoomPrep:
		if emptySeatCount(view) > 0 {
			return "等人入座：b 补一个机器人　B 补满"
		}
		return "准备开局：Enter 确认　? 帮助"
	default:
		return "等待：? 帮助　i 房间信息　Tab 玩家"
	}
}
