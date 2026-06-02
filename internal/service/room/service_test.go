// 房间服务单元测试：加入、准备与广播触发。
package room

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/clock"
	_ "racoo.cn/lsp/internal/mahjong/guobiao/jingji"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
	"racoo.cn/lsp/internal/protocol"
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
	l := NewRoomRegistry()
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
			if id, ok := outboundTestMsgID(notification.Kind); ok {
				f.Broadcast(rid, id, notification.Payload)
			}
		}
	}
	if f.n == 0 {
		t.Fatal("expected broadcast")
	}
	if f.lastRoom != rid {
		t.Fatalf("unexpected broadcast room=%s msg=%d", f.lastRoom, f.lastMsg)
	}
	if f.lastMsg != protocol.DrawTile && f.lastMsg != protocol.ActionNotify && f.lastMsg != protocol.StartGame {
		t.Fatalf("unexpected broadcast room=%s msg=%d", f.lastRoom, f.lastMsg)
	}
}

func TestJoinRejectsAfterRoundStarted(t *testing.T) {
	svc := NewService(NewRoomRegistry())
	ctx := context.Background()
	const roomID = "room-started"
	for _, uid := range []string{"p0", "p1", "p2", "p3"} {
		_, err := svc.Join(ctx, roomID, uid)
		require.NoError(t, err)
	}
	for _, uid := range []string{"p0", "p1", "p2", "p3"} {
		_, err := svc.Ready(ctx, roomID, uid)
		require.NoError(t, err)
	}

	_, err := svc.Join(ctx, roomID, "late-player")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already started")

	seat, err := svc.Join(ctx, roomID, "p0")
	require.NoError(t, err)
	require.Zero(t, seat)
}

func TestEnsureRoomConcurrentFirstJoin(t *testing.T) {
	t.Parallel()

	svc := NewService(NewRoomRegistry())
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

	svc := NewService(NewRoomRegistry())
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
	require.NoError(t, driveRoundToClose(ctx, svc, roomID))
	require.Eventually(t, func() bool {
		return svc.getActor(roomID) == nil
	}, time.Second, 10*time.Millisecond)
}

func TestRecoverRoomAndRuleID(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	err := svc.RecoverRoom("room-recover", []string{"u1", "u2", "u3", "u4"}, "ready", nil)
	require.NoError(t, err)

	players, state, _, ok := svc.RoomSnapshot("room-recover")
	require.True(t, ok)
	require.Equal(t, "ready", state)
	require.ElementsMatch(t, []string{"u1", "u2", "u3", "u4"}, players)
	require.Equal(t, "sichuan_xuezhandaodi_huansanzhang", svc.RuleID())
}

func TestRecoverRoomPlayingRequiresRoundSnapshot(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	err := svc.RecoverRoom("room-playing-missing", []string{"u1", "u2", "u3", "u4"}, "playing", nil)
	require.Error(t, err)
}

func TestRecoverRoomFSMStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		fsmInput   string
		wantState  string
		expectFail bool
	}{
		{name: "empty becomes waiting", fsmInput: "", wantState: "waiting"},
		{name: "explicit waiting", fsmInput: "waiting", wantState: "waiting"},
		{name: "ready", fsmInput: "ready", wantState: "ready"},
		{name: "settling", fsmInput: "settling", wantState: "settling"},
		{name: "closed", fsmInput: "closed", wantState: "closed"},
		{name: "unknown rejected", fsmInput: "garbage", expectFail: true},
		{name: "idle rejected", fsmInput: "idle", expectFail: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
			roomID := "room-recover-" + tc.fsmInput
			if roomID == "room-recover-" {
				roomID = "room-recover-empty"
			}
			err := svc.RecoverRoom(roomID, []string{"u1", "u2", "u3", "u4"}, tc.fsmInput, nil)
			if tc.expectFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			_, state, _, ok := svc.RoomSnapshot(roomID)
			require.True(t, ok)
			require.Equal(t, tc.wantState, state)
		})
	}
}

func TestRecoverRoomReadyDoesNotChainTransitions(t *testing.T) {
	// 旧实现走 Transition(StateReady) 链式爬升；新实现走 FSM.Restore 一次性置位。
	// 关键差异：从 ready 直接迁到 settling 在普通 transition 下非法（ready→playing→settling），
	// 但通过 RecoverRoom("settling", ...) 应当成功一次性置位。
	t.Parallel()

	svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	err := svc.RecoverRoom("room-direct-settling", []string{"u1", "u2", "u3", "u4"}, "settling", nil)
	require.NoError(t, err)
	_, state, _, ok := svc.RoomSnapshot("room-direct-settling")
	require.True(t, ok)
	require.Equal(t, "settling", state)
}

func TestRecoverRoomIdempotentForExistingRoom(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	const rid = "room-recover-idem"
	require.NoError(t, svc.RecoverRoom(rid, []string{"u1", "u2", "u3", "u4"}, "ready", nil))

	// 第二次调用应当复用 lobby 中已有 room，不返回错误也不重置 FSM。
	require.NoError(t, svc.RecoverRoom(rid, []string{"u1", "u2", "u3", "u4"}, "playing", nil))

	_, state, _, ok := svc.RoomSnapshot(rid)
	require.True(t, ok)
	require.Equal(t, "ready", state, "已存在房间不应被第二次 RecoverRoom 改写")
}

func TestServiceAutoTimeoutSurrendersThroughActor(t *testing.T) {
	t.Parallel()

	rs := NewRoundStateFromConfig(RoundStateConfig{
		RoomID:          "r-service-timeout",
		RuleID:          "sichuan_xuezhandaodi_huansanzhang",
		Rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		PlayerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		Wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		Hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1)}), hand.New(), hand.New(), hand.New()},
		RuleState:       testRuleState(make([]int32, 4)),
		WaitingDiscard:  true,
		Turn:            0,
		LastDiscardSeat: -1,
	})
	data, err := rs.MarshalRoundPersistJSON()
	require.NoError(t, err)

	svc := NewService(NewRoomRegistry())
	require.NoError(t, svc.RecoverRoom("r-service-timeout", []string{"u0", "u1", "u2", "u3"}, "playing", data))
	notifs, err := svc.AutoTimeout(context.Background(), "r-service-timeout")
	require.NoError(t, err)
	// 摸牌展开为 4 条（每座位一条）
	require.Len(t, notifs, 4)
	a := svc.getActor("r-service-timeout")
	require.NotNil(t, a)
	require.True(t, a.Room.Surrendered[0])
}

func TestSchedulerAutoTimeoutUsesFakeClock(t *testing.T) {
	fc := clock.NewFake(time.Unix(0, 0))
	svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	svc.SetClock(fc)
	svc.SetTimeoutConfig(TimeoutConfig{
		OpeningDefault: time.Second,
		OpeningByAction: map[string]time.Duration{
			openingExchangeThree: time.Second,
			openingQueMen:        time.Second,
		},
		ClaimWindow: time.Second,
		TsumoWindow: time.Second,
		Discard:     time.Second,
	})
	got := make(chan []Notification, 1)
	svc.SetAutoTimeoutHandler(func(_ context.Context, roomID string, notifications []Notification) {
		if roomID == "r-scheduler" {
			got <- notifications
		}
	})
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, err := svc.Join(context.Background(), "r-scheduler", uid)
		require.NoError(t, err)
	}
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, err := svc.Ready(context.Background(), "r-scheduler", uid)
		require.NoError(t, err)
	}

	for i := 0; i < 4; i++ {
		fc.Advance(time.Second)
	}
	require.Eventually(t, func() bool {
		select {
		case notifications := <-got:
			return len(notifications) > 0
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestServiceMailboxCapacityOverride(t *testing.T) {
	t.Parallel()

	svc := NewService(NewRoomRegistry())
	svc.SetMailboxCapacity(3)
	require.NoError(t, svc.EnsureRoom("mailbox-config-room"))
	a := svc.getActor("mailbox-config-room")
	require.NotNil(t, a)
	require.Equal(t, 3, a.MailboxCap())
}

func TestServiceLeaveDuringPlayMarksSurrender(t *testing.T) {
	ctx := context.Background()
	svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	roomID := "r-leave-playing"
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, err := svc.Join(ctx, roomID, uid)
		require.NoError(t, err)
	}
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, err := svc.Ready(ctx, roomID, uid)
		require.NoError(t, err)
	}
	require.NoError(t, svc.Leave(ctx, roomID, "u2"))
	a := svc.getActor(roomID)
	require.NotNil(t, a)
	require.True(t, a.Room.Surrendered[2])
	require.Equal(t, "u2", a.Room.PlayerIDs[2])
}

func TestServiceLeaveDuringPlayCanBeDisabled(t *testing.T) {
	ctx := context.Background()
	svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	svc.SetAllowLeaveDuringPlay(false)
	roomID := "r-leave-disabled"
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, err := svc.Join(ctx, roomID, uid)
		require.NoError(t, err)
	}
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, err := svc.Ready(ctx, roomID, uid)
		require.NoError(t, err)
	}
	require.Error(t, svc.Leave(ctx, roomID, "u2"))
}

func outboundTestMsgID(kind Kind) (uint16, bool) {
	switch kind {
	case KindOpeningDone:
		return protocol.OpeningDone, true
	case KindStartGame:
		return protocol.StartGame, true
	case KindDrawTile:
		return protocol.DrawTile, true
	case KindAction:
		return protocol.ActionNotify, true
	case KindSettlement:
		return protocol.Settlement, true
	default:
		return 0, false
	}
}

// driveRoundToClose 通过公开 API（RoundView + Service 命令）驱动牌局直至结算，
// 不依赖引擎内部字段，适用于白盒服务层测试。
func driveRoundToClose(ctx context.Context, svc *Service, roomID string) error {
	for i := 0; i < 512; i++ {
		a := svc.getActor(roomID)
		if a == nil {
			return nil
		}
		view, ok, err := svc.RoundView(ctx, roomID)
		if err != nil || !ok {
			return err
		}
		actingSeat := int(view.ActingSeat)
		var userID string
		if actingSeat >= 0 && actingSeat < len(a.Room.PlayerIDs) {
			userID = a.Room.PlayerIDs[actingSeat]
		}
		switch view.WaitingAction {
		case "":
			return nil
		case openingExchangeThree:
			for _, seatIdx := range view.ActingSeats {
				seat := int(seatIdx)
				uid := a.Room.PlayerIDs[seat]
				hand := view.HandsBySeat[seat]
				if len(hand) > 3 {
					hand = hand[:3]
				}
				if _, err := svc.OpeningAction(ctx, roomID, uid, openingExchangeThree, hand, 0, 0, nil, nil); err != nil {
					return err
				}
			}
		case openingQueMen:
			for _, seatIdx := range view.ActingSeats {
				uid := a.Room.PlayerIDs[seatIdx]
				if _, err := svc.OpeningAction(ctx, roomID, uid, openingQueMen, nil, 0, 0, nil, nil); err != nil {
					return err
				}
			}
		case "claim_window":
			if len(view.ClaimCandidates) > 0 {
				best := view.ClaimCandidates[0]
				uid := a.Room.PlayerIDs[best.Seat]
				if len(best.Actions) > 0 {
					switch best.Actions[0] {
					case "gang":
						if _, e := svc.Gang(ctx, roomID, uid, view.PendingTile, nil); e != nil && !roundClosedErr(e) {
							return e
						}
					case "pong":
						if _, e := svc.Pong(ctx, roomID, uid, nil); e != nil && !roundClosedErr(e) {
							return e
						}
					case "hu":
						if notifs, e := svc.Hu(ctx, roomID, uid, nil); e == nil && containsSettlement(notifs) {
							return nil
						}
					}
				}
			} else if userID != "" {
				if _, e := svc.Pass(ctx, roomID, userID, nil); e != nil && !roundClosedErr(e) {
					return e
				}
			}
		case "tsumo_window":
			if userID != "" {
				notifs, e := svc.Hu(ctx, roomID, userID, nil)
				if e == nil && containsSettlement(notifs) {
					return nil
				}
				if e == nil {
					continue
				}
				if !strings.Contains(e.Error(), "hu not allowed") {
					return e
				}
				// 无法胡牌则随意出牌
				tiles := view.HandsBySeat[actingSeat]
				if len(tiles) > 0 {
					if notifs, e2 := svc.Discard(ctx, roomID, userID, tiles[0], nil); e2 == nil && containsSettlement(notifs) {
						return nil
					}
				}
			}
		case "discard":
			if userID != "" {
				tiles := view.HandsBySeat[actingSeat]
				if len(tiles) == 0 {
					continue
				}
				notifs, e := svc.Discard(ctx, roomID, userID, tiles[0], nil)
				if e != nil {
					if roundClosedErr(e) {
						return nil
					}
					return e
				}
				if containsSettlement(notifs) {
					return nil
				}
			}
		default:
			return fmt.Errorf("不支持的等待动作: %s", view.WaitingAction)
		}
	}
	return context.DeadlineExceeded
}

func roundClosedErr(err error) bool {
	return err != nil && bytes.Contains([]byte(err.Error()), []byte("round closed"))
}

func containsSettlement(notifications []Notification) bool {
	for _, notification := range notifications {
		if notification.Kind == KindSettlement {
			return true
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(notification.Payload, &env); err == nil && env.GetSettlement() != nil {
			return true
		}
	}
	return false
}
