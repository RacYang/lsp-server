// Package config 负责加载运行时配置（Phase 1 使用 viper 读取 YAML）。
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 为进程级配置快照。
type Config struct {
	ServerAddr       string
	RuleID           string
	ClusterLobbyAddr string
	ClusterRoomAddr  string
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
		RuleID:           v.GetString("rule.default_id"),
		ClusterLobbyAddr: v.GetString("cluster.lobby_addr"),
		ClusterRoomAddr:  v.GetString("cluster.room_addr"),
	}, nil
}
