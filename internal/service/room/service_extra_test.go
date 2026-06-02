// service_extra_test.go — 补充房间服务层覆盖率偏低路径的单元测试。
package room

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServiceSetters(t *testing.T) {
	t.Parallel()
	svc := NewService(NewRoomRegistry())

	// 测试最大局数设置
	svc.SetMaxHands(3)
	require.EqualValues(t, 3, svc.maxHands)
	svc.SetMaxHands(0) // 非正值回退为 1
	require.EqualValues(t, 1, svc.maxHands)

	// 测试离线投降延迟设置
	svc.SetOfflineSurrenderAfter(10 * time.Second)
	require.Equal(t, 10*time.Second, svc.offlineSurrenderAfter)
	svc.SetOfflineSurrenderAfter(0) // 非正值忽略
	require.Equal(t, 10*time.Second, svc.offlineSurrenderAfter)

	// 测试命令后回调注册
	svc.SetAfterCmdHook(func(_ string) {})
	require.NotNil(t, svc.onAfterCmd)
}

func TestServiceNilReceivers(t *testing.T) {
	t.Parallel()
	var svc *Service
	svc.SetClock(nil)
	svc.SetMaxHands(1)
	svc.SetOfflineSurrenderAfter(time.Second)
	svc.SetAfterCmdHook(nil)
	svc.SetAllowLeaveDuringPlay(true)
	svc.SetAutoTimeoutHandler(nil)
	svc.MarkSeatOffline("r", "u")
	svc.CancelOfflineSurrender("r", "u")
	require.Equal(t, 0, svc.ActiveRoomCount())
	_, ok := svc.PlayerIDs("r")
	require.False(t, ok)
}

func TestServicePlayerIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService(NewRoomRegistry())
	require.NoError(t, svc.EnsureRoom("r-pids"))
	_, _ = svc.Join(ctx, "r-pids", "p0")

	ids, ok := svc.PlayerIDs("r-pids")
	require.True(t, ok)
	require.Equal(t, "p0", ids[0])

	_, ok = svc.PlayerIDs("nonexistent")
	require.False(t, ok)
}

func TestServiceActiveRoomCount(t *testing.T) {
	t.Parallel()
	svc := NewService(NewRoomRegistry())
	require.Equal(t, 0, svc.ActiveRoomCount())
	require.NoError(t, svc.EnsureRoom("r-count"))
	require.Equal(t, 1, svc.ActiveRoomCount())
}

func TestServiceMarkOfflineAndCancel(t *testing.T) {
	t.Parallel()
	svc := NewService(NewRoomRegistry())
	ctx := context.Background()
	_, _ = svc.Join(ctx, "r-offline", "u-offline")

	// fire-and-forget — 不 panic 即可
	svc.MarkSeatOffline("r-offline", "u-offline")
	svc.CancelOfflineSurrender("r-offline", "u-offline")
	svc.MarkSeatOffline("nonexistent", "u")
	svc.MarkSeatOffline("r-offline", "")
}

func TestServiceChiGangHuPass(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	ctx := context.Background()

	// 路由层覆盖：actor 不存在时均返回错误
	_, err := svc.Chi(ctx, "no-room", "u", nil, nil)
	require.Error(t, err)
	_, err = svc.Gang(ctx, "no-room", "u", "", nil)
	require.Error(t, err)
	_, err = svc.Hu(ctx, "no-room", "u", nil)
	require.Error(t, err)
	_, err = svc.Pass(ctx, "no-room", "u", nil)
	require.Error(t, err)
}

func TestServiceEnsureRoomIdempotent(t *testing.T) {
	t.Parallel()
	svc := NewService(NewRoomRegistry())
	require.NoError(t, svc.EnsureRoom("r-idem2"))
	require.NoError(t, svc.EnsureRoom("r-idem2")) // 已存在，走 ensureActorForExistingRoom
	require.Equal(t, 1, svc.ActiveRoomCount())
}

func TestServiceOpeningActionNoRoom(t *testing.T) {
	t.Parallel()
	svc := NewServiceWithRule(NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	ctx := context.Background()
	_, err := svc.OpeningAction(ctx, "no-room", "u", "exchange_three", nil, 0, 0, nil, nil)
	require.Error(t, err)
}
