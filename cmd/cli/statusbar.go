package main

import (
	"fmt"
	"strings"
)

// lastPlayerVisibleEvent 取最近一条对玩家有意义的事件文本。
//
// 协议握手类（"已入座 0"、"准备成功"、"客户端已启动"等）会被过滤掉，避免顶部
// HUD 长时间显示无关噪声。若没有可见事件，返回空字符串让上层回落到 "等待开局"。
func lastPlayerVisibleEvent(view RoomView) string {
	for i := len(view.Log) - 1; i >= 0; i-- {
		text := strings.TrimSpace(view.Log[i].Text)
		if text == "" {
			continue
		}
		if isInternalLogNoise(text) {
			continue
		}
		return text
	}
	if view.ActingSeat >= 0 && view.ActingSeat < 4 && gameStarted(view) {
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

// internalLogNoisePrefixes 列出对玩家无意义、仅作调试痕迹的日志前缀。
//
// 这里只用前缀匹配，避免误伤"等待 xx 出牌"这类合法事件；如需扩展，请保持
// 列表精简，并在 history sidebar 与顶部 HUD 共享同一过滤集。
var internalLogNoisePrefixes = []string{
	"已入座",
	"准备成功",
	"准备失败",
	"客户端已启动",
	"已返回大厅",
	"离房失败",
	"离房成功",
	"收到路由重定向",
	"收到开局手牌",
	"自动匹配",
	"创建房间",
	"加入房间",
}

func isInternalLogNoise(text string) bool {
	for _, p := range internalLogNoisePrefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}

// visibleLogEntries 取出最近 limit 条对玩家可见的日志（按时间正序返回）。
//
// 牌桌主界面不再常驻历史栏，但房间信息浮层和测试仍共用这段过滤逻辑。
func visibleLogEntries(log []LogEntry, limit int) []string {
	if limit <= 0 || len(log) == 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for i := len(log) - 1; i >= 0 && len(out) < limit; i-- {
		text := strings.TrimSpace(log[i].Text)
		if text == "" || isInternalLogNoise(text) {
			continue
		}
		out = append(out, text)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func phaseLabel(phase TablePhase) string {
	switch phase {
	case PhaseWaiting:
		return "等待开局"
	case PhaseExchange:
		return "换三张"
	case PhaseQueMen:
		return "定缺"
	case PhaseDiscard:
		return "出牌"
	case PhaseMyTurnIdle, PhaseMyTurnSelected:
		return "你的回合"
	case PhaseOtherTurn:
		return "等待他家"
	case PhaseClaim, PhaseTsumo:
		return "鸣牌"
	case PhaseSettlement:
		return "结算"
	default:
		return "牌桌"
	}
}
