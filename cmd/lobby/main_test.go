package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWithCanceledEtcdContext(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "lobby-etcd.yaml")
	content := "server:\n  addr: \"127.0.0.1:0\"\nrule:\n  default_id: \"sichuan_xzdd\"\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\netcd:\n  endpoints: \"127.0.0.1:1\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LSP_CONFIG", cfgPath)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := run(ctx, cancel); code == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestSplitEtcdEndpoints(t *testing.T) {
	got := splitEtcdEndpoints(" a, ,b ")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("%+v", got)
	}
}
