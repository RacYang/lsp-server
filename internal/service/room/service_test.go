// 房间服务单元测试：加入、准备与广播触发。
package room

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/clock"
	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
	"racoo.cn/lsp/internal/net/msgid"
)

type fakeBC struct {
	lastRoom string
	lastMsg  uint16
	n        int
}

func TestRestoreRoundRejectsFutureSchema(t *testing.T) {
	_, err := RestoreRoundFromPersistJSON("room-future-schema", []byte(`{"schema_version":999}`))
	require.ErrorIs(t, err, ErrRoundPersistUnsupportedSchema)
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
	if f.lastMsg != msgid.DrawTile && f.lastMsg != msgid.ActionNotify && f.lastMsg != msgid.StartGame {
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
	require.NoError(t, driveRoundToClose(ctx, svc, roomID))
	require.Eventually(t, func() bool {
		return svc.getActor(roomID) == nil
	}, time.Second, 10*time.Millisecond)
}

func TestRecoverRoomAndRuleID(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithRule(NewLobby(), "sichuan_xzdd")
	err := svc.RecoverRoom("room-recover", []string{"u1", "u2", "u3", "u4"}, "ready", nil)
	require.NoError(t, err)

	players, state, ok := svc.RoomSnapshot("room-recover")
	require.True(t, ok)
	require.Equal(t, "ready", state)
	require.ElementsMatch(t, []string{"u1", "u2", "u3", "u4"}, players)
	require.Equal(t, "sichuan_xzdd", svc.RuleID())
}

func TestRecoverRoomPlayingRequiresRoundSnapshot(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithRule(NewLobby(), "sichuan_xzdd")
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

			svc := NewServiceWithRule(NewLobby(), "sichuan_xzdd")
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
			_, state, ok := svc.RoomSnapshot(roomID)
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

	svc := NewServiceWithRule(NewLobby(), "sichuan_xzdd")
	err := svc.RecoverRoom("room-direct-settling", []string{"u1", "u2", "u3", "u4"}, "settling", nil)
	require.NoError(t, err)
	_, state, ok := svc.RoomSnapshot("room-direct-settling")
	require.True(t, ok)
	require.Equal(t, "settling", state)
}

func TestRecoverRoomIdempotentForExistingRoom(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithRule(NewLobby(), "sichuan_xzdd")
	const rid = "room-recover-idem"
	require.NoError(t, svc.RecoverRoom(rid, []string{"u1", "u2", "u3", "u4"}, "ready", nil))

	// 第二次调用应当复用 lobby 中已有 room，不返回错误也不重置 FSM。
	require.NoError(t, svc.RecoverRoom(rid, []string{"u1", "u2", "u3", "u4"}, "playing", nil))

	_, state, ok := svc.RoomSnapshot(rid)
	require.True(t, ok)
	require.Equal(t, "ready", state, "已存在房间不应被第二次 RecoverRoom 改写")
}

func TestRoundViewShowsClaimWindow(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-claim",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 7)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitDots, 9)}), hand.New()},
		queBySeat:       make([]int32, 4),
		waitingDiscard:  true,
		turn:            1,
		currentDraw:     tile.Must(tile.SuitDots, 7),
		lastDiscard:     tile.Must(tile.SuitCharacters, 3),
		lastDiscardSeat: 0,
	}
	rs.openClaimWindow()

	view := rs.SnapshotView()
	require.EqualValues(t, 2, view.ActingSeat)
	require.Equal(t, "claim", view.WaitingAction)
	require.Equal(t, "m3", view.PendingTile)
	require.Equal(t, []string{"pong"}, view.AvailableActions)
}

func TestExchangeThreeUsesClientDirection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		direction int32
		wantSeat0 []tile.Tile
	}{
		{name: "clockwise", direction: 1, wantSeat0: []tile.Tile{tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 3)}},
		{name: "opposite", direction: 2, wantSeat0: []tile.Tile{tile.Must(tile.SuitBamboo, 1), tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitBamboo, 3)}},
		{name: "counterclockwise", direction: 3, wantSeat0: []tile.Tile{tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 5), tile.Must(tile.SuitCharacters, 6)}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rs := roundWaitingExchange()
			e := NewEngine("sichuan_xzdd")
			for seat := 0; seat < 4; seat++ {
				_, err := e.ApplyExchangeThree(context.Background(), rs, seat, tilesToStrings(rs.hands[seat].Tiles()), tt.direction)
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantSeat0, rs.hands[0].Tiles())
		})
	}
}

func TestExchangeThreeRejectsMismatchedDirection(t *testing.T) {
	t.Parallel()

	rs := roundWaitingExchange()
	e := NewEngine("sichuan_xzdd")
	_, err := e.ApplyExchangeThree(context.Background(), rs, 0, tilesToStrings(rs.hands[0].Tiles()), 1)
	require.NoError(t, err)
	_, err = e.ApplyExchangeThree(context.Background(), rs, 1, tilesToStrings(rs.hands[1].Tiles()), 2)
	require.ErrorContains(t, err, "exchange direction mismatch")
}

func TestQueMenUsesClientSuit(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-que",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3)}), hand.New(), hand.New(), hand.New()},
		queBySeat:       make([]int32, 4),
		queSubmitted:    make([]bool, 4),
		waitingQueMen:   true,
		lastDiscardSeat: -1,
	}
	e := NewEngine("sichuan_xzdd")
	_, err := e.ApplyQueMen(context.Background(), rs, 0, int32(tile.SuitBamboo))
	require.NoError(t, err)
	require.EqualValues(t, tile.SuitBamboo, rs.queBySeat[0])
}

func TestApplyPongInterruptsPendingTurn(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-pong",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 7)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitDots, 9)}), hand.New()},
		queBySeat:       make([]int32, 4),
		waitingDiscard:  true,
		turn:            1,
		currentDraw:     tile.Must(tile.SuitDots, 7),
		lastDiscard:     tile.Must(tile.SuitCharacters, 3),
		lastDiscardSeat: 0,
	}
	rs.openClaimWindow()

	e := NewEngine("sichuan_xzdd")
	notifs, err := e.ApplyPong(context.Background(), rs, 2)
	require.NoError(t, err)
	require.Len(t, notifs, 3)
	require.Equal(t, 3, rs.turn)
	require.True(t, rs.waitingDiscard)
	require.Equal(t, tile.Must(tile.SuitDots, 7), rs.currentDraw)
	require.Equal(t, []tile.Tile{tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 2)}, rs.hands[1].Tiles())
	require.Empty(t, rs.hands[2].Tiles())
}

func roundWaitingExchange() *RoundState {
	return &RoundState{
		roomID:            "r-exchange",
		ruleID:            "sichuan_xzdd",
		rule:              rules.MustGet("sichuan_xzdd"),
		playerIDs:         [4]string{"u0", "u1", "u2", "u3"},
		hands:             []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitBamboo, 1), tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitBamboo, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 5), tile.Must(tile.SuitCharacters, 6)})},
		queBySeat:         make([]int32, 4),
		waitingExchange:   true,
		exchangeSubmitted: make([]bool, 4),
		exchangeDirection: -1,
		exchangeSelection: make([][]tile.Tile, 4),
		queSubmitted:      make([]bool, 4),
		lastDiscardSeat:   -1,
	}
}

func TestApplyDiscardPromptsClaimInsteadOfNextDraw(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-claim-prompt",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3)}), hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.New()},
		queBySeat:       make([]int32, 4),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}

	e := NewEngine("sichuan_xzdd")
	notifs, err := e.ApplyDiscard(context.Background(), rs, 0, "m3")
	require.NoError(t, err)
	require.Len(t, notifs, 2)

	var sawClaim bool
	for _, notification := range notifs {
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		if action := env.GetAction(); action != nil && action.GetAction() == "pong_choice" {
			sawClaim = true
			require.EqualValues(t, 2, action.GetSeatIndex())
		}
		if env.GetDrawTile() != nil {
			t.Fatal("claim window should not broadcast next draw")
		}
	}
	require.True(t, sawClaim)
}

func TestApplyDiscardPromptsMultipleClaimCandidates(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-multi-claim",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)})},
		queBySeat:       make([]int32, 4),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}

	e := NewEngine("sichuan_xzdd")
	notifs, err := e.ApplyDiscard(context.Background(), rs, 0, "m3")
	require.NoError(t, err)
	require.Len(t, notifs, 4)
	require.True(t, rs.claimWindowOpen)
	require.Len(t, rs.claimCandidates, 3)

	claimBySeat := map[int32]string{}
	for _, notification := range notifs {
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		if action := env.GetAction(); action != nil && action.GetAction() != "discard" {
			claimBySeat[action.GetSeatIndex()] = action.GetAction()
		}
	}
	require.Equal(t, map[int32]string{1: "pong_choice", 2: "gang_choice", 3: "pong_choice"}, claimBySeat)
	_, err = e.ApplyPong(context.Background(), rs, 1)
	require.Error(t, err)

	notifs, err = e.ApplyGang(context.Background(), rs, 2, "m3")
	require.NoError(t, err)
	require.False(t, rs.claimWindowOpen)
	require.Len(t, notifs, 2)
	require.Equal(t, 2, rs.turn)
}

func TestClaimWindowPersistsAndRestores(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-claim-persist",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.New()},
		queBySeat:       make([]int32, 4),
		turn:            1,
		lastDiscard:     tile.Must(tile.SuitCharacters, 3),
		lastDiscardSeat: 0,
	}
	rs.openClaimWindow()

	data, err := rs.MarshalRoundPersistJSON()
	require.NoError(t, err)
	restored, err := RestoreRoundFromPersistJSON("r-claim-persist", data)
	require.NoError(t, err)

	view := restored.SnapshotView()
	require.EqualValues(t, 2, view.ActingSeat)
	require.Equal(t, "claim", view.WaitingAction)
	require.Equal(t, "m3", view.PendingTile)
	require.Equal(t, []string{"gang", "pong"}, view.AvailableActions)
}

func TestDiscardHuContinuesFromDiscarderNextSeat(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-discard-hu",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 1)}), hand.New()},
		queBySeat:       make([]int32, 4),
		turn:            1,
		huedSeats:       make([]bool, 4),
		winnerSeats:     make([]int, 0, 3),
		lastDiscard:     tile.Must(tile.SuitCharacters, 3),
		lastDiscardSeat: 0,
	}
	rs.openClaimWindow()

	notifs, err := NewEngine("sichuan_xzdd").ApplyHu(context.Background(), rs, 2)
	require.NoError(t, err)
	require.False(t, rs.closed)
	require.True(t, rs.isHued(2))
	require.Equal(t, 1, rs.turn)

	var sawSeat1Draw bool
	for _, notification := range notifs {
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		if draw := env.GetDrawTile(); draw != nil && draw.GetSeatIndex() == 1 {
			sawSeat1Draw = true
		}
	}
	require.True(t, sawSeat1Draw)
}

func TestApplyTimeoutUsesBestClaimCandidate(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-timeout-claim",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.New()},
		queBySeat:       make([]int32, 4),
		turn:            1,
		lastDiscard:     tile.Must(tile.SuitCharacters, 3),
		lastDiscardSeat: 0,
	}
	rs.openClaimWindow()

	notifs, err := NewEngine("sichuan_xzdd").ApplyTimeout(context.Background(), rs)
	require.NoError(t, err)
	require.False(t, rs.claimWindowOpen)
	require.Len(t, notifs, 2)
	require.Equal(t, 2, rs.turn)
}

func TestApplyTimeoutAutoDiscardsCurrentTurn(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-timeout-discard",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1)}), hand.New(), hand.New(), hand.New()},
		queBySeat:       make([]int32, 4),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}

	notifs, err := NewEngine("sichuan_xzdd").ApplyTimeout(context.Background(), rs)
	require.NoError(t, err)
	require.Len(t, notifs, 2)
	require.Equal(t, 1, rs.turn)
}

func TestServiceAutoTimeoutSubmitsThroughActor(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-service-timeout",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1)}), hand.New(), hand.New(), hand.New()},
		queBySeat:       make([]int32, 4),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}
	data, err := rs.MarshalRoundPersistJSON()
	require.NoError(t, err)

	svc := NewService(NewLobby())
	require.NoError(t, svc.RecoverRoom("r-service-timeout", []string{"u0", "u1", "u2", "u3"}, "playing", data))
	notifs, err := svc.AutoTimeout(context.Background(), "r-service-timeout")
	require.NoError(t, err)
	require.Len(t, notifs, 2)
}

func TestSchedulerAutoTimeoutUsesFakeClock(t *testing.T) {
	fc := clock.NewFake(time.Unix(0, 0))
	svc := NewServiceWithRule(NewLobby(), "sichuan_xzdd")
	svc.SetClock(fc)
	svc.SetTimeoutConfig(TimeoutConfig{
		ExchangeThree: time.Second,
		QueMen:        time.Second,
		ClaimWindow:   time.Second,
		TsumoWindow:   time.Second,
		Discard:       time.Second,
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

func TestSubmitActionReturnsActorResultAfterContextCanceled(t *testing.T) {
	t.Parallel()

	a := &roomActor{ch: make(chan any)}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		msg := (<-a.ch).(cmdDiscard)
		cancel()
		time.Sleep(10 * time.Millisecond)
		msg.res <- actionResult{notifications: []Notification{{Kind: KindAction}}, err: nil}
	}()

	notifs, err := a.submitAction(ctx, cmdDiscard{userID: "u1", tile: "m1", res: make(chan actionResult, 1)})
	require.NoError(t, err)
	require.Len(t, notifs, 1)
}

func TestSubmitActionReturnsRateLimitedWhenMailboxFull(t *testing.T) {
	t.Parallel()

	a := &roomActor{ch: make(chan any, 1)}
	a.ch <- cmdReady{}
	notifs, err := a.submitAction(context.Background(), cmdDiscard{userID: "u1", tile: "m1", res: make(chan actionResult, 1)})
	require.ErrorIs(t, err, ErrRateLimited)
	require.Nil(t, notifs)
}

func TestServiceMailboxCapacityOverride(t *testing.T) {
	t.Parallel()

	svc := NewService(NewLobby())
	svc.SetMailboxCapacity(3)
	require.NoError(t, svc.EnsureRoom("mailbox-config-room"))
	a := svc.getActor("mailbox-config-room")
	require.NotNil(t, a)
	require.Equal(t, 3, cap(a.ch))
}

func TestDoGangClosesRoomAfterSettlement(t *testing.T) {
	t.Parallel()

	r := domainroom.NewRoom("r-gang-close")
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, ok := r.JoinAutoSeat(uid)
		require.True(t, ok)
	}
	for i := 0; i < 4; i++ {
		require.NoError(t, r.SetReady(i, true))
	}
	require.NoError(t, r.StartPlaying())

	rs := &RoundState{
		roomID:         "r-gang-close",
		ruleID:         "sichuan_xzdd",
		rule:           rules.MustGet("sichuan_xzdd"),
		playerIDs:      [4]string{"u0", "u1", "u2", "u3"},
		wall:           wall.NewFromOrderedTiles(nil),
		hands:          []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1)}), hand.New(), hand.New(), hand.New()},
		queBySeat:      make([]int32, 4),
		waitingDiscard: true,
		turn:           0,
	}

	a := newRoomActor(r, nil)
	a.engine = NewEngine("sichuan_xzdd")
	a.round = rs

	notifs, err := a.doGang("u0", "m1")
	require.NoError(t, err)
	require.Len(t, notifs, 2)
	require.Nil(t, a.round)
	require.Equal(t, domainroom.StateClosed, r.FSM.State())
}

func outboundTestMsgID(kind Kind) (uint16, bool) {
	switch kind {
	case KindExchangeThreeDone:
		return msgid.ExchangeThreeDone, true
	case KindQueMenDone:
		return msgid.QueMenDone, true
	case KindStartGame:
		return msgid.StartGame, true
	case KindDrawTile:
		return msgid.DrawTile, true
	case KindAction:
		return msgid.ActionNotify, true
	case KindSettlement:
		return msgid.Settlement, true
	default:
		return 0, false
	}
}

func driveRoundToClose(ctx context.Context, svc *Service, roomID string) error {
	for i := 0; i < 256; i++ {
		a := svc.getActor(roomID)
		if a == nil || a.round == nil {
			return nil
		}
		if a.round.waitingExchange {
			for seat, done := range a.round.exchangeSubmitted {
				if !done {
					if _, err := svc.ExchangeThree(ctx, roomID, a.room.PlayerIDs[seat], nil, 0); err != nil {
						return err
					}
				}
			}
			continue
		}
		if a.round.waitingQueMen {
			for seat, done := range a.round.queSubmitted {
				if !done {
					if _, err := svc.QueMen(ctx, roomID, a.room.PlayerIDs[seat], 0); err != nil {
						return err
					}
				}
			}
			continue
		}
		if claimSeat := a.round.claimSeat(); claimSeat >= 0 {
			userID := a.room.PlayerIDs[claimSeat]
			if a.round.rawCanClaimGang(claimSeat) {
				if _, err := svc.Gang(ctx, roomID, userID, a.round.lastDiscard.String()); err != nil {
					if bytes.Contains([]byte(err.Error()), []byte("round closed")) {
						return nil
					}
					return err
				}
			} else {
				if _, err := svc.Pong(ctx, roomID, userID); err != nil {
					if bytes.Contains([]byte(err.Error()), []byte("round closed")) {
						return nil
					}
					return err
				}
			}
			continue
		}
		seat := a.round.turn
		userID := a.room.PlayerIDs[seat]
		if a.round.waitingTsumo {
			notifs, err := svc.Hu(ctx, roomID, userID)
			if err == nil && containsSettlement(notifs) {
				return nil
			}
			if err != nil && !bytes.Contains([]byte(err.Error()), []byte("hu not allowed")) {
				return err
			}
		}
		discard := chooseDiscard(a.round.hands[seat], tile.Suit(a.round.queBySeat[seat]))
		notifs, err := svc.Discard(ctx, roomID, userID, discard.String())
		if err != nil {
			if bytes.Contains([]byte(err.Error()), []byte("round closed")) {
				return nil
			}
			return err
		}
		if containsSettlement(notifs) {
			return nil
		}
	}
	return context.DeadlineExceeded
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
