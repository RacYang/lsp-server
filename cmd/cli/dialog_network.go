package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

// NetStatus 是网络异常浮窗的三态。
type NetStatus int

const (
	// NetStatusOnline 正常连接，浮窗不显示。
	NetStatusOnline NetStatus = iota
	// NetStatusReconnecting 短暂断开/重连中，显示"网络异常,重连中..."。
	NetStatusReconnecting
	// NetStatusOffline 重连超过阈值（默认 30s），切换文案到"长时间无法恢复"，并提供返回大厅按钮。
	NetStatusOffline
)

// DefaultNetTimeout 是 NetOverlayState 的默认离线阈值，与计划一致。
const DefaultNetTimeout = 30 * time.Second

// NetOverlayState 描述网络异常浮窗的状态机。
//
// Since 字段记录进入 Reconnecting 的时间，after threshold 后转为 Offline。
// 字段尽量简单，便于在测试中用注入的 time.Time 走完所有分支。
type NetOverlayState struct {
	Status    NetStatus
	Since     time.Time
	Threshold time.Duration
}

// NewNetOverlay 返回阈值化默认值的状态机；threshold <= 0 时使用 DefaultNetTimeout。
func NewNetOverlay(threshold time.Duration) *NetOverlayState {
	if threshold <= 0 {
		threshold = DefaultNetTimeout
	}
	return &NetOverlayState{Status: NetStatusOnline, Threshold: threshold}
}

// Update 把连接状态推进一步。
//
// connected=true 时立即恢复 Online。
// connected=false 且当前 Online 时进入 Reconnecting，记录 Since。
// 已经 Reconnecting 且 now-Since 超阈值时进入 Offline。
func (n *NetOverlayState) Update(connected bool, now time.Time) {
	if connected {
		n.Status = NetStatusOnline
		n.Since = time.Time{}
		return
	}
	switch n.Status {
	case NetStatusOnline:
		n.Status = NetStatusReconnecting
		n.Since = now
	case NetStatusReconnecting:
		if !n.Since.IsZero() && now.Sub(n.Since) >= n.Threshold {
			n.Status = NetStatusOffline
		}
	}
}

// Visible 是否需要绘制浮窗。
func (n *NetOverlayState) Visible() bool { return n.Status != NetStatusOnline }

// title 返回浮窗标题，按状态选不同模板。
func (n *NetOverlayState) title() string {
	switch n.Status {
	case NetStatusReconnecting:
		return "网 络 异 常,重 连 中..."
	case NetStatusOffline:
		return "网 络 长 时 间 无 法 恢 复"
	}
	return ""
}

// detail 返回浮窗下方的辅助说明。
func (n *NetOverlayState) detail(now time.Time) string {
	switch n.Status {
	case NetStatusReconnecting:
		if n.Since.IsZero() {
			return "已尝试重连"
		}
		return fmt.Sprintf("已尝试 %.1fs", now.Sub(n.Since).Seconds())
	case NetStatusOffline:
		return "建议: 检查网络后按 Enter 返回大厅"
	}
	return ""
}

// DrawNetOverlay 绘制网络异常浮窗到中央区域。
func DrawNetOverlay(scr tcell.Screen, layout TableLayout, n *NetOverlayState, now time.Time) {
	if n == nil || !n.Visible() {
		return
	}
	innerWidth := minInt(40, layout.Width-6)
	if innerWidth < 28 {
		innerWidth = 28
	}
	lines := []string{
		centerVisual(n.title(), innerWidth),
		"",
		centerVisual(n.detail(now), innerWidth),
	}
	if n.Status == NetStatusOffline {
		lines = append(lines, "")
		lines = append(lines, centerVisual("[ 返回大厅 ]", innerWidth))
	}
	height := len(lines) + 2
	width := innerWidth + 2
	x := layout.CenterArea.X + (layout.CenterArea.Width-width)/2
	y := layout.CenterArea.Y + (layout.CenterArea.Height-height)/2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	style := defaultStyle()
	drawText(scr, x, y, style, "┏"+strings.Repeat("━", width-2)+"┓")
	for i, line := range lines {
		drawText(scr, x, y+1+i, style, "┃")
		drawText(scr, x+1, y+1+i, style, padRightVisual(line, innerWidth))
		drawText(scr, x+width-1, y+1+i, style, "┃")
	}
	drawText(scr, x, y+1+len(lines), style, "┗"+strings.Repeat("━", width-2)+"┛")
}
