// Package player 定义玩家标识与展示名，纯数据模型。
package player

// Player 为房间内玩家快照。
type Player struct {
	ID       string
	Nickname string
	Score    int64
}
