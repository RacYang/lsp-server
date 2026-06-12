package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	goredis "github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	lobbyadapter "racoo.cn/lsp/internal/adapter/lobby"
	"racoo.cn/lsp/internal/app"

	"racoo.cn/lsp/internal/cluster"
	"racoo.cn/lsp/internal/config"
	lobbysvc "racoo.cn/lsp/internal/service/lobby"
	"racoo.cn/lsp/internal/store/redis"
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
	var svc *lobbysvc.Service
	if cfg.RedisAddr != "" {
		rcli := goredis.NewClient(&goredis.Options{Addr: cfg.RedisAddr})
		rdb := redis.NewClientFromUniversal(rcli)
		reg := redis.NewLobbyRoomRegistry(rdb)
		svc = lobbysvc.NewWithRegistry(reg)
		if err := svc.RecoverFromRegistry(ctx); err != nil {
			logx.Warn(ctx, "大厅状态从 Redis 恢复失败，以空状态启动", "err", err.Error())
		}
	} else {
		svc = lobbysvc.New()
	}
	var (
		claimer  *cluster.EtcdRouter
		selector cluster.RoomNodeSelector
	)
	if cfg.EtcdEndpoints != "" {
		cli, err := clientv3.New(clientv3.Config{
			Endpoints:   cluster.ParseEndpoints(cfg.EtcdEndpoints),
			DialTimeout: 5 * time.Second,
		})
		if err != nil {
			logx.Error(ctx, "大厅服务 etcd 客户端初始化失败", "err", err.Error())
			return 1
		}
		defer func() { _ = cli.Close() }()
		disco := cluster.NewEtcdDiscovery(cli, cfg.EtcdPrefix, 30)
		reg, err := disco.RegisterAndKeepAlive(ctx, cluster.KindLobby, cluster.NewNodeID(), cluster.NodeMeta{
			AdvertiseAddr: cfg.ServerAddr,
			Version:       "phase3",
		}, 10*time.Second)
		if err != nil {
			logx.Error(ctx, "大厅节点注册到 etcd 失败", "err", err.Error())
			return 1
		}
		defer func() { _ = reg.Stop(context.Background()) }()
		// 摘增量先于排空：收到停机信号立即注销节点发现，gate 不再把新请求路由到本节点；
		// 在途请求仍由 GRPCApp 的 GracefulStop 排空。重复 Stop 幂等（撤销同一租约）。
		go func() {
			<-ctx.Done()
			_ = reg.Stop(context.Background())
		}()
		claimer = cluster.NewEtcdRouter(cli, cfg.EtcdPrefix)
		selector = cluster.NewLeastRoomsSelector(disco)
	}
	serverCreds, err := cluster.NewServerTransportCredentials(cfg.ClusterTLS.CertFile, cfg.ClusterTLS.KeyFile, cfg.ClusterTLS.CAFile)
	if err != nil {
		logx.Error(ctx, "大厅服务集群凭据构造失败", "err", err.Error())
		return 1
	}
	if !cfg.ClusterTLS.Enabled() {
		logx.Warn(ctx, "集群 gRPC 未配置 mTLS，大厅服务以明文监听")
	}
	a, err := app.NewGRPC(ctx, cfg.ServerAddr, func(s *grpc.Server) {
		srv := lobbyadapter.NewGRPCServer(svc, claimer, defaultRoomNodeID)
		srv.SetRoomNodeSelector(selector)
		lobbyadapter.RegisterService(s, srv)
	}, grpc.Creds(serverCreds))
	if err != nil {
		logx.Error(ctx, "大厅服务装配失败", "err", err.Error())
		return 1
	}
	obsStop, err := app.StartObsHTTP(cfg.ObsAddr)
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
