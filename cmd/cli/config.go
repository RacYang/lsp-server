package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Config 持久化玩家在终端客户端的本地偏好与会话状态。
type Config struct {
	Nickname     string `toml:"nickname"`
	ServerURL    string `toml:"server_url"`
	SessionToken string `toml:"session_token"`
}

const defaultServerURL = "wss://racoo.cn/ws"

// NewDefaultConfig 返回内置默认值。
func NewDefaultConfig() Config {
	return Config{
		ServerURL: defaultServerURL,
	}
}

// DefaultConfigPath 给出推荐的本地配置路径 `~/.lsp/config.toml`。
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".lsp", "config.toml")
	}
	return filepath.Join(home, ".lsp", "config.toml")
}

// LoadConfig 从指定路径读取配置；文件不存在时返回默认值且不报错。
func LoadConfig(path string) (Config, error) {
	cfg := NewDefaultConfig()
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("读取配置 %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return NewDefaultConfig(), fmt.Errorf("解析配置 %s: %w", path, err)
	}
	cfg.fillDefaults()
	return cfg, nil
}

// SaveConfig 原子地把配置落盘到 path。
func SaveConfig(path string, cfg Config) error {
	if path == "" {
		return errors.New("配置路径为空")
	}
	cfg.fillDefaults()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录: %w", err)
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写入临时配置: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换配置文件: %w", err)
	}
	return nil
}

func (c *Config) fillDefaults() {
	def := NewDefaultConfig()
	if c.ServerURL == "" {
		c.ServerURL = def.ServerURL
	}
}
