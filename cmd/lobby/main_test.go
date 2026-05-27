package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"racoo.cn/lsp/internal/cluster"
)

func writeLobbyCfg(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunWithCanceledEtcdContext(t *testing.T) {
	cfgPath := writeLobbyCfg(t, "lobby-etcd.yaml",
		"server:\n  addr: \"127.0.0.1:0\"\nrule:\n  default_id: \"sichuan_xuezhandaodi_huansanzhang\"\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\netcd:\n  endpoints: \"127.0.0.1:1\"\n")
	t.Setenv("LSP_CONFIG", cfgPath)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := run(ctx, cancel); code == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

// TestRunConfigLoadFails 验证配置加载失败时 run 返回非零退出码。
func TestRunConfigLoadFails(t *testing.T) {
	t.Setenv("LSP_CONFIG", "/nonexistent/path/lobby.yaml")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if code := run(ctx, cancel); code == 0 {
		t.Fatal("配置加载失败时应返回非零退出码")
	}
}

// TestRunNoExternalDepsCleanShutdown 验证无 Redis/etcd 时大厅服务正常启动并响应 ctx 取消。
func TestRunNoExternalDepsCleanShutdown(t *testing.T) {
	cfgPath := writeLobbyCfg(t, "lobby-local.yaml",
		"server:\n  addr: \"127.0.0.1:0\"\nrule:\n  default_id: \"\"\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\n")
	t.Setenv("LSP_CONFIG", cfgPath)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消，触发正常关闭路径
	if code := run(ctx, cancel); code != 0 {
		t.Fatalf("正常关闭应返回 0，got %d", code)
	}
}

func TestParseEndpoints(t *testing.T) {
	// 验证 cluster.ParseEndpoints 正确处理空白与空项（逻辑已移至 internal/cluster）。
	got := cluster.ParseEndpoints(" a, ,b ")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("%+v", got)
	}
}
