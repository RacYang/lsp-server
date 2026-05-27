package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
