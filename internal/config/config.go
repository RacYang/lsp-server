// Package config 负责加载运行时配置（Phase 1 使用 viper 读取 YAML）。
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 为进程级配置快照。
type Config struct {
	ServerAddr       string
	WSAllowedOrigins []string
	RuleID           string
	ClusterLobbyAddr string
	ClusterRoomAddr  string
	// ClusterAdvertiseAddr 是当前节点对其它集群进程公布的可连接地址。
	ClusterAdvertiseAddr string
	// RedisAddr 非空时启用会话、快照元数据等数据面（Phase 3）。
	RedisAddr string
	// PostgresDSN 非空时启用对局事件与结算持久化（Phase 3）。
	PostgresDSN string
	// ObsAddr 非空时绑定可观测性 HTTP（健康检查、指标、pprof）。
	ObsAddr string
	// EtcdEndpoints 逗号分隔的 etcd 端点；空表示不启用控制面客户端（单测与本地默认）。
	EtcdEndpoints string
	RoomTimeouts  RoomTimeouts
	Runtime       RuntimeConfig
}

// RoomTimeouts 定义房间各等待态服务端托管超时。
type RoomTimeouts struct {
	ExchangeThree time.Duration
	QueMen        time.Duration
	ClaimWindow   time.Duration
	TsumoWindow   time.Duration
	Discard       time.Duration
}

// RuntimeConfig 定义可在运行时 YAML 中调整的容量与限流参数。
type RuntimeConfig struct {
	GateWSRateLimitPerSecond   float64
	GateWSRateLimitBurst       float64
	GateWSIdempotencyCache     int
	RoomMailboxCapacity        int
	RoomSurrenderActionTimeout time.Duration
	RoomSurrenderAfterOffline  time.Duration
	RoomAllowLeaveDuringPlay   bool
	LobbyBotSupervisorEnabled  bool
	LobbyMaxBotsPerRoom        int
	RedisIdempotencyTTL        time.Duration
	Postgres                   PostgresRuntimeConfig
	Logging                    LoggingConfig
}

// PostgresRuntimeConfig 定义 PostgreSQL 运行时参数。
type PostgresRuntimeConfig struct {
	Pool PostgresPoolConfig
}

// PostgresPoolConfig 定义 PostgreSQL 连接池参数；零值表示沿用 pgx/DSN 默认值。
type PostgresPoolConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// LoggingConfig 定义日志门面的运行时开关。
type LoggingConfig struct {
	Level        string
	Format       string
	Sample       LoggingSamplingConfig
	OTelEnabled  bool
	OTelEndpoint string
	DynamicLevel bool
}

// LoggingSamplingConfig 定义日志采样参数；默认关闭。
type LoggingSamplingConfig struct {
	Initial    int
	Thereafter int
	Tick       time.Duration
	ErrorNever bool
}

const (
	defaultGateWSRateLimitPerSecond   = 20
	defaultGateWSRateLimitBurst       = 40
	defaultGateWSIdempotencyCache     = 4096
	defaultRoomMailboxCapacity        = 96
	defaultRoomSurrenderActionTimeout = time.Second
	defaultRoomSurrenderAfterOffline  = 30 * time.Second
	defaultLobbyMaxBotsPerRoom        = 3
	defaultRedisIdempotencyTTL        = 10 * time.Minute
	defaultLoggingLevel               = "info"
	defaultLoggingFormat              = "json"
	defaultLoggingSamplingTick        = time.Second
)

func (cfg RuntimeConfig) withDefaults() RuntimeConfig {
	if cfg.GateWSRateLimitPerSecond <= 0 {
		cfg.GateWSRateLimitPerSecond = defaultGateWSRateLimitPerSecond
	}
	if cfg.GateWSRateLimitBurst <= 0 {
		cfg.GateWSRateLimitBurst = defaultGateWSRateLimitBurst
	}
	if cfg.GateWSIdempotencyCache <= 0 {
		cfg.GateWSIdempotencyCache = defaultGateWSIdempotencyCache
	}
	if cfg.RoomMailboxCapacity <= 0 {
		cfg.RoomMailboxCapacity = defaultRoomMailboxCapacity
	}
	if cfg.RoomSurrenderActionTimeout <= 0 {
		cfg.RoomSurrenderActionTimeout = defaultRoomSurrenderActionTimeout
	}
	if cfg.RoomSurrenderAfterOffline <= 0 {
		cfg.RoomSurrenderAfterOffline = defaultRoomSurrenderAfterOffline
	}
	if cfg.LobbyMaxBotsPerRoom <= 0 {
		cfg.LobbyMaxBotsPerRoom = defaultLobbyMaxBotsPerRoom
	}
	if cfg.RedisIdempotencyTTL <= 0 {
		cfg.RedisIdempotencyTTL = defaultRedisIdempotencyTTL
	}
	cfg.Logging = cfg.Logging.withDefaults()
	return cfg
}

func (cfg LoggingConfig) withDefaults() LoggingConfig {
	if strings.TrimSpace(cfg.Level) == "" {
		cfg.Level = defaultLoggingLevel
	}
	if strings.TrimSpace(cfg.Format) == "" {
		cfg.Format = defaultLoggingFormat
	}
	if cfg.Sample.Tick <= 0 {
		cfg.Sample.Tick = defaultLoggingSamplingTick
	}
	cfg.Sample.ErrorNever = true
	return cfg
}

// Load 从路径加载 YAML；path 为空时使用默认 `configs/dev.yaml`。
func Load(path string) (Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetDefault("runtime.room.allow_leave_during_play", true)
	v.SetDefault("runtime.lobby.bot_supervisor_enabled", true)
	if strings.TrimSpace(path) == "" {
		path = "configs/dev.yaml"
	}
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg := Config{
		ServerAddr:           v.GetString("server.addr"),
		WSAllowedOrigins:     v.GetStringSlice("server.ws_allowed_origins"),
		RuleID:               v.GetString("rule.default_id"),
		ClusterLobbyAddr:     v.GetString("cluster.lobby_addr"),
		ClusterRoomAddr:      v.GetString("cluster.room_addr"),
		ClusterAdvertiseAddr: v.GetString("cluster.advertise_addr"),
		RedisAddr:            v.GetString("redis.addr"),
		PostgresDSN:          v.GetString("postgres.dsn"),
		ObsAddr:              v.GetString("obs.addr"),
		EtcdEndpoints:        v.GetString("etcd.endpoints"),
		RoomTimeouts: RoomTimeouts{
			ExchangeThree: v.GetDuration("room.timeout.exchange_three"),
			QueMen:        v.GetDuration("room.timeout.que_men"),
			ClaimWindow:   v.GetDuration("room.timeout.claim_window"),
			TsumoWindow:   v.GetDuration("room.timeout.tsumo_window"),
			Discard:       v.GetDuration("room.timeout.discard"),
		},
		Runtime: RuntimeConfig{
			GateWSRateLimitPerSecond:   v.GetFloat64("runtime.gate.ws_rate_limit_per_second"),
			GateWSRateLimitBurst:       v.GetFloat64("runtime.gate.ws_rate_limit_burst"),
			GateWSIdempotencyCache:     v.GetInt("runtime.gate.ws_idempotency_cache"),
			RoomMailboxCapacity:        v.GetInt("runtime.room.mailbox_capacity"),
			RoomSurrenderActionTimeout: v.GetDuration("runtime.room.surrender_action_timeout"),
			RoomSurrenderAfterOffline:  v.GetDuration("runtime.room.surrender_after_offline"),
			RoomAllowLeaveDuringPlay:   v.GetBool("runtime.room.allow_leave_during_play"),
			LobbyBotSupervisorEnabled:  v.GetBool("runtime.lobby.bot_supervisor_enabled"),
			LobbyMaxBotsPerRoom:        v.GetInt("runtime.lobby.max_bots_per_room"),
			RedisIdempotencyTTL:        v.GetDuration("runtime.redis.idempotency_ttl"),
			Postgres: PostgresRuntimeConfig{
				Pool: PostgresPoolConfig{
					MaxConns:          v.GetInt32("runtime.postgres.pool.max_conns"),
					MinConns:          v.GetInt32("runtime.postgres.pool.min_conns"),
					MaxConnLifetime:   v.GetDuration("runtime.postgres.pool.max_conn_lifetime"),
					MaxConnIdleTime:   v.GetDuration("runtime.postgres.pool.max_conn_idle_time"),
					HealthCheckPeriod: v.GetDuration("runtime.postgres.pool.health_check_period"),
				},
			},
			Logging: LoggingConfig{
				Level:        v.GetString("runtime.logging.level"),
				Format:       v.GetString("runtime.logging.format"),
				OTelEnabled:  v.GetBool("runtime.logging.otel_enabled"),
				OTelEndpoint: v.GetString("runtime.logging.otel_endpoint"),
				DynamicLevel: v.GetBool("runtime.logging.dynamic_level"),
				Sample: LoggingSamplingConfig{
					Initial:    v.GetInt("runtime.logging.sample.initial"),
					Thereafter: v.GetInt("runtime.logging.sample.thereafter"),
					Tick:       v.GetDuration("runtime.logging.sample.tick"),
					ErrorNever: v.GetBool("runtime.logging.sample.error_never"),
				},
			},
		}.withDefaults(),
	}
	return cfg, nil
}
