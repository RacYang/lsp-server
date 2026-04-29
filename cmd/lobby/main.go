package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"racoo.cn/lsp/internal/app"
	"racoo.cn/lsp/internal/cluster/discovery"
	"racoo.cn/lsp/internal/cluster/nodeid"
	"racoo.cn/lsp/internal/cluster/router"
	"racoo.cn/lsp/internal/config"
	lobbysvc "racoo.cn/lsp/internal/service/lobby"
	"racoo.cn/lsp/pkg/logx"
)

const defaultRoomNodeID = "room-local"

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
	cfg, err := config.Load(os.Getenv("LSP_CONFIG"))
	if err != nil {
		logx.Error(ctx, "大厅服务配置加载失败", "err", err.Error())
		return 1
	}
	svc := lobbysvc.New()
	var claimer *router.Etcd
	if cfg.EtcdEndpoints != "" {
		cli, err := clientv3.New(clientv3.Config{
			Endpoints:   splitEtcdEndpoints(cfg.EtcdEndpoints),
			DialTimeout: 5 * time.Second,
		})
		if err != nil {
			logx.Error(ctx, "大厅服务 etcd 客户端初始化失败", "err", err.Error())
			return 1
		}
		defer func() { _ = cli.Close() }()
		disco := discovery.NewEtcd(cli, "/lsp", 30)
		reg, err := disco.RegisterAndKeepAlive(ctx, nodeid.KindLobby, nodeid.New(), discovery.NodeMeta{
			AdvertiseAddr: cfg.ServerAddr,
			Version:       "phase3",
		}, 10*time.Second)
		if err != nil {
			logx.Error(ctx, "大厅节点注册到 etcd 失败", "err", err.Error())
			return 1
		}
		defer func() { _ = reg.Stop(context.Background()) }()
		claimer = router.NewEtcd(cli, "/lsp")
	}
	a, err := app.NewGRPC(cfg.ServerAddr, func(s *grpc.Server) {
		registerLobbyService(s, newLobbyGRPCServer(svc, claimer, defaultRoomNodeID))
	})
	if err != nil {
		logx.Error(ctx, "大厅服务装配失败", "err", err.Error())
		return 1
	}
	obsStop, err := app.StartObsHTTP(cfg.ObsAddr, nil)
	if err != nil {
		logx.Error(ctx, "可观测性 HTTP 启动失败", "err", err.Error())
		return 1
	}
	defer obsStop()
	logx.Info(ctx, "大厅服务启动", "addr", cfg.ServerAddr)
	if err := a.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logx.Error(ctx, "大厅服务退出异常", "err", err.Error())
		return 1
	}
	return 0
}

func splitEtcdEndpoints(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
