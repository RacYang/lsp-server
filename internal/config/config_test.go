// 配置加载单元测试：从临时 YAML 读取字段。
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTempFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	content := "server:\n  addr: \":19999\"\n  ws_allowed_origins:\n    - \"https://trusted.example\"\nrule:\n  default_id: \"sichuan_xzdd\"\nruntime:\n  gate:\n    ws_rate_limit_per_second: 7\n    ws_rate_limit_burst: 9\n    ws_idempotency_cache: 11\n  room:\n    mailbox_capacity: 13\n  redis:\n    idempotency_ttl: 2m\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerAddr != ":19999" || cfg.RuleID != "sichuan_xzdd" {
		t.Fatalf("%+v", cfg)
	}
	if len(cfg.WSAllowedOrigins) != 1 || cfg.WSAllowedOrigins[0] != "https://trusted.example" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Runtime.GateWSRateLimitPerSecond != 7 ||
		cfg.Runtime.GateWSRateLimitBurst != 9 ||
		cfg.Runtime.GateWSIdempotencyCache != 11 ||
		cfg.Runtime.RoomMailboxCapacity != 13 ||
		cfg.Runtime.RedisIdempotencyTTL.String() != "2m0s" {
		t.Fatalf("%+v", cfg.Runtime)
	}
}
