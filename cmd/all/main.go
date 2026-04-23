// main 为 Phase 1 单体入口：读取环境变量 LSP_CONFIG 指向的 YAML（可选），加载配置并启动 WebSocket 服务。
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"racoo.cn/lsp/internal/app"
	"racoo.cn/lsp/internal/config"
	"racoo.cn/lsp/pkg/logx"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := run(ctx, stop)
	os.Exit(code)
}

func run(ctx context.Context, stop context.CancelFunc) int {
	defer stop()
	cfgPath := os.Getenv("LSP_CONFIG")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logx.Error(ctx, "配置加载失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return 1
	}
	a, err := app.New(cfg)
	if err != nil {
		logx.Error(ctx, "应用装配失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return 1
	}
	logx.Info(ctx, "服务启动", "trace_id", "", "user_id", "", "room_id", "", "addr", cfg.ServerAddr)
	if err := a.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logx.Error(ctx, "服务退出异常", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return 1
	}
	return 0
}
