// actor 生命周期集成测试：通过 Submit* 公开 API 驱动 Run() 事件循环。
package actor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	_ "racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
	eng "racoo.cn/lsp/internal/service/room/engine"
)

// newPlayingRoom 创建已进入 playing 态的四人房间。
func newPlayingRoom(roomID string, playerIDs [4]string) *domainroom.Room {
	r := domainroom.NewRoom(roomID)
	for _, uid := range playerIDs {
		_, _ = r.JoinAutoSeat(uid)
	}
	for i := 0; i < 4; i++ {
		_ = r.SetReady(domainroom.Seat(i), true)
	}
	_ = r.StartPlaying()
	return r
}

// discardWaitingRound 创建等待出牌态的 RoundState（跳过开局阶段）。
func discardWaitingRound(roomID string, playerIDs [4]string) *RoundState {
	return eng.NewRoundStateFromConfig(eng.RoundStateConfig{
		RoomID:    roomID,
		RuleID:    testRule,
		Rule:      rules.MustGet(testRule),
		PlayerIDs: playerIDs,
		Wall:      wall.NewFromOrderedTiles(nil),
		Hands: []*hand.Hand{
			hand.FromTiles([]tile.Tile{
				tile.Must(tile.SuitCharacters, 1),
				tile.Must(tile.SuitCharacters, 1),
				tile.Must(tile.SuitCharacters, 1),
				tile.Must(tile.SuitCharacters, 1),
			}),
			hand.New(), hand.New(), hand.New(),
		},
		RuleState:      testRuleState(make([]int32, 4)),
		WaitingDiscard: true,
		Turn:           0,
	})
}

// startActor 启动 actor 的 Run() 事件循环，并注册 t.Cleanup 保证 goroutine 在测试结束前退出。
func startActor(t *testing.T, a *Actor) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		a.Run()
		close(done)
	}()
	t.Cleanup(func() {
		a.submitMu.Lock()
		if !a.closed.Load() {
			a.closed.Store(true)
			close(a.ch)
		}
		a.submitMu.Unlock()
		<-done
	})
}

func TestActorNewAndMailboxCap(t *testing.T) {
	t.Parallel()
	r := domainroom.NewRoom("r1")
	a := New(r, nil, Config{Capacity: 16})
	require.NotNil(t, a)
	require.Equal(t, 16, a.MailboxCap())

	// nil room
	require.Nil(t, New(nil, nil, Config{}))

	// 零容量回退默认
	a2 := New(r, nil, Config{})
	require.Equal(t, DefaultMailboxCapacity, a2.MailboxCap())

	// nil actor
	var nilA *Actor
	require.Equal(t, 0, nilA.MailboxCap())
}

func TestActorJoinAndLeave(t *testing.T) {
	t.Parallel()
	r := domainroom.NewRoom("r-join-leave")
	a := New(r, nil, Config{Engine: newTestEngine()})
	startActor(t, a)

	ctx := context.Background()

	seat, err := a.SubmitJoin(ctx, "u1")
	require.NoError(t, err)
	require.GreaterOrEqual(t, seat, 0)

	// 再次加入（已在房间，playing 前应返回座位）
	seat2, err := a.SubmitJoin(ctx, "u1")
	require.NoError(t, err)
	require.Equal(t, seat, seat2)

	err = a.SubmitLeave(ctx, "u1")
	require.NoError(t, err)
}

func TestActorRoomSnapshot(t *testing.T) {
	t.Parallel()
	pids := [4]string{"s0", "s1", "s2", "s3"}
	r := newPlayingRoom("r-snap", pids)
	a := New(r, nil, Config{Engine: newTestEngine()})
	startActor(t, a)

	ctx := context.Background()
	players, state, ready, err := a.SubmitRoomSnapshot(ctx)
	require.NoError(t, err)
	require.Equal(t, "playing", state)
	require.Len(t, players, 4)
	require.Len(t, ready, 4)
	_ = players
}

func TestActorRoundViewNoRound(t *testing.T) {
	t.Parallel()
	r := domainroom.NewRoom("r-noround")
	a := New(r, nil, Config{Engine: newTestEngine()})
	startActor(t, a)

	ctx := context.Background()
	_, ok, err := a.SubmitRoundView(ctx)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestActorRoundViewWithRound(t *testing.T) {
	t.Parallel()
	pids := [4]string{"v0", "v1", "v2", "v3"}
	r := newPlayingRoom("r-view", pids)
	rs := discardWaitingRound("r-view", pids)

	a := New(r, rs, Config{Engine: newTestEngine()})
	startActor(t, a)

	ctx := context.Background()
	view, ok, err := a.SubmitRoundView(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "discard", view.WaitingAction)
}

func TestActorRoundSnapJSON(t *testing.T) {
	t.Parallel()
	pids := [4]string{"j0", "j1", "j2", "j3"}
	r := newPlayingRoom("r-snap-json", pids)
	rs := discardWaitingRound("r-snap-json", pids)

	a := New(r, rs, Config{Engine: newTestEngine()})
	startActor(t, a)

	ctx := context.Background()
	data, err := a.SubmitRoundSnapJSON(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// 无局面时返回 nil
	r2 := domainroom.NewRoom("r-no-json")
	a2 := New(r2, nil, Config{Engine: newTestEngine()})
	startActor(t, a2)
	data2, err := a2.SubmitRoundSnapJSON(ctx)
	require.NoError(t, err)
	require.Nil(t, data2)
}

func TestActorGangClosesRoom(t *testing.T) {
	t.Parallel()
	pids := [4]string{"g0", "g1", "g2", "g3"}
	r := newPlayingRoom("r-gang2", pids)
	rs := discardWaitingRound("r-gang2", pids)

	exited := make(chan string, 1)
	a := New(r, rs, Config{
		Engine: newTestEngine(),
		OnExit: func(id string) { exited <- id },
	})
	startActor(t, a)

	ctx := context.Background()
	notifs, err := a.SubmitGang(ctx, "g0", "m1", nil)
	require.NoError(t, err)
	require.NotEmpty(t, notifs)

	select {
	case id := <-exited:
		require.Equal(t, "r-gang2", id)
	case <-time.After(2 * time.Second):
		t.Fatal("OnExit 未被调用")
	}
}

func TestActorSubmitReady(t *testing.T) {
	t.Parallel()
	r := domainroom.NewRoom("r-ready-test")
	for _, uid := range []string{"r0", "r1", "r2", "r3"} {
		_, _ = r.JoinAutoSeat(uid)
	}
	a := New(r, nil, Config{Engine: newTestEngine()})
	startActor(t, a)

	ctx := context.Background()
	// 前三人 ready（未开局，无通知）
	for _, uid := range []string{"r0", "r1", "r2"} {
		notifs, err := a.SubmitReady(ctx, uid)
		require.NoError(t, err)
		require.Empty(t, notifs)
	}
	// 第四人 ready，触发开局
	notifs, err := a.SubmitReady(ctx, "r3")
	require.NoError(t, err)
	require.NotEmpty(t, notifs, "开局应产生通知")
}

func TestActorMarkAndCancelOffline(t *testing.T) {
	t.Parallel()
	r := domainroom.NewRoom("r-offline")
	_, _ = r.JoinAutoSeat("u-offline")
	a := New(r, nil, Config{
		Engine:                newTestEngine(),
		OfflineSurrenderAfter: 30 * time.Second,
	})
	startActor(t, a)

	// fire-and-forget，只验证不 panic
	a.SubmitMarkOffline("u-offline")
	a.SubmitCancelOffline("u-offline")
}

func TestActorClosedRejectsJoin(t *testing.T) {
	t.Parallel()
	r := domainroom.NewRoom("r-closed")
	a := New(r, nil, Config{Engine: newTestEngine()})
	done := make(chan struct{})
	go func() {
		a.Run()
		close(done)
	}()

	// 正确关闭顺序：先在 submitMu 保护下置 closed，再关闭 channel，最后等待 Run() 退出。
	a.submitMu.Lock()
	a.closed.Store(true)
	close(a.ch)
	a.submitMu.Unlock()
	<-done

	ctx := context.Background()
	_, err := a.SubmitJoin(ctx, "u")
	require.Error(t, err)
}

func TestActorContextCancelDuringJoin(t *testing.T) {
	t.Parallel()
	r := domainroom.NewRoom("r-ctx")
	a := New(r, nil, Config{
		Engine:   newTestEngine(),
		Capacity: 0, // 零容量回退 DefaultMailboxCapacity，SubmitJoin 阻塞在 <-res
	})
	// 不启动 Run()，让命令无人消费

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := a.SubmitJoin(ctx, "u")
	require.Error(t, err)
}

func TestActorNilSubmits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var a *Actor

	// nil actor 不 panic
	_, err := a.SubmitJoin(ctx, "u")
	require.Error(t, err)
	_, err = a.SubmitReady(ctx, "u")
	require.Error(t, err)
	err = a.SubmitLeave(ctx, "u")
	require.Error(t, err)
	a.SubmitMarkOffline("u")
	a.SubmitCancelOffline("u")
}

func TestActorOnAfterCmdHook(t *testing.T) {
	t.Parallel()
	r := domainroom.NewRoom("r-hook")
	_, _ = r.JoinAutoSeat("u-hook")
	called := make(chan string, 1)
	a := New(r, nil, Config{
		Engine:     newTestEngine(),
		OnAfterCmd: func(id string) { called <- id },
	})
	startActor(t, a)

	ctx := context.Background()
	_, _ = a.SubmitJoin(ctx, "extra")
	select {
	case id := <-called:
		require.NotEmpty(t, id)
	case <-time.After(time.Second):
		t.Fatal("OnAfterCmd 未触发")
	}
}
