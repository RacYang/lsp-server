// main 为本地单进程聚合入口：用于保留 Phase 1 冒烟路径与 Phase 2 开发自测。
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
	ctx = logx.WithTraceID(ctx, "process")
	ctx = logx.WithUserID(ctx, "")
	ctx = logx.WithRoomID(ctx, "")
	cfgPath := os.Getenv("LSP_CONFIG")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logx.Error(ctx, "聚合入口配置加载失败", "err", err.Error())
		return 1
	}
	a, err := app.NewAllInProcess(cfg)
	if err != nil {
		logx.Error(ctx, "聚合入口装配失败", "err", err.Error())
		return 1
	}
	logx.Info(ctx, "聚合入口启动", "addr", cfg.ServerAddr)
	if err := a.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logx.Error(ctx, "聚合入口退出异常", "err", err.Error())
		return 1
	}
	return 0
}
