// 房间服务单元测试：加入、准备与广播触发。
package room

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/clock"
	domainroom "racoo.cn/lsp/internal/domain/room"
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

func TestRestoreRoundRejectsFutureSchema(t *testing.T) {
	_, err := RestoreRoundFromPersistJSON("room-future-schema", []byte(`{"schema_version":999}`))
	require.ErrorIs(t, err, ErrRoundPersistUnsupportedSchema)
}

func (f *fakeBC) Broadcast(roomID string, msgID uint16, payload []byte) {
	f.lastRoom = roomID
	f.lastMsg = msgID
	f.n++
}

func TestStartRoundEmitsTargetedInitialDeals(t *testing.T) {
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	players := [4]string{"u0", "u1", "u2", "u3"}
	_, notifications, err := e.StartRound(context.Background(), "room-initial-deal", players)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(notifications), 4)
	for seat := 0; seat < 4; seat++ {
		notification := notifications[seat]
		require.Equal(t, KindInitialDeal, notification.Kind)
		require.EqualValues(t, seat, notification.TargetSeat)
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		deal := env.GetInitialDeal()
		require.NotNil(t, deal)
		require.EqualValues(t, seat, deal.GetSeatIndex())
		require.Len(t, deal.GetTiles(), 13)
	}
}

func TestStartRoundBiaozhunSkipsExchangeThree(t *testing.T) {
	e := NewEngine("sichuan_xuezhandaodi_biaozhun")
	players := [4]string{"u0", "u1", "u2", "u3"}
	rs, notifications, err := e.StartRound(context.Background(), "room-biaozhun", players)
	require.NoError(t, err)
	require.True(t, rs.waitingOpening)
	require.Equal(t, "que_men", rs.waitingKind())
	require.Len(t, notifications, 8)
	for _, notification := range notifications[4:] {
		require.Equal(t, KindAction, notification.Kind)
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		action := env.GetAction()
		require.NotNil(t, action)
		require.Equal(t, "que_men", action.GetAction())
		require.Equal(t, clientv1.Phase_PHASE_OPENING, action.GetPhase())
	}
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
	if f.lastMsg != protocol.DrawTile && f.lastMsg != protocol.ActionNotify && f.lastMsg != protocol.StartGame {
		t.Fatalf("unexpected broadcast room=%s msg=%d", f.lastRoom, f.lastMsg)
	}
}

func TestJoinRejectsAfterRoundStarted(t *testing.T) {
	svc := NewService(NewLobby())
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

func TestServiceSupportsConfiguredNextHand(t *testing.T) {
	t.Parallel()

	r := domainroom.NewRoom("room-next-hand")
	r.MaxHands = 2
	for i := 0; i < 4; i++ {
		_, ok := r.JoinAutoSeat("p" + string(rune('0'+i)))
		require.True(t, ok)
		require.NoError(t, r.SetReady(domainroom.Seat(i), true))
	}
	require.NoError(t, r.StartPlaying())
	a := &roomActor{
		room: r,
		round: &RoundState{
			scoreEvents: make([]rules.ScoreEvent, 0),
		},
	}
	a.closeRoomAfterRound()
	a.round = nil
	require.Nil(t, a.round)
	require.Equal(t, domainroom.StateWaiting, a.room.FSM.State())
	require.EqualValues(t, 2, a.room.HandIndex)
}

func TestRecoverRoomAndRuleID(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithRule(NewLobby(), "sichuan_xuezhandaodi_huansanzhang")
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

	svc := NewServiceWithRule(NewLobby(), "sichuan_xuezhandaodi_huansanzhang")
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

			svc := NewServiceWithRule(NewLobby(), "sichuan_xuezhandaodi_huansanzhang")
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

	svc := NewServiceWithRule(NewLobby(), "sichuan_xuezhandaodi_huansanzhang")
	err := svc.RecoverRoom("room-direct-settling", []string{"u1", "u2", "u3", "u4"}, "settling", nil)
	require.NoError(t, err)
	_, state, _, ok := svc.RoomSnapshot("room-direct-settling")
	require.True(t, ok)
	require.Equal(t, "settling", state)
}

func TestRecoverRoomIdempotentForExistingRoom(t *testing.T) {
	t.Parallel()

	svc := NewServiceWithRule(NewLobby(), "sichuan_xuezhandaodi_huansanzhang")
	const rid = "room-recover-idem"
	require.NoError(t, svc.RecoverRoom(rid, []string{"u1", "u2", "u3", "u4"}, "ready", nil))

	// 第二次调用应当复用 lobby 中已有 room，不返回错误也不重置 FSM。
	require.NoError(t, svc.RecoverRoom(rid, []string{"u1", "u2", "u3", "u4"}, "playing", nil))

	_, state, _, ok := svc.RoomSnapshot(rid)
	require.True(t, ok)
	require.Equal(t, "ready", state, "已存在房间不应被第二次 RecoverRoom 改写")
}

func TestRoundViewShowsClaimWindow(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-claim",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 7)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitDots, 9)}), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingDiscard:  true,
		turn:            1,
		currentDraw:     tile.Must(tile.SuitDots, 7),
		lastDiscard:     tile.Must(tile.SuitCharacters, 3),
		lastDiscardSeat: 0,
	}
	rs.openClaimWindow()

	view := rs.SnapshotView()
	require.EqualValues(t, 2, view.ActingSeat)
	require.Equal(t, "claim_window", view.WaitingAction)
	require.Equal(t, "m3", view.PendingTile)
	require.Equal(t, []string{"pong"}, view.AvailableActions)
}

func TestRoundViewIncludesAuthoritativeTUIFields(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-contract",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		caps:            rules.CapabilitiesOf(rules.MustGet("sichuan_xuezhandaodi_huansanzhang")),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7), tile.Must(tile.SuitDots, 8)}),
		hands:           []*hand.Hand{hand.New(), hand.New(), hand.New(), hand.New()},
		melds:           [][]string{{"pong:m3"}, nil, {"gang:p5"}, nil},
		scoreEvents:     make([]rules.ScoreEvent, 0),
		ruleState:       testRuleState(make([]int32, 4)),
		lastDiscardSeat: -1,
		deadlineUnixMs:  12345,
	}
	rs.rememberLastAction(rs.actionDetail(0, "discard", tile.Must(tile.SuitCharacters, 3), SeatInvalid, 0))

	view := rs.SnapshotView()
	require.EqualValues(t, 2, view.WallRemaining)
	require.EqualValues(t, 12345, view.DeadlineUnixMs)
	require.NotNil(t, view.LastAction)
	require.Equal(t, "discard", view.LastAction.GetAction())
	require.NotNil(t, view.RuleMeta)
	require.Contains(t, view.RuleMeta.GetEnabledFeatures(), "exchange_three")
	require.Len(t, view.MeldInfosBySeat, 4)
	require.Equal(t, "pong", view.MeldInfosBySeat[0].GetMelds()[0].GetKind())
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
			e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
			for seat := 0; seat < 4; seat++ {
				_, err := e.ApplyOpeningActionByPlayer(context.Background(), rs, Seat(seat), openingExchangeThree, tilesToStrings(rs.hands[seat].Tiles()), tt.direction, 0, nil)
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantSeat0, rs.hands[0].Tiles())
		})
	}
}

func TestExchangeThreeKeepsFirstDirectionAsAuthority(t *testing.T) {
	t.Parallel()

	rs := roundWaitingExchange()
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	_, err := e.ApplyOpeningActionByPlayer(context.Background(), rs, 0, openingExchangeThree, tilesToStrings(rs.hands[0].Tiles()), 1, 0, nil)
	require.NoError(t, err)
	_, err = e.ApplyOpeningActionByPlayer(context.Background(), rs, 1, openingExchangeThree, tilesToStrings(rs.hands[1].Tiles()), 2, 0, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, testSichuanExchangeDirection(rs.ruleState))
}

func TestExchangeThreeRejectsInvalidPlayerSelection(t *testing.T) {
	t.Parallel()

	rs := roundWaitingExchange()
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	_, err := e.ApplyOpeningActionByPlayer(context.Background(), rs, 0, openingExchangeThree, []string{"m1", "m2"}, 0, 0, nil)
	require.ErrorContains(t, err, "invalid exchange selection")
	require.False(t, rs.openingSubmitted(openingExchangeThree)[0])

	_, err = e.ApplyOpeningActionByPlayer(context.Background(), rs, 0, openingExchangeThree, []string{"m1", "m2", "p9"}, 0, 0, nil)
	require.ErrorContains(t, err, "exchange tile from hand")
	require.False(t, rs.openingSubmitted(openingExchangeThree)[0])
}

func TestExchangeTimeoutUsesAuthoritativeDirection(t *testing.T) {
	t.Parallel()

	rs := roundWaitingExchange()
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	_, err := e.ApplyOpeningActionByPlayer(context.Background(), rs, 0, openingExchangeThree, tilesToStrings(rs.hands[0].Tiles()), 1, 0, nil)
	require.NoError(t, err)
	for seat := 1; seat < 4; seat++ {
		_, err := e.ApplyTimeout(context.Background(), rs)
		require.NoError(t, err)
	}
	require.True(t, rs.waitingOpening)
	require.Equal(t, "que_men", rs.waitingKind())
	require.EqualValues(t, 1, testSichuanExchangeDirection(rs.ruleState))
}

func TestQueMenUsesClientSuit(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-que",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3)}), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleStateWithSubmitted(make([]int32, 4), openingQueMen, make([]bool, 4)),
		waitingOpening:  true,
		lastDiscardSeat: -1,
	}
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	_, err := e.ApplyOpeningActionByPlayer(context.Background(), rs, 0, openingQueMen, nil, 0, int32(tile.SuitBamboo), nil)
	require.NoError(t, err)
	require.EqualValues(t, tile.SuitBamboo, rs.missingSuitBySeat()[0])
}

func TestQueMenStartGameUsesZeroBasedRoundAndHandIndex(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-que-start",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleStateWithSubmitted(make([]int32, 4), openingQueMen, []bool{true, true, true, false}),
		waitingOpening:  true,
		lastDiscardSeat: -1,
	}
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	notifs, err := e.ApplyOpeningActionByPlayer(context.Background(), rs, 3, openingQueMen, nil, 0, int32(tile.SuitBamboo), nil)
	require.NoError(t, err)

	var start *clientv1.StartGameNotify
	for _, notification := range notifs {
		if notification.Kind != KindStartGame {
			continue
		}
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		start = env.GetStartGame()
	}
	require.NotNil(t, start)
	require.Zero(t, start.GetRoundIndex())
	require.Zero(t, start.GetHandIndex())
}

func TestApplyPongInterruptsPendingTurn(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-pong",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 7)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitDots, 9)}), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingDiscard:  true,
		turn:            1,
		currentDraw:     tile.Must(tile.SuitDots, 7),
		lastDiscard:     tile.Must(tile.SuitCharacters, 3),
		lastDiscardSeat: 0,
	}
	rs.openClaimWindow()

	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	notifs, err := e.ApplyPong(context.Background(), rs, 2)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	require.Equal(t, Seat(2), rs.turn)
	require.True(t, rs.waitingDiscard)
	require.Zero(t, rs.currentDraw)
	require.Equal(t, []tile.Tile{tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 2)}, rs.hands[1].Tiles())
	require.Equal(t, []tile.Tile{tile.Must(tile.SuitDots, 9)}, rs.hands[2].Tiles())
}

func roundWaitingExchange() *RoundState {
	return &RoundState{
		roomID:          "r-exchange",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitBamboo, 1), tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitBamboo, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 5), tile.Must(tile.SuitCharacters, 6)})},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingOpening:  true,
		lastDiscardSeat: -1,
	}
}

func TestApplyDiscardPromptsClaimInsteadOfNextDraw(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-claim-prompt",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3)}), hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}

	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
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

func TestQueSuitDiscardDoesNotOfferPongOrGang(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-que-claim-filter",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3)}), hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.New()},
		ruleState:       testRuleStateWithSubmitted([]int32{int32(tile.SuitDots), int32(tile.SuitDots), int32(tile.SuitCharacters), int32(tile.SuitDots)}, openingQueMen, []bool{true, true, true, true}),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}

	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	_, err := e.ApplyDiscard(context.Background(), rs, 0, "m3")
	require.NoError(t, err)
	require.False(t, rs.claimWindowOpen)
	require.Empty(t, rs.claimCandidates)
}

func TestQueSuitCannotSelfGang(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		ruleID:         "sichuan_xuezhandaodi_huansanzhang",
		rule:           rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		hands:          []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitDots, 9), tile.Must(tile.SuitDots, 9), tile.Must(tile.SuitDots, 9), tile.Must(tile.SuitDots, 9)}), hand.New(), hand.New(), hand.New()},
		ruleState:      testRuleStateWithSubmitted([]int32{int32(tile.SuitDots), -1, -1, -1}, openingQueMen, []bool{true, false, false, false}),
		waitingDiscard: true,
		turn:           0,
	}
	require.False(t, rs.canSelfGang(0, "p9"))
}

func TestApplyDiscardOpenMeldHuBeatsPong(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-open-discard-hu",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitBamboo, 2)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitBamboo, 3), tile.Must(tile.SuitBamboo, 3), tile.Must(tile.SuitBamboo, 3), tile.Must(tile.SuitBamboo, 5), tile.Must(tile.SuitBamboo, 5)}), hand.New(), hand.New()},
		ruleState:       testRuleStateWithSubmitted([]int32{int32(tile.SuitCharacters), int32(tile.SuitCharacters), -1, -1}, openingQueMen, []bool{true, true, false, false}),
		melds:           [][]string{nil, {"pong:p7,p7,p7:2", "pong:p2,p2,p2:3"}, nil, nil},
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
		surrendered:     make([]bool, 4),
	}

	notifs, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").ApplyDiscard(context.Background(), rs, 0, "s2")
	require.NoError(t, err)
	require.True(t, rs.claimWindowOpen)
	require.Len(t, rs.claimCandidates, 1)
	require.Equal(t, Seat(1), rs.claimCandidates[0].seat)
	require.Contains(t, rs.claimCandidates[0].actions, "hu")
	require.Contains(t, rs.claimCandidates[0].actions, "pong")
	require.Equal(t, "hu_choice", rs.claimCandidates[0].claimChoiceAction())

	var sawHuChoice bool
	for _, notification := range notifs {
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		if action := env.GetAction(); action != nil && action.GetAction() == "hu_choice" {
			sawHuChoice = true
			require.EqualValues(t, 1, action.GetSeatIndex())
			require.Equal(t, []string{"hu", "pong"}, action.GetPhaseUpdate().GetAvailableActions())
		}
	}
	require.True(t, sawHuChoice)
}

func TestQueSuitBlocksHuCandidateAndSelfDraw(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-que-blocks-hu",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitBamboo, 2)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitBamboo, 3), tile.Must(tile.SuitBamboo, 3), tile.Must(tile.SuitBamboo, 3), tile.Must(tile.SuitBamboo, 5), tile.Must(tile.SuitBamboo, 5)}), hand.New(), hand.New()},
		ruleState:       testRuleStateWithSubmitted([]int32{int32(tile.SuitCharacters), int32(tile.SuitBamboo), -1, -1}, openingQueMen, []bool{true, true, false, false}),
		melds:           [][]string{nil, {"pong:p7,p7,p7:2", "pong:p2,p2,p2:3"}, nil, nil},
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
		surrendered:     make([]bool, 4),
	}
	_, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").ApplyDiscard(context.Background(), rs, 0, "s2")
	require.NoError(t, err)
	require.False(t, rs.claimWindowOpen)
	require.Empty(t, rs.claimCandidates)

	drawState := &RoundState{
		roomID:          "r-que-blocks-tsumo",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitBamboo, 2), tile.Must(tile.SuitBamboo, 3), tile.Must(tile.SuitBamboo, 3), tile.Must(tile.SuitBamboo, 3), tile.Must(tile.SuitBamboo, 5), tile.Must(tile.SuitBamboo, 5)}), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleStateWithSubmitted([]int32{int32(tile.SuitBamboo), -1, -1, -1}, openingQueMen, []bool{true, false, false, false}),
		melds:           [][]string{{"pong:p7,p7,p7:1", "pong:p2,p2,p2:2"}, nil, nil, nil},
		turn:            0,
		lastDiscardSeat: -1,
		surrendered:     make([]bool, 4),
	}
	_, err = NewEngine("sichuan_xuezhandaodi_huansanzhang").drawForCurrentTurn(drawState)
	require.NoError(t, err)
	require.False(t, drawState.waitingTsumo)
	require.True(t, drawState.waitingDiscard)
}

func TestSelfDrawHuWithOpenQueSuitMeldUsesClosedHandOnlyForQueBlock(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-open-que-meld-tsumo",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7), tile.Must(tile.SuitDots, 9)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitDots, 4), tile.Must(tile.SuitDots, 4), tile.Must(tile.SuitDots, 5), tile.Must(tile.SuitDots, 6), tile.Must(tile.SuitBamboo, 4), tile.Must(tile.SuitBamboo, 4), tile.Must(tile.SuitBamboo, 4), tile.Must(tile.SuitBamboo, 7), tile.Must(tile.SuitBamboo, 8), tile.Must(tile.SuitBamboo, 9)}), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleStateWithSubmitted([]int32{int32(tile.SuitCharacters), -1, -1, -1}, openingQueMen, []bool{true, false, false, false}),
		melds:           [][]string{{"pong:m3,m3,m3:1"}, nil, nil, nil},
		turn:            0,
		lastDiscardSeat: -1,
		surrendered:     make([]bool, 4),
	}

	notifs, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").drawForCurrentTurn(rs)
	require.NoError(t, err)
	require.True(t, rs.waitingTsumo)
	require.False(t, rs.waitingDiscard)
	require.Equal(t, tile.Must(tile.SuitDots, 7), rs.pendingDraw)

	var sawChoice bool
	for _, notification := range notifs {
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		if action := env.GetAction(); action != nil && action.GetAction() == "tsumo_choice" {
			sawChoice = true
			require.EqualValues(t, 0, action.GetSeatIndex())
			require.Equal(t, []string{"hu", "pass"}, action.GetPhaseUpdate().GetAvailableActions())
		}
	}
	require.True(t, sawChoice)
}

func TestDrawTileNotificationProjectsTileOnlyToActor(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-private-draw",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		turn:            1,
		lastDiscardSeat: -1,
	}
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	notifs, err := e.drawForCurrentTurn(rs)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	require.Equal(t, PrivacyPerSeat, notifs[0].Privacy)
	require.NotNil(t, notifs[0].Project)

	var actorEnv clientv1.Envelope
	require.NoError(t, proto.Unmarshal(notifs[0].Project(1), &actorEnv))
	require.Equal(t, "p7", actorEnv.GetDrawTile().GetTile())

	var otherEnv clientv1.Envelope
	require.NoError(t, proto.Unmarshal(notifs[0].Project(0), &otherEnv))
	require.Empty(t, otherEnv.GetDrawTile().GetTile())
}

func TestApplyDiscardPromptsMultipleClaimCandidates(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-multi-claim",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)})},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}

	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	notifs, err := e.ApplyDiscard(context.Background(), rs, 0, "m3")
	require.NoError(t, err)
	require.Len(t, notifs, 2)
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
	require.Equal(t, map[int32]string{2: "gang_choice"}, claimBySeat)
	_, err = e.ApplyPong(context.Background(), rs, 1)
	require.Error(t, err)

	notifs, err = e.ApplyGang(context.Background(), rs, 2, "m3")
	require.NoError(t, err)
	require.False(t, rs.claimWindowOpen)
	require.Len(t, notifs, 2)
	require.Equal(t, Seat(2), rs.turn)
	require.Len(t, rs.meldInfosBySeat()[2].GetMelds(), 1)
	require.Equal(t, "zhi_gang", rs.meldInfosBySeat()[2].GetMelds()[0].GetKind())
	require.EqualValues(t, 0, rs.meldInfosBySeat()[2].GetMelds()[0].GetClaimedFromSeat())
	require.Equal(t, rules.GangKindMing, rs.gangRecords[0].Kind)
}

func TestApplyAnGangRecordsConcealedMeldAndHidesActionTile(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-an-gang",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1)}), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}
	notifs, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").ApplyGang(context.Background(), rs, 0, "m1")
	require.NoError(t, err)
	require.Len(t, notifs, 2)
	require.Equal(t, rules.GangKindAn, rs.gangRecords[0].Kind)
	info := rs.meldInfosBySeat()[0].GetMelds()[0]
	require.Equal(t, "an_gang", info.GetKind())
	require.True(t, info.GetConcealed())
	require.Equal(t, PrivacyPerSeat, notifs[0].Privacy)

	var other clientv1.Envelope
	require.NoError(t, proto.Unmarshal(notifs[0].Project(1), &other))
	require.Empty(t, other.GetAction().GetTile())
	require.Empty(t, other.GetAction().GetDetail().GetTile())
}

func TestApplyBuGangCompletesWithoutRobCandidate(t *testing.T) {
	t.Parallel()

	gangTile := tile.Must(tile.SuitCharacters, 5)
	rs := &RoundState{
		roomID:          "r-bu-gang",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{gangTile}), hand.New(), hand.New(), hand.New()},
		melds:           [][]string{{"pong:m5,m5,m5:1"}, nil, nil, nil},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}
	_, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").ApplyGang(context.Background(), rs, 0, "m5")
	require.NoError(t, err)
	require.Equal(t, rules.GangKindBu, rs.gangRecords[0].Kind)
	info := rs.meldInfosBySeat()[0].GetMelds()[0]
	require.Equal(t, "bu_gang", info.GetKind())
	require.False(t, rs.qiangGangWindow)
}

func TestApplyBuGangCanBeRobbedWithoutGangRecord(t *testing.T) {
	t.Parallel()

	gangTile := tile.Must(tile.SuitCharacters, 5)
	readyHand := []tile.Tile{
		tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1),
		tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2),
		tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3),
		tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4),
		gangTile,
	}
	rs := &RoundState{
		roomID:          "r-rob-bu-gang",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{gangTile}), hand.New(), hand.FromTiles(readyHand), hand.New()},
		melds:           [][]string{{"pong:m5,m5,m5:1"}, nil, nil, nil},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	notifs, err := e.ApplyGang(context.Background(), rs, 0, "m5")
	require.NoError(t, err)
	require.True(t, rs.qiangGangWindow)
	require.Len(t, notifs, 1)
	require.Equal(t, Seat(2), rs.claimSeat())

	_, err = e.ApplyHu(context.Background(), rs, 2)
	require.NoError(t, err)
	require.Empty(t, rs.gangRecords)
	require.NotContains(t, rs.hands[0].Tiles(), gangTile)
	require.Equal(t, "pong", rs.meldInfosBySeat()[0].GetMelds()[0].GetKind())
}

type allowChiClaimPolicy struct{}

func (allowChiClaimPolicy) Candidates(ctx rules.ClaimContext) []rules.ClaimAction {
	if ctx.Hued || ctx.Hand == nil || ctx.Tile == 0 || ctx.Seat == ctx.SourceSeat {
		return nil
	}
	return []rules.ClaimAction{{Name: rules.ActionChi, ChoiceAction: "chi_choice", Priority: 1}}
}

func TestApplyChiRecordsStructuredMeldAndRequiresExplicitDiscard(t *testing.T) {
	t.Parallel()

	rule := rules.MustGet("sichuan_xuezhandaodi_huansanzhang")
	caps := rules.CapabilitiesOf(rule)
	caps.Claims = allowChiClaimPolicy{}
	rs := &RoundState{
		roomID:          "r-chi",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rule,
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles(nil),
		hands:           []*hand.Hand{hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3)}), hand.New(), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		caps:            caps,
		lastDiscard:     tile.Must(tile.SuitCharacters, 1),
		lastDiscardSeat: 0,
		claimWindowOpen: true,
		claimCandidates: []claimCandidate{{seat: 1, actions: []string{"chi"}, priority: 1, choiceAction: "chi_choice"}},
		turn:            0,
	}
	notifs, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").ApplyChi(context.Background(), rs, 1, nil)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	require.True(t, rs.waitingDiscard)
	require.Equal(t, Seat(1), rs.turn)
	require.Empty(t, rs.hands[1].Tiles())
	info := rs.meldInfosBySeat()[1].GetMelds()[0]
	require.Equal(t, "chi", info.GetKind())
	require.Equal(t, []string{"m1", "m2", "m3"}, info.GetTiles())
	require.EqualValues(t, 0, info.GetClaimedFromSeat())
}

func TestApplyPassRelaysClaimCandidate(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-pass-relay",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)})},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	_, err := e.ApplyDiscard(context.Background(), rs, 0, "m3")
	require.NoError(t, err)

	notifs, err := e.ApplyPass(context.Background(), rs, 2)
	require.NoError(t, err)
	require.True(t, rs.claimWindowOpen)
	require.Len(t, rs.claimCandidates, 2)
	require.Len(t, notifs, 1)

	var env clientv1.Envelope
	require.NoError(t, proto.Unmarshal(notifs[0].Payload, &env))
	require.Equal(t, "pong_choice", env.GetAction().GetAction())
	require.EqualValues(t, 1, env.GetAction().GetSeatIndex())
}

func TestApplyPassSelfDrawKeepsPlayerDiscardChoice(t *testing.T) {
	t.Parallel()

	drawn := tile.Must(tile.SuitDots, 7)
	rs := &RoundState{
		roomID:          "r-pass-tsumo",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles(nil),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitDots, 1)}), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		turn:            0,
		waitingTsumo:    true,
		pendingDraw:     drawn,
		currentDraw:     drawn,
		lastDiscardSeat: -1,
	}
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	notifs, err := e.ApplyPass(context.Background(), rs, 0)
	require.NoError(t, err)
	require.Empty(t, notifs)
	require.False(t, rs.waitingTsumo)
	require.True(t, rs.waitingDiscard)
	require.Contains(t, rs.hands[0].Tiles(), drawn)
}

func TestApplyHuClearsHuedSeatActionWindowBeforeNextDraw(t *testing.T) {
	t.Parallel()

	drawn := tile.Must(tile.SuitCharacters, 5)
	rs := &RoundState{
		roomID:          "r-hu-clears-window",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 5)}), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		turn:            0,
		waitingTsumo:    true,
		pendingDraw:     drawn,
		currentDraw:     drawn,
		lastDiscardSeat: -1,
	}

	notifs, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").ApplyHu(context.Background(), rs, 0)
	require.NoError(t, err)
	require.True(t, rs.isHued(0))
	require.Equal(t, Seat(1), rs.turn)
	require.NotEmpty(t, notifs)
	require.False(t, containsSettlement(notifs), "血战首家胡牌后不得立刻结算")

	var hu *clientv1.ActionNotify
	for _, notification := range notifs {
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		if action := env.GetAction(); action != nil && action.GetAction() == "hu" {
			hu = action
			break
		}
	}
	require.NotNil(t, hu)
	require.Equal(t, clientv1.WaitingReason_WAITING_REASON_NONE, hu.GetPhaseUpdate().GetReason())
	require.NotContains(t, hu.GetPhaseUpdate().GetActingSeats(), int32(0), "已胡座位不得继续作为行动座位下发")
}

func TestGuobiaoFirstHuEndsRound(t *testing.T) {
	t.Parallel()

	drawn := tile.Must(tile.SuitCharacters, 5)
	rs := &RoundState{
		roomID:          "r-guobiao-hu",
		ruleID:          "guobiao_jingji_biaozhun",
		rule:            rules.MustGet("guobiao_jingji_biaozhun"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 5)}), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		turn:            0,
		waitingTsumo:    true,
		pendingDraw:     drawn,
		currentDraw:     drawn,
		lastDiscardSeat: -1,
		scoreEvents:     make([]rules.ScoreEvent, 0),
	}

	notifs, err := NewEngine("guobiao_jingji_biaozhun").ApplyHu(context.Background(), rs, 0)
	require.NoError(t, err)
	require.True(t, rs.closed)
	require.True(t, containsSettlement(notifs), "国标首个合法胡牌应立即结算")
}

func TestClaimWindowPersistsAndRestores(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-claim-persist",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
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
	require.Equal(t, "claim_window", view.WaitingAction)
	require.Equal(t, "m3", view.PendingTile)
	require.Equal(t, []string{"gang", "pong"}, view.AvailableActions)
}

func TestDiscardHuContinuesFromDiscarderNextSeat(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-discard-hu",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitCharacters, 4), tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 1)}), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		turn:            1,
		lastDiscard:     tile.Must(tile.SuitCharacters, 3),
		lastDiscardSeat: 0,
	}
	rs.openClaimWindow()

	notifs, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").ApplyHu(context.Background(), rs, 2)
	require.NoError(t, err)
	require.False(t, rs.closed)
	require.True(t, rs.isHued(2))
	require.Equal(t, Seat(1), rs.turn)
	require.False(t, containsSettlement(notifs), "血战点炮胡后未达到终局条件不得结算")

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

func TestPlayerJourney_C2_2_ClaimTimeoutDoesNotChooseForHuman(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-timeout-claim",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3), tile.Must(tile.SuitCharacters, 3)}), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		turn:            1,
		lastDiscard:     tile.Must(tile.SuitCharacters, 3),
		lastDiscardSeat: 0,
	}
	rs.openClaimWindow()

	notifs, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").ApplyTimeout(context.Background(), rs)
	require.NoError(t, err)
	require.True(t, rs.isSurrendered(2))
	require.True(t, rs.claimWindowOpen, "下一候选仍可继续抢答")
	require.NotEmpty(t, notifs)
	for _, notification := range notifs {
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		if action := env.GetAction(); action != nil {
			require.NotContains(t, []string{"hu", "gang", "pong"}, action.GetAction(), "真人超时不得替玩家选择收益动作")
		}
	}
}

func TestPlayerJourney_E1_ExchangeTimeoutSurrendersDoesNotChooseForHuman(t *testing.T) {
	t.Parallel()

	rs := roundWaitingExchange()
	e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
	_, err := e.ApplyOpeningActionByPlayer(context.Background(), rs, 0, openingExchangeThree, tilesToStrings(rs.hands[0].Tiles()), 1, 0, nil)
	require.NoError(t, err)
	seat1Hand := append([]tile.Tile(nil), rs.hands[1].Tiles()...)

	notifs, err := e.ApplyTimeout(context.Background(), rs)
	require.NoError(t, err)
	require.True(t, rs.isSurrendered(1))
	require.True(t, rs.waitingOpening)
	require.Equal(t, "exchange_three", rs.waitingKind())
	require.Empty(t, notifs)
	require.Equal(t, seat1Hand, rs.hands[1].Tiles(), "真人换三张超时不得由服务端代选手牌")
}

func TestPlayerJourney_Q1_QueTimeoutSurrendersDoesNotChooseForHuman(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-timeout-que",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 2), tile.Must(tile.SuitDots, 9)}), hand.New(), hand.New()},
		ruleState:       testRuleStateWithSubmitted([]int32{0, -1, -1, -1}, openingQueMen, []bool{true, false, false, false}),
		waitingOpening:  true,
		lastDiscardSeat: -1,
	}

	notifs, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").ApplyTimeout(context.Background(), rs)
	require.NoError(t, err)
	require.True(t, rs.isSurrendered(1))
	require.True(t, rs.waitingOpening)
	require.Equal(t, "que_men", rs.waitingKind())
	require.Empty(t, notifs)
	require.EqualValues(t, -1, rs.missingSuitBySeat()[1], "真人定缺超时不得由服务端代选缺门")
}

func TestPlayerJourney_D2_4_DiscardTimeoutSurrenders(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-timeout-discard",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1)}), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingDiscard:  true,
		turn:            0,
		lastDiscardSeat: -1,
	}

	notifs, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").ApplyTimeout(context.Background(), rs)
	require.NoError(t, err)
	require.True(t, rs.isSurrendered(0))
	require.Len(t, notifs, 1)
	require.Equal(t, Seat(1), rs.turn)
	for _, notification := range notifs {
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		if action := env.GetAction(); action != nil {
			require.NotEqual(t, "discard", action.GetAction(), "真人出牌超时不得由服务端代选弃牌")
		}
	}
}

func TestServiceAutoTimeoutSurrendersThroughActor(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-service-timeout",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1)}), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
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
	require.Len(t, notifs, 1)
	a := svc.getActor("r-service-timeout")
	require.NotNil(t, a)
	require.True(t, a.room.Surrendered[0])
}

func TestSchedulerAutoTimeoutUsesFakeClock(t *testing.T) {
	fc := clock.NewFake(time.Unix(0, 0))
	svc := NewServiceWithRule(NewLobby(), "sichuan_xuezhandaodi_huansanzhang")
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

func TestSubmitActionReturnsCtxErrAfterContextCanceled(t *testing.T) {
	// 行为变更说明（FIX-1）：submit* 发送成功后立即释放 submitMu，等待 <-res 时已无锁。
	// ctx 取消后 select 优先返回 ctx.Err()，不再持锁等待 actor 的迟到响应。
	// 这是正确的 ctx 语义：调用方取消表示不再需要结果，迟到的 res 写入缓冲 channel 后被 GC 回收。
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
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, notifs)
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

func TestServiceLeaveDuringPlayMarksSurrender(t *testing.T) {
	ctx := context.Background()
	svc := NewServiceWithRule(NewLobby(), "sichuan_xuezhandaodi_huansanzhang")
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
	require.True(t, a.room.Surrendered[2])
	require.Equal(t, "u2", a.room.PlayerIDs[2])
	require.True(t, a.round.surrendered[2])
}

func TestServiceLeaveDuringPlayCanBeDisabled(t *testing.T) {
	ctx := context.Background()
	svc := NewServiceWithRule(NewLobby(), "sichuan_xuezhandaodi_huansanzhang")
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

func TestRoundPersistRestoresSurrenderedSeats(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:          "r-persist-surrendered",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 7)}),
		hands:           []*hand.Hand{hand.New(), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		waitingDiscard:  true,
		turn:            1,
		lastDiscardSeat: -1,
		surrendered:     []bool{false, false, true, false},
	}

	data, err := rs.MarshalRoundPersistJSON()
	require.NoError(t, err)
	restored, err := RestoreRoundFromPersistJSON(rs.roomID, data)
	require.NoError(t, err)
	require.True(t, restored.isSurrendered(2))
	require.Equal(t, []int32{1}, restored.actingSeats())
}

func TestDoGangClosesRoomAfterSettlement(t *testing.T) {
	t.Parallel()

	r := domainroom.NewRoom("r-gang-close")
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, ok := r.JoinAutoSeat(uid)
		require.True(t, ok)
	}
	for i := 0; i < 4; i++ {
		require.NoError(t, r.SetReady(domainroom.Seat(i), true))
	}
	require.NoError(t, r.StartPlaying())

	rs := &RoundState{
		roomID:         "r-gang-close",
		ruleID:         "sichuan_xuezhandaodi_huansanzhang",
		rule:           rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:      [4]string{"u0", "u1", "u2", "u3"},
		wall:           wall.NewFromOrderedTiles(nil),
		hands:          []*hand.Hand{hand.FromTiles([]tile.Tile{tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1), tile.Must(tile.SuitCharacters, 1)}), hand.New(), hand.New(), hand.New()},
		ruleState:      testRuleState(make([]int32, 4)),
		waitingDiscard: true,
		turn:           0,
	}

	a := newRoomActor(r, nil)
	a.engine = NewEngine("sichuan_xuezhandaodi_huansanzhang")
	a.round = rs

	notifs, err := a.doGang("u0", "m1")
	require.NoError(t, err)
	require.Len(t, notifs, 2)
	require.Nil(t, a.round)
	require.Equal(t, domainroom.StateClosed, r.FSM.State())
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

func driveRoundToClose(ctx context.Context, svc *Service, roomID string) error {
	for i := 0; i < 256; i++ {
		a := svc.getActor(roomID)
		if a == nil || a.round == nil {
			return nil
		}
		if a.round.waitingOpening {
			step, ok := a.round.currentOpeningStep()
			if !ok {
				return fmt.Errorf("round waiting opening without current step")
			}
			switch step.Action {
			case openingExchangeThree:
				for seat, done := range a.round.openingSubmitted(step.Action) {
					if !done {
						tiles := tilesToStrings(a.round.hands[seat].Tiles()[:3])
						if _, err := svc.OpeningAction(ctx, roomID, a.room.PlayerIDs[seat], openingExchangeThree, tiles, 0, 0, nil, nil); err != nil {
							return err
						}
					}
				}
			case openingQueMen:
				for seat, done := range a.round.openingSubmitted(step.Action) {
					if !done {
						if _, err := svc.OpeningAction(ctx, roomID, a.room.PlayerIDs[seat], openingQueMen, nil, 0, 0, nil, nil); err != nil {
							return err
						}
					}
				}
			default:
				return fmt.Errorf("unsupported opening action %s", step.Action)
			}
			continue
		}
		if claimSeat := a.round.claimSeat(); claimSeat >= 0 {
			userID := a.room.PlayerIDs[claimSeat]
			if a.round.rawCanClaimGang(claimSeat) {
				if _, err := svc.Gang(ctx, roomID, userID, a.round.lastDiscard.String(), nil); err != nil {
					if bytes.Contains([]byte(err.Error()), []byte("round closed")) {
						return nil
					}
					return err
				}
			} else {
				if _, err := svc.Pong(ctx, roomID, userID, nil); err != nil {
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
			notifs, err := svc.Hu(ctx, roomID, userID, nil)
			if err == nil && containsSettlement(notifs) {
				return nil
			}
			if err == nil {
				continue
			}
			if err != nil && !bytes.Contains([]byte(err.Error()), []byte("hu not allowed")) {
				return err
			}
		}
		//nolint:gosec // G115：queBySeat 仅在 0..2 范围（三种花色），不会溢出 byte
		discard := chooseDiscard(a.round.hands[seat], tile.Suit(a.round.missingSuitBySeat()[seat]))
		notifs, err := svc.Discard(ctx, roomID, userID, discard.String(), nil)
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
