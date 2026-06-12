// actor 优雅停机测试：覆盖 Shutdown 命令路径、scheduler draining 与定时器拦截。
package actor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainroom "racoo.cn/lsp/internal/domain/room"
)

// TestActorShutdownStopsTimersAndBlocksRearm 断言停机不变量：Shutdown 返回后
// 离线投降定时器不再创建、scheduler 不再 arm、在途 fire 直接短路。
func TestActorShutdownStopsTimersAndBlocksRearm(t *testing.T) {
	t.Parallel()

	playerIDs := [4]string{"u0", "u1", "u2", "u3"}
	r := newPlayingRoom("r-shutdown-actor", playerIDs)
	a := New(r, discardWaitingRound("r-shutdown-actor", playerIDs), Config{
		Engine:                newTestEngine(),
		AllowLeaveDuringPlay:  true,
		OfflineSurrenderAfter: 5 * time.Millisecond,
	})
	startActor(t, a)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, a.Shutdown(ctx))

	// 停机后离线标记不得再创建投降定时器（ensureOfflineTimer 的 draining 守卫）。
	a.SubmitMarkOffline("u0")
	time.Sleep(30 * time.Millisecond)
	view, ok, err := a.SubmitRoundView(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, 0, view.ActingSeat, "停机后离线投降不得驱动状态变更")

	// 停机后到达的命令路径（resetScheduler→armUntil）不得重新 arm 定时器；
	// Ready 在 playing 态返回业务错误但仍会走 resetScheduler，借此覆盖 draining 守卫。
	_, _ = a.SubmitReady(ctx, "u0")
	require.True(t, a.scheduler.draining, "draining 置位后不可被命令路径清除")
	a.scheduler.mu.Lock()
	require.Nil(t, a.scheduler.timer, "draining 后不得存在已 arm 的定时器")
	a.scheduler.mu.Unlock()

	// 已在途的 fire 在 draining 后必须直接短路（不提交 AutoTimeout）。
	a.scheduler.fire()

	// 重复 Shutdown 幂等。
	require.NoError(t, a.Shutdown(ctx))
}

// TestActorShutdownClosedActorIsNoop 断言已关闭房间的 Shutdown 直接返回：
// 关闭分支已清理全部定时器，停机语义天然达成。
func TestActorShutdownClosedActorIsNoop(t *testing.T) {
	t.Parallel()

	r := domainroom.NewRoom("r-shutdown-closed")
	a := New(r, nil, Config{Engine: newTestEngine()})
	a.closed.Store(true)
	require.NoError(t, a.Shutdown(context.Background()))

	var nilActor *Actor
	require.NoError(t, nilActor.Shutdown(context.Background()))
}

// TestActorShutdownContextCancelled 断言 actor 循环未消费时 Shutdown 以 ctx 超时退出，
// 不无限阻塞进程停机流程。
func TestActorShutdownContextCancelled(t *testing.T) {
	t.Parallel()

	r := domainroom.NewRoom("r-shutdown-ctx")
	a := New(r, nil, Config{Engine: newTestEngine()})
	// 故意不启动 Run()：命令入队后无人消费，等待确认必须由 ctx 解除。
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, a.Shutdown(ctx), context.DeadlineExceeded)
}

// TestRejectPendingShutdownTimers 断言房间关闭排空时挂起的停机命令会被确认，
// 等待方不会悬挂。
func TestRejectPendingShutdownTimers(t *testing.T) {
	t.Parallel()

	r := domainroom.NewRoom("r-shutdown-drain")
	a := New(r, nil, Config{Engine: newTestEngine()})
	res := make(chan struct{}, 1)
	a.rejectPendingMsg(cmdShutdownTimers{res: res})
	select {
	case <-res:
	default:
		t.Fatal("排空路径应确认挂起的停机命令")
	}
}

// TestRunExitsWhenChannelClosedDuringDrain 断言关闭分支排空循环对"信道已被外部
// 关闭"健壮：Run 必须退出，而不是从已关闭信道无限读零值自旋（CI 曾以 10 分钟
// 包超时暴露：房间因末次命令关闭与测试 cleanup 关闭信道竞态时命中）。
func TestRunExitsWhenChannelClosedDuringDrain(t *testing.T) {
	t.Parallel()

	r := domainroom.NewRoom("r-drain-closed-ch")
	// CloseRoom 在非 settling 态是静默空操作，这里直接做 waiting→closed 的合法迁移。
	require.NoError(t, r.FSM.Transition(domainroom.StateClosed))
	require.Equal(t, domainroom.StateClosed, r.FSM.State())
	a := New(r, nil, Config{Engine: newTestEngine()})

	// 预投递一条命令并立即关闭信道：Run 处理该命令后进入关闭分支排空，
	// 此时信道已关闭且为空，缺少 comma-ok 检查的排空实现会无限自旋。
	res := make(chan joinResult, 1)
	a.ch <- cmdJoin{userID: "u1", res: res}
	close(a.ch)

	done := make(chan struct{})
	go func() {
		a.Run()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run 在信道关闭后未退出：关闭分支排空循环自旋")
	}
}
