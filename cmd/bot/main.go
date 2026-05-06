// lsp-bot 是独立机器人陪玩客户端，按普通玩家协议加入指定房间。
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"racoo.cn/lsp/internal/bot"
	"racoo.cn/lsp/pkg/logx"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		wsURL              = flag.String("ws", "ws://127.0.0.1:8080/ws", "服务器 WebSocket 地址")
		roomID             = flag.String("room", "", "要加入的房间 ID")
		count              = flag.Int("count", 1, "机器人数量")
		namePrefix         = flag.String("name-prefix", "机器人", "机器人昵称前缀")
		strategy           = flag.String("strategy", string(bot.DifficultyNormal), "内置策略难度: easy / normal / hard")
		thinkMin           = flag.Duration("think-min", -1, "覆盖最小思考延迟；0 表示关闭延迟，负数使用默认")
		thinkMax           = flag.Duration("think-max", -1, "覆盖最大思考延迟；0 表示关闭延迟，负数使用默认")
		tokenDir           = flag.String("token-dir", filepath.Join(os.TempDir(), "lsp-bot-tokens"), "机器人会话 token 目录")
		origin             = flag.String("origin", "", "WebSocket Origin 头")
		insecureSkipVerify = flag.Bool("insecure-skip-verify", false, "wss 调试时跳过证书校验")
		logLevel           = flag.String("log-level", "info", "日志级别: debug / info / warn / error")
		seed               = flag.Int64("seed", time.Now().UnixNano(), "随机种子")
	)
	flag.String("llm-provider", "", "预留：LLM Provider，仅在对应 build tag 下生效")
	flag.String("llm-base-url", "", "预留：LLM API base URL")
	flag.String("llm-api-key", "", "预留：LLM API key")
	flag.String("llm-model", "", "预留：LLM 模型名")
	flag.Int("llm-timeout-ms", 800, "预留：LLM 请求超时毫秒")
	flag.Parse()

	if strings.TrimSpace(*roomID) == "" {
		fmt.Fprintln(os.Stderr, "-room 不能为空")
		return 2
	}
	if *count <= 0 {
		fmt.Fprintln(os.Stderr, "-count 必须大于 0")
		return 2
	}
	if err := os.MkdirAll(*tokenDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "创建 token 目录失败:", err)
		return 1
	}
	logx.SetDefault(logx.NewWithOptions(os.Stderr, parseLevel(*logLevel), logx.Options{Format: "console"}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx = logx.WithTraceID(ctx, "bot")
	ctx = logx.WithRoomID(ctx, *roomID)

	var wg sync.WaitGroup
	for i := 0; i < *count; i++ {
		idx := i
		name := fmt.Sprintf("%s-%02d", *namePrefix, idx+1)
		rnd := rand.New(rand.NewSource(*seed + int64(idx))) //nolint:gosec // 策略扰动不用于安全边界。
		cfg := bot.RunnerConfig{
			Name:               name,
			RoomID:             *roomID,
			WSURL:              *wsURL,
			Origin:             *origin,
			TokenFile:          filepath.Join(*tokenDir, name+".token"),
			InsecureSkipVerify: *insecureSkipVerify,
			ThinkMin:           *thinkMin,
			ThinkMax:           *thinkMax,
			Strategy: bot.NewRuleStrategy(bot.RuleStrategyConfig{
				Difficulty: bot.Difficulty(*strategy),
				Rand:       rnd,
			}),
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := bot.NewRunner(cfg).Run(logx.WithUserID(ctx, name)); err != nil && ctx.Err() == nil {
				logx.Warn(ctx, "机器人退出", "name", name, "err", err.Error())
			}
		}()
	}
	wg.Wait()
	return 0
}

func parseLevel(raw string) logx.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return logx.LevelDebug
	case "warn":
		return logx.LevelWarn
	case "error":
		return logx.LevelError
	default:
		return logx.LevelInfo
	}
}
