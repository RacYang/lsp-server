package main

// playerSeated 判定一个座位是否已经有玩家入座。
//
// 协议事实：服务端的 JoinRoomResp.Seats 只在自家那一格回填 nickname / user_id，
// 其他三格即便已被真人或机器人占用也是空 SeatInfo。所以这里只把 UserID/Nickname
// 当 hint，真正"是否有玩家"还要结合手牌、鸣牌、弃牌或"游戏已开局"信号。
func playerSeated(p PlayerView) bool {
	if p.UserID != "" || p.Nickname != "" {
		return true
	}
	return p.HandCnt > 0 || len(p.Hand) > 0 || len(p.Melds) > 0 || len(p.Discards) > 0 || p.Hued
}

// seatLabelFallback 给一个还没回传 Nickname 但已经在打牌的座位一个可读名。
//
// 用绝对座位编号 +1 作为"N号位"，与 statusbar.seatName 的兜底一致。
func seatLabelFallback(seat int32) string {
	if seat < 0 || seat > 3 {
		return "玩家"
	}
	return []string{"1号位", "2号位", "3号位", "4号位"}[seat]
}

// defaultStartingHandSize 是开局后但尚未跟踪过 HandCnt 时的兜底牌数（13 张）。
//
// 真实牌数会随服务端 DrawTile/DiscardAction 维护；客户端目前还没有完整的
// "对家手牌增减" 路径，所以在 gameStarted 时遇到 HandCnt==0 的对家用 13 占位。
const defaultStartingHandSize = 13
