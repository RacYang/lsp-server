package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/cluster"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
)

func writeRoomCfg(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestRunConfigLoadFails 验证配置加载失败时 run 返回非零退出码。
func TestRunConfigLoadFails(t *testing.T) {
	t.Setenv("LSP_CONFIG", "/nonexistent/path/room.yaml")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if code := run(ctx, cancel); code == 0 {
		t.Fatal("配置加载失败时应返回非零退出码")
	}
}

// TestRunNoExternalDepsCleanShutdown 验证无 Postgres/Redis/etcd 时房间服务正常启动并响应 ctx 取消。
func TestRunNoExternalDepsCleanShutdown(t *testing.T) {
	cfgPath := writeRoomCfg(t, "room-local.yaml",
		"server:\n  addr: \"127.0.0.1:0\"\nrule:\n  default_id: \"\"\n")
	t.Setenv("LSP_CONFIG", cfgPath)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消，触发正常关闭路径
	if code := run(ctx, cancel); code != 0 {
		t.Fatalf("正常关闭应返回 0，got %d", code)
	}
}

// TestRunWithRedisNoEtcdCleanShutdown 验证配置了 Redis（但无 etcd）时正常关闭返回 0。
// go-redis 客户端创建为懒连接，不依赖 Redis 实际可用。
func TestRunWithRedisNoEtcdCleanShutdown(t *testing.T) {
	cfgPath := writeRoomCfg(t, "room-redis-local.yaml",
		"server:\n  addr: \"127.0.0.1:0\"\nrule:\n  default_id: \"\"\nredis:\n  addr: \"127.0.0.1:9999\"\n")
	t.Setenv("LSP_CONFIG", cfgPath)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := run(ctx, cancel); code != 0 {
		t.Fatalf("正常关闭应返回 0，got %d", code)
	}
}

// TestRunEtcdMissingRedis 验证 etcd 启用时缺少 Redis 返回非零退出码。
func TestRunEtcdMissingRedis(t *testing.T) {
	cfgPath := writeRoomCfg(t, "room-etcd-nored.yaml",
		"server:\n  addr: \"127.0.0.1:0\"\nrule:\n  default_id: \"\"\netcd:\n  endpoints: \"127.0.0.1:1\"\n")
	t.Setenv("LSP_CONFIG", cfgPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if code := run(ctx, cancel); code == 0 {
		t.Fatal("etcd 配置缺少 Redis 时应返回非零退出码")
	}
}

// TestRecoverOwnedRoomsNilInputs 验证 nil 参数时 recoverOwnedRooms 安全返回。
func TestRecoverOwnedRoomsNilInputs(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, recoverOwnedRooms(ctx, nil, "room-local", nil, nil, nil, nil))
}

// TestRecoverOwnedRoomsEtcdCliNil 验证 etcd client 内部为 nil 时返回错误。
// cluster.NewEtcdRouter(nil, ...) 构造非 nil 的 *cluster.EtcdRouter，但 ListRoomsByOwner 会返回错误。
func TestRecoverOwnedRoomsEtcdCliNil(t *testing.T) {
	ctx := context.Background()
	rt := cluster.NewEtcdRouter(nil, "/lsp") // 非 nil 的 Etcd，但内部 cli 为 nil
	rcli, err := redis.NewClient("127.0.0.1:9999")
	require.NoError(t, err)
	svc := roomsvc.NewServiceWithRule(roomsvc.NewLobby(), "")
	err = recoverOwnedRooms(ctx, rt, "room-local", rcli, nil, nil, svc)
	require.Error(t, err, "etcd cli 为 nil 时应返回错误")
}

// TestDeriveRecoveredState 验证事件行推导恢复状态的完整分支。
func TestDeriveRecoveredState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		rows    []postgres.RoomEventRow
		want    string
	}{
		{
			name:    "空事件列表不变",
			current: "waiting",
			rows:    nil,
			want:    "waiting",
		},
		{
			name:    "start_game 推导 playing",
			current: "ready",
			rows:    []postgres.RoomEventRow{{Kind: string(roomsvc.KindStartGame)}},
			want:    "playing",
		},
		{
			name:    "settlement 推导 closed",
			current: "ready",
			rows:    []postgres.RoomEventRow{{Kind: string(roomsvc.KindSettlement)}},
			want:    "closed",
		},
		{
			name:    "连续事件取最后状态",
			current: "ready",
			rows: []postgres.RoomEventRow{
				{Kind: string(roomsvc.KindStartGame)},
				{Kind: string(roomsvc.KindSettlement)},
			},
			want: "closed",
		},
		{
			name:    "opening_done 推导 playing",
			current: "waiting",
			rows:    []postgres.RoomEventRow{{Kind: string(roomsvc.KindOpeningDone)}},
			want:    "playing",
		},
		{
			name:    "draw_tile 推导 playing",
			current: "waiting",
			rows:    []postgres.RoomEventRow{{Kind: string(roomsvc.KindDrawTile)}},
			want:    "playing",
		},
		{
			name:    "action 推导 playing",
			current: "waiting",
			rows:    []postgres.RoomEventRow{{Kind: string(roomsvc.KindAction)}},
			want:    "playing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := deriveRecoveredState(tt.current, tt.rows)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSplitEndpointsTrimsEmptyParts(t *testing.T) {
	t.Parallel()

	got := splitEndpoints(" http://etcd-0:2379, ,http://etcd-1:2379 ")

	require.Equal(t, []string{"http://etcd-0:2379", "http://etcd-1:2379"}, got)
}
