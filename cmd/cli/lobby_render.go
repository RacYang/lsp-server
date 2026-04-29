package main

import "fmt"

func RenderLoginLines(rv RoomView, width int) []string {
	if width <= 0 {
		width = 80
	}
	return []string{
		borderLine(width, "="),
		centerText("lsp-cli 登录", width),
		borderLine(width, "-"),
		padRight("昵称: "+valueOr(rv.Nickname, "终端玩家"), width),
		padRight("服务器: "+valueOr(rv.ServerURL, "wss://racoo.cn/ws"), width),
		padRight("操作: 输入昵称和服务器后选择进入大厅", width),
		borderLine(width, "="),
	}
}

func RenderLobbyLines(rv RoomView, width int) []string {
	if width <= 0 {
		width = 96
	}
	out := []string{
		borderLine(width, "="),
		centerText(fmt.Sprintf("大厅 user=%s rooms=%d", valueOr(rv.UserID, "-"), len(rv.RoomList)), width),
		borderLine(width, "-"),
		padRight("房间       规则              玩家   阶段      名称", width),
		borderLine(width, "-"),
	}
	if len(rv.RoomList) == 0 {
		out = append(out, padRight("暂无公开房间；按 m 自动匹配，按 n 创建房间，或输入 join <room_id>", width))
	} else {
		for _, room := range rv.RoomList {
			line := fmt.Sprintf("%-10s %-17s %d/%d    %-8s %s",
				room.GetRoomId(), room.GetRuleId(), room.GetSeatCount(), room.GetMaxSeats(), room.GetStage(), room.GetDisplayName())
			out = append(out, padRight(line, width))
		}
	}
	out = append(out, borderLine(width, "="))
	return out
}
