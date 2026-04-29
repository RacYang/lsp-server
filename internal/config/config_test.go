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
	content := "server:\n  addr: \":19999\"\n  ws_allowed_origins:\n    - \"https://trusted.example\"\nrule:\n  default_id: \"sichuan_xzdd\"\nruntime:\n  gate:\n    ws_rate_limit_per_second: 7\n    ws_rate_limit_burst: 9\n    ws_idempotency_cache: 11\n  room:\n    mailbox_capacity: 13\n  redis:\n    idempotency_ttl: 2m\n  logging:\n    level: debug\n    format: console\n    otel_enabled: true\n    otel_endpoint: \"localhost:4317\"\n    dynamic_level: true\n    sample:\n      initial: 3\n      thereafter: 5\n      tick: 2s\n      error_never: true\n"
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
	if cfg.Runtime.Logging.Level != "debug" ||
		cfg.Runtime.Logging.Format != "console" ||
		!cfg.Runtime.Logging.OTelEnabled ||
		cfg.Runtime.Logging.OTelEndpoint != "localhost:4317" ||
		!cfg.Runtime.Logging.DynamicLevel ||
		cfg.Runtime.Logging.Sample.Initial != 3 ||
		cfg.Runtime.Logging.Sample.Thereafter != 5 ||
		cfg.Runtime.Logging.Sample.Tick.String() != "2s" ||
		!cfg.Runtime.Logging.Sample.ErrorNever {
		t.Fatalf("%+v", cfg.Runtime.Logging)
	}
}
