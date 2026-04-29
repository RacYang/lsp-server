// loadgen 是 Phase 6 压测入口，用 YAML 剧本驱动 WebSocket 客户端流量。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
	"racoo.cn/lsp/pkg/logx"
)

type scenarioConfig struct {
	Scenario       string `yaml:"scenario"`
	WSURL          string `yaml:"ws_url"`
	MetricsURL     string `yaml:"metrics_url"`
	RoomIDPrefix   string `yaml:"room_id_prefix"`
	Rooms          int    `yaml:"rooms"`
	PlayersPerRoom int    `yaml:"players_per_room"`
	RoundCount     int    `yaml:"round_count"`
	Version        string `yaml:"version"`
}

type scenarioSummary struct {
	Scenario       string        `json:"scenario"`
	Version        string        `json:"version"`
	StartedAt      time.Time     `json:"started_at"`
	FinishedAt     time.Time     `json:"finished_at"`
	Duration       time.Duration `json:"duration"`
	Rooms          int           `json:"rooms"`
	PlayersPerRoom int           `json:"players_per_room"`
	RoundCount     int           `json:"round_count"`
	Requests       int           `json:"requests"`
	Errors         int           `json:"errors"`
	Passed         bool          `json:"passed"`
	Notes          []string      `json:"notes"`
	RuntimeAdvice  []string      `json:"runtime_advice"`
}

func main() {
	var (
		scenario = flag.String("scenario", "a", "压测场景：a/b/c")
		cfgPath  = flag.String("config", "bench/scenarios/scenario_a/config.yaml", "场景配置路径")
		outDir   = flag.String("out", "", "输出目录，空值时自动生成 bench/runs/<run_id>")
	)
	flag.Parse()

	cfg, err := loadScenarioConfig(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取压测配置失败: %v\n", err)
		os.Exit(1)
	}
	if cfg.Scenario == "" {
		cfg.Scenario = *scenario
	}
	if *outDir == "" {
		*outDir = filepath.Join("bench", "runs", time.Now().UTC().Format("20060102T150405Z")+"-"+cfg.Scenario)
	}
	if err := os.MkdirAll(*outDir, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	ctx = logx.WithTraceID(ctx, "loadgen")
	ctx = logx.WithUserID(ctx, "")
	ctx = logx.WithRoomID(ctx, cfg.RoomIDPrefix)

	var summary scenarioSummary
	switch cfg.Scenario {
	case "a":
		summary, err = runScenarioA(ctx, cfg)
	case "b":
		summary, err = runScenarioB(ctx, cfg)
	case "c":
		summary, err = runScenarioC(ctx, cfg)
	default:
		err = fmt.Errorf("未知压测场景: %s", cfg.Scenario)
	}
	if err != nil {
		summary.Errors++
		summary.Notes = append(summary.Notes, err.Error())
	}
	summary.StartedAt = started
	summary.FinishedAt = time.Now()
	summary.Duration = summary.FinishedAt.Sub(summary.StartedAt)
	if summary.Scenario == "" {
		summary.Scenario = cfg.Scenario
	}
	if summary.Version == "" {
		summary.Version = cfg.Version
	}
	if err := writeOutputs(*outDir, cfg, summary); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "写入压测结果失败: %v\n", err)
		os.Exit(1)
	}
	cancel()
	if !summary.Passed {
		os.Exit(1)
	}
}

func loadScenarioConfig(path string) (scenarioConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304：压测配置路径由调用方显式传入。
	if err != nil {
		return scenarioConfig{}, err
	}
	var cfg scenarioConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return scenarioConfig{}, err
	}
	if cfg.WSURL == "" {
		cfg.WSURL = "ws://127.0.0.1:18080/ws"
	}
	if cfg.RoomIDPrefix == "" {
		cfg.RoomIDPrefix = "bench-room"
	}
	if cfg.Rooms <= 0 {
		cfg.Rooms = 1
	}
	if cfg.PlayersPerRoom <= 0 {
		cfg.PlayersPerRoom = 4
	}
	if cfg.RoundCount <= 0 {
		cfg.RoundCount = 100
	}
	return cfg, nil
}

func writeOutputs(outDir string, cfg scenarioConfig, summary scenarioSummary) error {
	cfgData, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "config.yaml"), cfgData, 0o600); err != nil {
		return err
	}
	metricsData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "metrics.json"), metricsData, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "summary.md"), []byte(renderSummary(summary)), 0o600)
}

func renderSummary(summary scenarioSummary) string {
	status := "未通过"
	if summary.Passed {
		status = "通过"
	}
	return fmt.Sprintf(`# 压测摘要

- 场景：%s
- 版本：%s
- 房间数：%d
- 每房玩家数：%d
- 轮数：%d
- 请求数：%d
- 错误数：%d
- 结果：%s
- 建议的 runtime.* 调整方向：%s

## 备注

%s
`, summary.Scenario, summary.Version, summary.Rooms, summary.PlayersPerRoom, summary.RoundCount, summary.Requests, summary.Errors, status, joinOrDefault(summary.RuntimeAdvice, "保持当前默认值，等待更多容量样本"), joinOrDefault(summary.Notes, "无"))
}

func joinOrDefault(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	out := ""
	for i, item := range items {
		if i > 0 {
			out += "；"
		}
		out += item
	}
	return out
}
