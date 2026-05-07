package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// drawStatusBar 渲染牌桌顶部的全局信息，减少玩家在房号、规则、剩牌与上一动作之间来回找。
func drawStatusBar(scr tcell.Screen, in FrameInputs) {
	region := in.Layout.StatusBar
	if region.Empty() {
		return
	}
	roomID := in.View.RoomID
	if roomID == "" {
		roomID = "--"
	}
	ruleID := in.View.RuleID
	if ruleID == "" {
		ruleID = "川麻血战"
	}
	dealer := seatName(in.View, in.View.DealerSeat)
	if dealer == "" {
		dealer = "--"
	}
	last := lastActionText(in.View)
	if last == "" {
		last = "等待开局"
	}
	line := fmt.Sprintf(" %s  %s  庄 %s  %s ", roomID, ruleID, dealer, last)
	drawClippedText(scr, region.X, region.Y, defaultStyle().Reverse(true), line, region.Width)
}

func lastActionText(view RoomView) string {
	for i := len(view.Log) - 1; i >= 0; i-- {
		text := strings.TrimSpace(view.Log[i].Text)
		if text != "" {
			return text
		}
	}
	if view.ActingSeat >= 0 && view.ActingSeat < 4 {
		return "等待 " + seatName(view, view.ActingSeat)
	}
	return ""
}

func seatName(view RoomView, seat int32) string {
	if seat < 0 || seat > 3 {
		return ""
	}
	player := view.Players[seat]
	if player.Nickname != "" {
		return player.Nickname
	}
	if player.UserID != "" {
		return player.UserID
	}
	return fmt.Sprintf("%d号位", seat+1)
}
