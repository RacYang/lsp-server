// Lobby 单元测试：创建与查询房间。
package room

import (
	"testing"

	domainroom "racoo.cn/lsp/internal/domain/room"
)

func TestLobbyCreateGet(t *testing.T) {
	l := NewLobby()
	r := domainroom.NewRoom("x")
	if err := l.CreateRoom("x", r); err != nil {
		t.Fatal(err)
	}
	got, ok := l.GetRoom("x")
	if !ok || got.ID != "x" {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}
