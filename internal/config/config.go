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
	// RedisAddr 非空时启用会话、快照元数据等数据面（Phase 3）。
	RedisAddr string
	// PostgresDSN 非空时启用对局事件与结算持久化（Phase 3）。
	PostgresDSN string
	// ObsAddr 非空时绑定可观测性 HTTP（健康检查、指标、pprof）。
	ObsAddr string
	// EtcdEndpoints 逗号分隔的 etcd 端点；空表示不启用控制面客户端（单测与本地默认）。
	EtcdEndpoints string
	RoomTimeouts  RoomTimeouts
}

// RoomTimeouts 定义房间各等待态服务端托管超时。
type RoomTimeouts struct {
	ExchangeThree time.Duration
	QueMen        time.Duration
	ClaimWindow   time.Duration
	TsumoWindow   time.Duration
	Discard       time.Duration
}

// Load 从路径加载 YAML；path 为空时使用默认 `configs/dev.yaml`。
func Load(path string) (Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if strings.TrimSpace(path) == "" {
		path = "configs/dev.yaml"
	}
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	return Config{
		ServerAddr:       v.GetString("server.addr"),
		WSAllowedOrigins: v.GetStringSlice("server.ws_allowed_origins"),
		RuleID:           v.GetString("rule.default_id"),
		ClusterLobbyAddr: v.GetString("cluster.lobby_addr"),
		ClusterRoomAddr:  v.GetString("cluster.room_addr"),
		RedisAddr:        v.GetString("redis.addr"),
		PostgresDSN:      v.GetString("postgres.dsn"),
		ObsAddr:          v.GetString("obs.addr"),
		EtcdEndpoints:    v.GetString("etcd.endpoints"),
		RoomTimeouts: RoomTimeouts{
			ExchangeThree: v.GetDuration("room.timeout.exchange_three"),
			QueMen:        v.GetDuration("room.timeout.que_men"),
			ClaimWindow:   v.GetDuration("room.timeout.claim_window"),
			TsumoWindow:   v.GetDuration("room.timeout.tsumo_window"),
			Discard:       v.GetDuration("room.timeout.discard"),
		},
	}, nil
}
