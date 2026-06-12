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
	content := "server:\n  addr: \":19999\"\n  ws_allowed_origins:\n    - \"https://trusted.example\"\nrule:\n  default_id: \"sichuan_xuezhandaodi_huansanzhang\"\ncluster:\n  advertise_addr: \"room:19082\"\nruntime:\n  gate:\n    ws_rate_limit_per_second: 7\n    ws_rate_limit_burst: 9\n    ws_idempotency_cache: 11\n  room:\n    mailbox_capacity: 13\n    surrender_action_timeout: 1500ms\n    allow_leave_during_play: true\n  lobby:\n    bot_supervisor_enabled: true\n    max_bots_per_room: 2\n  redis:\n    idempotency_ttl: 2m\n  postgres:\n    pool:\n      max_conns: 17\n      min_conns: 2\n      max_conn_lifetime: 45m\n      max_conn_idle_time: 5m\n      health_check_period: 30s\n  logging:\n    level: debug\n    format: console\n    otel_enabled: true\n    otel_endpoint: \"localhost:4317\"\n    dynamic_level: true\n    sample:\n      initial: 3\n      thereafter: 5\n      tick: 2s\n      error_never: true\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerAddr != ":19999" || cfg.RuleID != "sichuan_xuezhandaodi_huansanzhang" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.ClusterAdvertiseAddr != "room:19082" {
		t.Fatalf("%+v", cfg)
	}
	if len(cfg.WSAllowedOrigins) != 1 || cfg.WSAllowedOrigins[0] != "https://trusted.example" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Runtime.GateWSRateLimitPerSecond != 7 ||
		cfg.Runtime.GateWSRateLimitBurst != 9 ||
		cfg.Runtime.GateWSIdempotencyCache != 11 ||
		cfg.Runtime.RoomMailboxCapacity != 13 ||
		cfg.Runtime.RoomSurrenderActionTimeout.String() != "1.5s" ||
		!cfg.Runtime.RoomAllowLeaveDuringPlay ||
		!cfg.Runtime.LobbyBotSupervisorEnabled ||
		cfg.Runtime.LobbyMaxBotsPerRoom != 2 ||
		cfg.Runtime.RedisIdempotencyTTL.String() != "2m0s" {
		t.Fatalf("%+v", cfg.Runtime)
	}
	if cfg.Runtime.Postgres.Pool.MaxConns != 17 ||
		cfg.Runtime.Postgres.Pool.MinConns != 2 ||
		cfg.Runtime.Postgres.Pool.MaxConnLifetime.String() != "45m0s" ||
		cfg.Runtime.Postgres.Pool.MaxConnIdleTime.String() != "5m0s" ||
		cfg.Runtime.Postgres.Pool.HealthCheckPeriod.String() != "30s" {
		t.Fatalf("%+v", cfg.Runtime.Postgres.Pool)
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

func TestLoadClusterTLS(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tls.yaml")
	content := "server:\n  addr: \":19999\"\ncluster:\n  tls:\n    cert_file: \"/etc/lsp/tls/node.pem\"\n    key_file: \"/etc/lsp/tls/node.key\"\n    ca_file: \"/etc/lsp/tls/ca.pem\"\n    server_name: \"lsp-cluster\"\netcd:\n  tls:\n    cert_file: \"/etc/lsp/tls/etcd.pem\"\n    key_file: \"/etc/lsp/tls/etcd.key\"\n    ca_file: \"/etc/lsp/tls/etcd-ca.pem\"\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClusterTLS.CertFile != "/etc/lsp/tls/node.pem" ||
		cfg.ClusterTLS.KeyFile != "/etc/lsp/tls/node.key" ||
		cfg.ClusterTLS.CAFile != "/etc/lsp/tls/ca.pem" ||
		cfg.ClusterTLS.ServerName != "lsp-cluster" {
		t.Fatalf("%+v", cfg.ClusterTLS)
	}
	if !cfg.ClusterTLS.Enabled() {
		t.Fatalf("三项证书材料齐备时 Enabled 应为真: %+v", cfg.ClusterTLS)
	}
	if cfg.EtcdTLS.CertFile != "/etc/lsp/tls/etcd.pem" ||
		cfg.EtcdTLS.KeyFile != "/etc/lsp/tls/etcd.key" ||
		cfg.EtcdTLS.CAFile != "/etc/lsp/tls/etcd-ca.pem" ||
		!cfg.EtcdTLS.Enabled() {
		t.Fatalf("%+v", cfg.EtcdTLS)
	}
}
