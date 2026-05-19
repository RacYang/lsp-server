package main

import "github.com/gdamore/tcell/v2"

type TableCommandKind string

const (
	TableCommandNone          TableCommandKind = ""
	TableCommandReady         TableCommandKind = "ready"
	TableCommandDiscard       TableCommandKind = "discard"
	TableCommandExchangeThree TableCommandKind = "exchange_three"
	TableCommandQueMen        TableCommandKind = "que_men"
	TableCommandClaim         TableCommandKind = "claim"
	TableCommandAddBot        TableCommandKind = "add_bot"
)

type TableCommand struct {
	Kind        TableCommandKind
	Tile        string
	Tiles       []string
	Suit        int32
	ClaimAction ClaimAction
	BotCount    int32
	Reject      string
}

func HandleTableInput(model TableFrontendModel, local TableLocalUI, ev *tcell.EventKey) TableCommand {
	if ev == nil {
		return TableCommand{}
	}
	if local.Cursor.Pending || local.ActionPending {
		return TableCommand{Reject: "正在等待服务端确认"}
	}
	switch ev.Key() {
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		if model.ActionWindow != nil && model.ActionWindow.Selected >= 0 && model.ActionWindow.Selected < len(model.ActionWindow.Actions) {
			return TableCommand{Kind: TableCommandClaim, ClaimAction: model.ActionWindow.Actions[model.ActionWindow.Selected]}
		}
		if containsAction(model.AllowedActions, ActionDiscard) && local.Cursor.Index >= 0 && local.Cursor.Index < len(model.SelfHand) {
			return TableCommand{Kind: TableCommandDiscard, Tile: model.SelfHand[local.Cursor.Index]}
		}
		if containsAction(model.AllowedActions, ActionQueMen) && local.Cursor.Index >= 0 && local.Cursor.Index < 3 {
			return TableCommand{Kind: TableCommandQueMen, Suit: int32(local.Cursor.Index)}
		}
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'h', 'H':
			return claimShortcutCommand(model, ActionHu, ClaimActionHu, "当前不能胡")
		case 'g', 'G':
			return claimShortcutCommand(model, ActionGang, ClaimActionGang, "当前不能杠")
		case 'p', 'P':
			return claimShortcutCommand(model, ActionPong, ClaimActionPong, "当前不能碰")
		case 'c', 'C':
			return claimShortcutCommand(model, ActionChi, ClaimActionChow, "当前不能吃")
		case 'n', 'N':
			return claimShortcutCommand(model, ActionPass, ClaimActionPass, "当前不能过")
		case 'm', 'M':
			if containsAction(model.AllowedActions, ActionQueMen) {
				return TableCommand{Kind: TableCommandQueMen, Suit: 0}
			}
		case 's', 'S':
			if containsAction(model.AllowedActions, ActionQueMen) {
				return TableCommand{Kind: TableCommandQueMen, Suit: 2}
			}
		}
	}
	if model.DisabledReason != "" {
		return TableCommand{Reject: model.DisabledReason}
	}
	return TableCommand{}
}

func claimShortcutCommand(model TableFrontendModel, required PlayerAction, action ClaimAction, reject string) TableCommand {
	if model.ActionWindow == nil || !containsAction(model.AllowedActions, required) {
		return TableCommand{Reject: reject}
	}
	return TableCommand{Kind: TableCommandClaim, ClaimAction: action}
}
