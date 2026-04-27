package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunWithTempConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gate.yaml")
	content := "server:\n  addr: \"127.0.0.1:0\"\nrule:\n  default_id: \"sichuan_xzdd\"\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LSP_CONFIG", cfgPath)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	if code := run(ctx, cancel); code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
}
