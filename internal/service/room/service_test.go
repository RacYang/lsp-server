// 房间服务单元测试：加入、准备与广播触发。
package room

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
		notifications, err := svc.Ready(ctx, rid, u)
		if err != nil {
			t.Fatalf("ready %s: %v", u, err)
		}
		for _, notification := range notifications {
			if notification.Kind == KindSettlement {
				f.Broadcast(rid, msgid.Settlement, notification.Payload)
			}
		}
	}
	if f.n == 0 {
		t.Fatal("expected broadcast")
	}
	if f.lastRoom != rid || f.lastMsg != msgid.Settlement {
		t.Fatalf("unexpected broadcast room=%s msg=%d", f.lastRoom, f.lastMsg)
	}
}

func TestEnsureRoomConcurrentFirstJoin(t *testing.T) {
	t.Parallel()

	svc := NewService(NewLobby())
	ctx := context.Background()
	const roomID = "room-race"

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, uid := range []string{"u1", "u2"} {
		wg.Add(1)
		go func(userID string) {
			defer wg.Done()
			_, err := svc.Join(ctx, roomID, userID)
			results <- err
		}(uid)
	}
	wg.Wait()
	close(results)

	for err := range results {
		require.NoError(t, err)
	}
}

func TestActorRemovedAfterRoomClosed(t *testing.T) {
	t.Parallel()

	svc := NewService(NewLobby())
	ctx := context.Background()
	const roomID = "room-close"
	for _, uid := range []string{"p0", "p1", "p2", "p3"} {
		_, err := svc.Join(ctx, roomID, uid)
		require.NoError(t, err)
	}
	for _, uid := range []string{"p0", "p1", "p2", "p3"} {
		_, err := svc.Ready(ctx, roomID, uid)
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool {
		return svc.getActor(roomID) == nil
	}, time.Second, 10*time.Millisecond)
}

func TestRecoverRoomAndRuleID(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithRule(NewLobby(), "sichuan_xzdd")
	err := svc.RecoverRoom("room-recover", []string{"u1", "u2", "u3", "u4"}, "ready")
	require.NoError(t, err)

	players, state, ok := svc.RoomSnapshot("room-recover")
	require.True(t, ok)
	require.Equal(t, "ready", state)
	require.ElementsMatch(t, []string{"u1", "u2", "u3", "u4"}, players)
	require.Equal(t, "sichuan_xzdd", svc.RuleID())
}
