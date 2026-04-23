package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"racoo.cn/lsp/internal/app"
	"racoo.cn/lsp/internal/config"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/pkg/logx"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := run(ctx, stop)
	os.Exit(code)
}

func run(ctx context.Context, stop context.CancelFunc) int {
	defer stop()
	cfg, err := config.Load(os.Getenv("LSP_CONFIG"))
	if err != nil {
		logx.Error(ctx, "房间服务配置加载失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return 1
	}
	svc := newRoomGRPCServer(roomsvc.NewServiceWithRule(roomsvc.NewLobby(), cfg.RuleID))
	a, err := app.NewGRPC(cfg.ServerAddr, func(s *grpc.Server) {
		registerRoomService(s, svc)
	})
	if err != nil {
		logx.Error(ctx, "房间服务装配失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return 1
	}
	logx.Info(ctx, "房间服务启动", "trace_id", "", "user_id", "", "room_id", "", "addr", cfg.ServerAddr)
	if err := a.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logx.Error(ctx, "房间服务退出异常", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return 1
	}
	return 0
}
