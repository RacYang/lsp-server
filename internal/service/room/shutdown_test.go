package room

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/clock"
	eng "racoo.cn/lsp/internal/service/room/engine"
)

func newShutdownTestService(fc *clock.Fake) *Service {
	svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	svc.SetClock(fc)
	svc.SetTimeoutConfig(eng.TimeoutConfig{
		OpeningDefault: time.Second,
		OpeningByAction: map[string]time.Duration{
			openingExchangeThree: time.Second,
			openingQueMen:        time.Second,
		},
		ClaimWindow: time.Second,
		TsumoWindow: time.Second,
		Discard:     time.Second,
	})
	return svc
}

func startPlayingRoom(t *testing.T, svc *Service, roomID string) {
	t.Helper()
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, err := svc.Join(context.Background(), roomID, uid)
		require.NoError(t, err)
	}
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, err := svc.Ready(context.Background(), roomID, uid)
		require.NoError(t, err)
	}
}

// TestShutdownStopsAutoTimeoutDriving 断言优雅停机不变量：Shutdown 返回后，
// 阶段超时定时器不再驱动状态变更与持久化回调——这是「排空在途 → 停自驱动源 →
// 关存储」停机顺序中"停自驱动源"一步的契约。
func TestShutdownStopsAutoTimeoutDriving(t *testing.T) {
	t.Parallel()

	fc := clock.NewFake(time.Unix(0, 0))
	svc := newShutdownTestService(fc)
	fired := make(chan struct{}, 16)
	svc.SetAutoTimeoutHandler(func(context.Context, string, []eng.Notification) {
		fired <- struct{}{}
	})
	startPlayingRoom(t, svc, "r-shutdown")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	svc.Shutdown(shutdownCtx)

	// 推进时钟越过开局阶段 deadline；停机后定时器已停且不可再 arm，不应有任何超时回调。
	for i := 0; i < 4; i++ {
		fc.Advance(time.Second)
	}
	select {
	case <-fired:
		t.Fatal("Shutdown 之后阶段超时仍在驱动状态变更")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestShutdownStopsOfflineSurrenderTimer 断言停机后离线投降定时器不再创建：
// 停机窗口内的 MarkSeatOffline 不得触发投降状态变更（开局阶段座位 0 投降会使
// ActingSeat 跳到座位 1，因此以 ActingSeat 不变作为"未投降"的观察点）。
func TestShutdownStopsOfflineSurrenderTimer(t *testing.T) {
	t.Parallel()

	fc := clock.NewFake(time.Unix(0, 0))
	svc := newShutdownTestService(fc)
	svc.SetAllowLeaveDuringPlay(true)
	svc.SetOfflineSurrenderAfter(10 * time.Millisecond)
	startPlayingRoom(t, svc, "r-shutdown-offline")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	svc.Shutdown(shutdownCtx)

	svc.MarkSeatOffline("r-shutdown-offline", "u0")
	time.Sleep(50 * time.Millisecond)

	view, ok, err := svc.RoundView(context.Background(), "r-shutdown-offline")
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, 0, view.ActingSeat, "停机后离线投降不得继续触发状态变更")
}

// TestShutdownAfterwardsTimerNotRearmedByCommands 断言停机后在途命令的
// resetScheduler 不会重新 arm 定时器（draining 由 scheduler 自身守住）。
func TestShutdownAfterwardsTimerNotRearmedByCommands(t *testing.T) {
	t.Parallel()

	fc := clock.NewFake(time.Unix(0, 0))
	svc := newShutdownTestService(fc)
	fired := make(chan struct{}, 16)
	svc.SetAutoTimeoutHandler(func(context.Context, string, []eng.Notification) {
		fired <- struct{}{}
	})
	startPlayingRoom(t, svc, "r-shutdown-rearm")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	svc.Shutdown(shutdownCtx)

	// Shutdown 后仍可能有在途命令进入 actor（如排空边缘的请求），其 resetScheduler 不得重新 arm。
	// AutoTimeout 命令路径必然产生状态变更并触发 resetScheduler。
	_, err := svc.AutoTimeout(context.Background(), "r-shutdown-rearm")
	require.NoError(t, err)
	for i := 0; i < 8; i++ {
		fc.Advance(time.Second)
	}
	select {
	case <-fired:
		t.Fatal("Shutdown 之后命令路径重新 arm 了阶段超时定时器")
	case <-time.After(100 * time.Millisecond):
	}
}
