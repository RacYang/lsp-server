package main

import "github.com/gdamore/tcell/v2"

// drawKeybar 渲染底部按键提示。B0 阶段不展示 q 走人，避免服务端离房未完成前误导玩家。
func drawKeybar(scr tcell.Screen, in FrameInputs) {
	region := in.Layout.KeyBar
	if region.Empty() {
		return
	}
	hint := bottomHint(in.View, in.Cursor)
	if hint == "" {
		hint = "Esc 菜单    ? 帮助"
	}
	drawClippedText(scr, region.X, region.Y, defaultStyle().Reverse(true), centerVisual(hint, region.Width), region.Width)
}
