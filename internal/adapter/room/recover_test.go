package roomadapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/cluster"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
)

// TestRecoverOwnedRoomsNilInputs 验证 nil 参数时 RecoverOwnedRooms 安全返回，不发生 panic。
func TestRecoverOwnedRoomsNilInputs(t *testing.T) {
	ctx := context.Background()
	// rt、rcli、svc 均为 nil，函数应静默返回 nil 而非崩溃。
	require.NoError(t, RecoverOwnedRooms(ctx, nil, "room-local", nil, nil, nil, nil))
}

// TestRecoverOwnedRoomsEtcdCliNil 验证 etcd 客户端内部指针为 nil 时返回错误。
// cluster.NewEtcdRouter(nil, ...) 会构造非 nil 的 *cluster.EtcdRouter，
// 但调用 ListRoomsByOwner 时内部 cli 为 nil，应返回错误而非 panic。
func TestRecoverOwnedRoomsEtcdCliNil(t *testing.T) {
	ctx := context.Background()
	rt := cluster.NewEtcdRouter(nil, "/lsp") // 外层非 nil，内部 cli 为 nil
	rcli, err := redis.NewClient("127.0.0.1:9999")
	require.NoError(t, err)
	svc := roomsvc.NewServiceWithRule(roomsvc.NewRoomRegistry(), "")
	err = RecoverOwnedRooms(ctx, rt, "room-local", rcli, nil, nil, svc)
	require.Error(t, err, "etcd 客户端内部为 nil 时应返回错误")
}

// TestDeriveRecoveredState 验证依据持久化事件行推导恢复状态的完整分支。
// 覆盖：空列表保持原状、各事件类型向 playing/closed 的状态迁移、连续事件取最后状态、
// 以及 settlement 后出现开局事件时 clearRound 为 true。
func TestDeriveRecoveredState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		current   string
		rows      []postgres.RoomEventRow
		wantState string
		wantClear bool
	}{
		{
			name:      "空事件列表不变",
			current:   "waiting",
			rows:      nil,
			wantState: "waiting",
			wantClear: false,
		},
		{
			name:      "start_game 推导为 playing",
			current:   "ready",
			rows:      []postgres.RoomEventRow{{Kind: string(roomsvc.KindStartGame)}},
			wantState: "playing",
			wantClear: false,
		},
		{
			name:      "settlement 推导为 closed",
			current:   "ready",
			rows:      []postgres.RoomEventRow{{Kind: string(roomsvc.KindSettlement)}},
			wantState: "closed",
			wantClear: false,
		},
		{
			name:    "连续事件取最后状态",
			current: "ready",
			rows: []postgres.RoomEventRow{
				{Kind: string(roomsvc.KindStartGame)},
				{Kind: string(roomsvc.KindSettlement)},
			},
			wantState: "closed",
			wantClear: false,
		},
		{
			name:    "settlement 后开局表示新一局 clearRound 为 true",
			current: "ready",
			rows: []postgres.RoomEventRow{
				{Kind: string(roomsvc.KindStartGame)},
				{Kind: string(roomsvc.KindSettlement)},
				{Kind: string(roomsvc.KindStartGame)},
			},
			wantState: "playing",
			wantClear: true,
		},
		{
			name:      "opening_done 推导为 playing",
			current:   "waiting",
			rows:      []postgres.RoomEventRow{{Kind: string(roomsvc.KindOpeningDone)}},
			wantState: "playing",
			wantClear: false,
		},
		{
			name:      "draw_tile 推导为 playing",
			current:   "waiting",
			rows:      []postgres.RoomEventRow{{Kind: string(roomsvc.KindDrawTile)}},
			wantState: "playing",
			wantClear: false,
		},
		{
			name:      "action 推导为 playing",
			current:   "waiting",
			rows:      []postgres.RoomEventRow{{Kind: string(roomsvc.KindAction)}},
			wantState: "playing",
			wantClear: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotState, gotClear := DeriveRecoveredState(tt.current, tt.rows)
			require.Equal(t, tt.wantState, gotState)
			require.Equal(t, tt.wantClear, gotClear)
		})
	}
}

// TestRecoverSingleRoomNilRcli 验证 rcli 为 nil 时错误正确向上传播，不崩溃。
// redis.Client 对 nil 接收者有保护，返回 "nil redis client" 错误。
func TestRecoverSingleRoomNilRcli(t *testing.T) {
	t.Parallel()
	svc := roomsvc.NewServiceWithRule(roomsvc.NewRoomRegistry(), "")
	err := RecoverSingleRoom(context.Background(), nil, nil, nil, svc, "room-nil-rcli")
	require.Error(t, err)
}

// TestRecoverSingleRoomUnreachableRedis 验证 Redis 不可达时连接错误被正确向上传播。
// 不可达地址下 GetRoomSnapMeta 返回网络错误，RecoverSingleRoom 不应静默忽略。
func TestRecoverSingleRoomUnreachableRedis(t *testing.T) {
	t.Parallel()
	rcli, err := redis.NewClient("127.0.0.1:9999")
	require.NoError(t, err)
	svc := roomsvc.NewServiceWithRule(roomsvc.NewRoomRegistry(), "")
	err = RecoverSingleRoom(context.Background(), rcli, nil, nil, svc, "room-unreachable")
	require.Error(t, err)
}
