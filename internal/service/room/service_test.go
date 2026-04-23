// 房间服务单元测试：加入、准备与广播触发。
package room

import (
	"context"
	"testing"

	"racoo.cn/lsp/internal/net/msgid"
)

type fakeBC struct {
	lastRoom string
	lastMsg  uint16
	n        int
}

func (f *fakeBC) Broadcast(roomID string, msgID uint16, payload []byte) {
	f.lastRoom = roomID
	f.lastMsg = msgID
	f.n++
}

func TestReadyTriggersBroadcast(t *testing.T) {
	l := NewLobby()
	f := &fakeBC{}
	svc := NewService(l)
	ctx := context.Background()
	const rid = "room-a"
	uids := []string{"p0", "p1", "p2", "p3"}
	for _, u := range uids {
		if _, err := svc.Join(ctx, rid, u); err != nil {
			t.Fatalf("join %s: %v", u, err)
		}
	}
	for _, u := range uids {
		payload, err := svc.Ready(ctx, rid, u)
		if err != nil {
			t.Fatalf("ready %s: %v", u, err)
		}
		if len(payload) > 0 {
			f.Broadcast(rid, msgid.Settlement, payload)
		}
	}
	if f.n == 0 {
		t.Fatal("expected broadcast")
	}
	if f.lastRoom != rid || f.lastMsg != msgid.Settlement {
		t.Fatalf("unexpected broadcast room=%s msg=%d", f.lastRoom, f.lastMsg)
	}
}
