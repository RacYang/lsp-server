// 玩家模型占位测试，后续可扩展积分与断线重连语义。
package player

import "testing"

func TestPlayerStruct(t *testing.T) {
	p := Player{ID: "u1", Nickname: "测试", Score: 0}
	if p.ID != "u1" {
		t.Fatal("unexpected")
	}
}
