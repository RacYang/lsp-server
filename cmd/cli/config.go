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
//
// 字段命名与 TOML key 一一对应（go-toml 默认下划线小写转换）。
// 任何字段都允许缺省，缺省值由 NewDefaultConfig 提供。
type Config struct {
	Nickname       string `toml:"nickname"`
	ServerURL      string `toml:"server_url"`
	SessionToken   string `toml:"session_token"`
	TileTheme      string `toml:"tile_theme"`
	ClaimTimeoutMS int    `toml:"claim_timeout_ms"`
}

const (
	tileThemeUnicode = "unicode"
	tileThemeASCII   = "ascii"

	defaultServerURL    = "wss://racoo.cn/ws"
	defaultClaimTimeout = 4500
)

// NewDefaultConfig 返回内置默认值；未来如果需要在多处复用，统一从此处取。
func NewDefaultConfig() Config {
	return Config{
		ServerURL:      defaultServerURL,
		TileTheme:      tileThemeUnicode,
		ClaimTimeoutMS: defaultClaimTimeout,
	}
}

// DefaultConfigPath 给出推荐的本地配置路径 `~/.lsp/config.toml`。
//
// 当 HOME 不可用（如某些 CI/容器场景）时退化到 `./.lsp/config.toml`，
// 让 CLI 仍能在受限环境下正常落盘。
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".lsp", "config.toml")
	}
	return filepath.Join(home, ".lsp", "config.toml")
}

// LoadConfig 从指定路径读取配置；文件不存在时返回默认值且不报错。
//
// 解析失败、磁盘 IO 错误等才会返回 error，便于上层决定是否提示用户。
// 返回值在缺失字段处会被默认值填补，避免下游再次判空。
func LoadConfig(path string) (Config, error) {
	cfg := NewDefaultConfig()
	data, err := os.ReadFile(path) // #nosec G304：路径由调用方传入。
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

// SaveConfig 原子地把配置落盘到 path：先写临时文件再 rename，避免半截文件。
//
// 目录不存在时自动 mkdir -p；文件权限 0600，因为里面会包含 session_token。
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
	if c.TileTheme != tileThemeUnicode && c.TileTheme != tileThemeASCII {
		c.TileTheme = def.TileTheme
	}
	if c.ClaimTimeoutMS <= 0 {
		c.ClaimTimeoutMS = def.ClaimTimeoutMS
	}
}
