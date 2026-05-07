package main

import (
	"github.com/gdamore/tcell/v2"
)

// drawCenterInfo 渲染牌桌中央信息盒，当前只展示玩家动作提示与可用动作。
func drawCenterInfo(scr tcell.Screen, in FrameInputs) {
	region := in.Layout.CenterArea
	if region.Empty() {
		return
	}
	boxWidth := region.Width / 3
	if in.Layout.Wide {
		boxWidth = region.Width / 4
	}
	if boxWidth < 18 {
		boxWidth = 18
	}
	x := region.X + (region.Width-boxWidth)/2
	y := region.Y + renderMaxInt(0, (region.Height-4)/2)
	drawBox(scr, Region{X: x, Y: y, Width: boxWidth, Height: renderMinInt(4, region.Height)}, "牌桌信息")
	prompt := centralPrompt(in.View, in.Cursor)
	if prompt == "" {
		prompt = "等待开始"
	}
	style := defaultStyle()
	if in.View.SeatIndex >= 0 && in.View.SeatIndex == in.View.ActingSeat {
		style = style.Reverse(true)
	}
	drawClippedText(scr, x+2, y+1, style, prompt, boxWidth-4)
	action := formatAvailableActions(in.View.AvailableActions)
	if action == "" {
		action = "动作: --"
	}
	drawClippedText(scr, x+2, y+2, defaultStyle(), action, boxWidth-4)
}

func formatAvailableActions(actions []string) string {
	if len(actions) == 0 {
		return ""
	}
	return "动作: " + joinLimited(actions, 4)
}
