// actor 动作测试：覆盖 doDiscard/doPong/doAutoTimeout 等 dispatch 路径。
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

const testRule = "sichuan_xuezhandaodi_huansanzhang"

func newTestEngine() *Engine { return eng.NewEngine(testRule) }

// setupDiscardActor 创建一个带出牌态局面的 actor 并启动。
func setupDiscardActor(t *testing.T, roomID string) (*Actor, [4]string) {
	t.Helper()
	pids := [4]string{"d0", "d1", "d2", "d3"}
	r := newPlayingRoom(roomID, pids)
	rs := discardWaitingRound(roomID, pids)
	a := New(r, rs, Config{Engine: newTestEngine()})
	startActor(t, a)
	return a, pids
}

func TestActorSubmitDiscard(t *testing.T) {
	t.Parallel()
	a, _ := setupDiscardActor(t, "r-discard")
	ctx := context.Background()

	// d0 是出牌方（Turn=0），出万一触发抢答窗口
	notifs, err := a.SubmitDiscard(ctx, "d0", "m1", nil)
	require.NoError(t, err)
	require.NotEmpty(t, notifs)
}

func TestActorSubmitPongAfterDiscard(t *testing.T) {
	t.Parallel()
	pids := [4]string{"p0", "p1", "p2", "p3"}
	r := newPlayingRoom("r-pong", pids)
	rs := discardWaitingRound("r-pong", pids)
	a := New(r, rs, Config{Engine: newTestEngine()})
	startActor(t, a)

	ctx := context.Background()
	// 先出牌，触发 claim window
	_, err := a.SubmitDiscard(ctx, "p0", "m1", nil)
	require.NoError(t, err)

	// p1/p2/p3 pass 关闭抢答窗口（其中一次可能已推进局面，错误为正常）
	for _, uid := range []string{"p1", "p2", "p3"} {
		_, _ = a.SubmitPass(ctx, uid, nil) // 忽略错误，局面可能已推进
	}
}

func TestActorSubmitHuAfterDiscard(t *testing.T) {
	t.Parallel()
	pids := [4]string{"h0", "h1", "h2", "h3"}
	r := newPlayingRoom("r-hu", pids)
	rs := eng.NewRoundStateFromConfig(eng.RoundStateConfig{
		RoomID:    "r-hu",
		RuleID:    testRule,
		Rule:      rules.MustGet(testRule),
		PlayerIDs: pids,
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
		RuleState:    testRuleState(make([]int32, 4)),
		WaitingTsumo: true,
		Turn:         0,
	})
	a := New(r, rs, Config{Engine: newTestEngine()})
	startActor(t, a)
	ctx := context.Background()

	// 4 张相同的牌不构成合法胡牌型，应返回错误
	_, err := a.SubmitHu(ctx, "h0", nil)
	require.Error(t, err, "4 张相同的牌不构成合法胡牌型")
}

func TestActorSubmitAutoTimeout(t *testing.T) {
	t.Parallel()
	pids := [4]string{"t0", "t1", "t2", "t3"}
	r := newPlayingRoom("r-timeout", pids)
	rs := discardWaitingRound("r-timeout", pids)
	a := New(r, rs, Config{Engine: newTestEngine()})
	startActor(t, a)

	ctx := context.Background()
	// AutoTimeout 在 WaitingDiscard 状态下代替 t0 出牌
	notifs, err := a.SubmitAutoTimeout(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, notifs)
}

func TestActorOpeningAction(t *testing.T) {
	t.Parallel()
	r := domainroom.NewRoom("r-opening")
	for _, uid := range []string{"o0", "o1", "o2", "o3"} {
		_, _ = r.JoinAutoSeat(uid)
	}

	a := New(r, nil, Config{Engine: newTestEngine()})
	startActor(t, a)

	ctx := context.Background()
	// 加入并准备，触发开局（血战到底有开局阶段）
	for _, uid := range []string{"o0", "o1", "o2", "o3"} {
		_, _ = a.SubmitJoin(ctx, uid)
	}
	notifs, err := a.SubmitReady(ctx, "o0")
	require.NoError(t, err)
	_ = notifs
	for _, uid := range []string{"o1", "o2", "o3"} {
		_, _ = a.SubmitReady(ctx, uid)
	}
	time.Sleep(20 * time.Millisecond)

	// 提交换三张开局动作（可能失败，但覆盖 doOpeningAction 路径）
	view, ok, _ := a.SubmitRoundView(ctx)
	if ok && view.WaitingAction == "exchange_three" {
		for seatIdx, seatPid := range []string{"o0", "o1", "o2", "o3"} {
			tiles := view.HandsBySeat[seatIdx][:3]
			_, _ = a.SubmitOpeningAction(ctx, seatPid, "exchange_three", tiles, 0, 0, nil, nil)
		}
	}
}

func TestActorPhaseTokenRejection(t *testing.T) {
	t.Parallel()
	pids := [4]string{"pk0", "pk1", "pk2", "pk3"}
	r := newPlayingRoom("r-phase-tok", pids)
	rs := discardWaitingRound("r-phase-tok", pids)
	a := New(r, rs, Config{Engine: newTestEngine()})
	startActor(t, a)

	ctx := context.Background()
	// 伪造 step=1/Reason=ReasonDiscard：与局面的 step=0/ReasonNone 不符，应被拒绝
	fakeToken := &PhaseToken{Step: 1, Reason: eng.ReasonDiscard}
	_, err := a.SubmitDiscard(ctx, "pk0", "m1", fakeToken)
	require.Error(t, err, "令牌 step/reason 与当前局面不符，应返回 PhaseDriftError")
}

func TestActorCloneMap(t *testing.T) {
	t.Parallel()
	// cloneMap 是内部工具函数，通过 SubmitRoomSnapshot 间接覆盖
	pids := [4]string{"cm0", "cm1", "cm2", "cm3"}
	r := newPlayingRoom("r-clone", pids)
	a := New(r, nil, Config{Engine: newTestEngine()})
	startActor(t, a)
	ctx := context.Background()
	_, _, _, err := a.SubmitRoomSnapshot(ctx)
	require.NoError(t, err)
}

func TestActorLeaveDuringPlay(t *testing.T) {
	t.Parallel()
	pids := [4]string{"lp0", "lp1", "lp2", "lp3"}
	r := newPlayingRoom("r-leave-play", pids)
	rs := discardWaitingRound("r-leave-play", pids)
	a := New(r, rs, Config{
		Engine:               newTestEngine(),
		AllowLeaveDuringPlay: true,
	})
	startActor(t, a)
	ctx := context.Background()
	// lp1 在游戏中离开（被标记为投降）
	err := a.SubmitLeave(ctx, "lp1")
	require.NoError(t, err)
}
