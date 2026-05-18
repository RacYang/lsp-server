package main

import (
	"fmt"
	"time"
)

// NetStatus 是网络异常浮窗的三态。
type NetStatus int

const (
	NetStatusOnline NetStatus = iota
	NetStatusReconnecting
	NetStatusOffline
)

// DefaultNetTimeout 是 NetOverlayState 的默认离线阈值。
const DefaultNetTimeout = 30 * time.Second

// NetOverlayState 描述网络异常浮窗的状态机。
type NetOverlayState struct {
	Status    NetStatus
	Since     time.Time
	Threshold time.Duration
}

// NewNetOverlay 返回阈值化默认值的状态机。
func NewNetOverlay(threshold time.Duration) *NetOverlayState {
	if threshold <= 0 {
		threshold = DefaultNetTimeout
	}
	return &NetOverlayState{Status: NetStatusOnline, Threshold: threshold}
}

// Update 把连接状态推进一步。
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

// Title 返回浮窗标题。
func (n *NetOverlayState) Title() string {
	switch n.Status {
	case NetStatusReconnecting:
		return "网 络 异 常,重 连 中..."
	case NetStatusOffline:
		return "网 络 长 时 间 无 法 恢 复"
	}
	return ""
}

// Detail 返回浮窗辅助说明。
func (n *NetOverlayState) Detail(now time.Time) string {
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
