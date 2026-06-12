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
	roomadapter "racoo.cn/lsp/internal/adapter/room"
	"racoo.cn/lsp/internal/app"
	"racoo.cn/lsp/internal/cluster"
	"racoo.cn/lsp/internal/config"
	roomsvc "racoo.cn/lsp/internal/service/room"
	eng "racoo.cn/lsp/internal/service/room/engine"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

const defaultRoomNodeID = "room-local"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	os.Exit(run(ctx, stop))
}

func run(ctx context.Context, stop context.CancelFunc) int {
	defer stop()
	ctx = logx.WithTraceID(ctx, "process")
	ctx = logx.WithUserID(ctx, "")
	ctx = logx.WithRoomID(ctx, "")
	cfg, err := config.Load(os.Getenv("LSP_CONFIG"))
	if err != nil {
		logx.Error(ctx, "房间服务配置加载失败", "err", err.Error())
		return 1
	}
	var (
		ev   *postgres.RoomEventStore
		gs   *postgres.GameSummaryStore
		st   *postgres.SettlementStore
		pg   postgres.Pinger
		rcli *redis.Client
	)
	if cfg.PostgresDSN != "" {
		pool, err := postgres.OpenPoolWithOptions(ctx, cfg.PostgresDSN, postgres.PoolOptions{
			MaxConns: cfg.Runtime.Postgres.Pool.MaxConns, MinConns: cfg.Runtime.Postgres.Pool.MinConns,
			MaxConnLifetime: cfg.Runtime.Postgres.Pool.MaxConnLifetime, MaxConnIdleTime: cfg.Runtime.Postgres.Pool.MaxConnIdleTime,
			HealthCheckPeriod: cfg.Runtime.Postgres.Pool.HealthCheckPeriod,
		})
		if err != nil {
			logx.Error(ctx, "房间事件持久化数据库连接失败", "err", err.Error())
			return 1
		}
		defer pool.Close()
		pg = pool
		ev = postgres.NewRoomEventStore(pool)
		gs = postgres.NewGameSummaryStore(pool)
		st = postgres.NewSettlementStore(pool)
	}
	if cfg.RedisAddr != "" {
		c, err := redis.NewClient(cfg.RedisAddr)
		if err != nil {
			logx.Error(ctx, "Redis 连接失败", "err", err.Error())
			return 1
		}
		defer func() { _ = c.Close() }()
		rcli = c
	}
	if cfg.EtcdEndpoints != "" && rcli == nil {
		logx.Error(ctx, "启用 etcd 房间恢复时必须同时配置 Redis", "err", "missing redis.addr")
		return 1
	}
	svcCore := roomsvc.NewServiceWithRule(roomsvc.NewRoomRegistry(), cfg.RuleID,
		roomsvc.WithMailboxCapacity(cfg.Runtime.RoomMailboxCapacity),
		roomsvc.WithAllowLeaveDuringPlay(cfg.Runtime.RoomAllowLeaveDuringPlay),
		roomsvc.WithTimeoutConfig(eng.TimeoutConfig{
			OpeningDefault:  cfg.RoomTimeouts.OpeningDefault,
			OpeningByAction: cfg.RoomTimeouts.OpeningByAction,
			ClaimWindow:     cfg.RoomTimeouts.ClaimWindow,
			TsumoWindow:     cfg.RoomTimeouts.TsumoWindow,
			Discard:         cfg.RoomTimeouts.Discard,
			SurrenderAction: cfg.Runtime.RoomSurrenderActionTimeout,
		}),
	)
	svc := roomadapter.NewGRPCServer(svcCore, ev, gs, st, rcli)
	svc.SetIdempotencyTTL(cfg.Runtime.RedisIdempotencyTTL)
	if cfg.EtcdEndpoints != "" {
		svc.SetReady(false)
		cli, err := clientv3.New(clientv3.Config{Endpoints: cluster.ParseEndpoints(cfg.EtcdEndpoints), DialTimeout: 5 * time.Second})
		if err != nil {
			logx.Error(ctx, "房间服务 etcd 客户端初始化失败", "err", err.Error())
			return 1
		}
		defer func() { _ = cli.Close() }()
		disco := cluster.NewEtcdDiscovery(cli, cfg.EtcdPrefix, 30)
		advertiseAddr := strings.TrimSpace(cfg.ClusterAdvertiseAddr)
		if advertiseAddr == "" {
			advertiseAddr = cfg.ServerAddr
		}
		reg, err := disco.RegisterAndKeepAlive(ctx, cluster.KindRoom, defaultRoomNodeID, cluster.NodeMeta{
			AdvertiseAddr: advertiseAddr, Version: "phase3",
		}, 10*time.Second)
		if err != nil {
			logx.Error(ctx, "房间节点注册到 etcd 失败", "err", err.Error())
			return 1
		}
		defer func() { _ = reg.Stop(context.Background()) }()
		// 摘增量先于排空：收到停机信号立即注销节点发现，lobby 不再为新房间选中本节点；
		// 在途请求仍由 GRPCApp 的 GracefulStop 排空。重复 Stop 幂等（撤销同一租约）。
		go func() {
			<-ctx.Done()
			_ = reg.Stop(context.Background())
		}()
		// 每 10 秒上报活跃房间数，供 Lobby 负载均衡选节点。
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = reg.UpdateMeta(ctx, cluster.KindRoom, cluster.NodeMeta{
						AdvertiseAddr: advertiseAddr, Version: "phase3",
						ActiveRooms: int32(svcCore.ActiveRoomCount()), //nolint:gosec // 房间数不超过 int32 范围
					})
				}
			}
		}()
		if rcli != nil {
			rt := cluster.NewEtcdRouter(cli, cfg.EtcdPrefix)
			if err := roomadapter.RecoverOwnedRooms(ctx, rt, defaultRoomNodeID, rcli, ev, gs, svcCore); err != nil {
				logx.Error(ctx, "房间冷启动恢复失败", "err", err.Error())
				return 1
			}
		}
		svc.SetReady(true)
	}
	a, err := app.NewGRPC(ctx, cfg.ServerAddr, func(s *grpc.Server) {
		roomadapter.RegisterService(s, svc)
	})
	if err != nil {
		logx.Error(ctx, "房间服务装配失败", "err", err.Error())
		return 1
	}
	var readiness []app.ReadinessProbe
	if rcli != nil {
		readiness = append(readiness, app.RedisReadinessProbe(rcli))
	}
	if pg != nil {
		readiness = append(readiness, app.ReadinessProbe{Name: "postgres", Check: pg.Ping})
	}
	obsStop, err := app.StartObsHTTP(cfg.ObsAddr, readiness...)
	if err != nil {
		logx.Error(ctx, "可观测性 HTTP 启动失败", "err", err.Error())
		return 1
	}
	defer obsStop()
	logx.Info(ctx, "房间服务启动", "addr", cfg.ServerAddr)
	runErr := a.Run(ctx)
	// 停机顺序契约：传输层排空（Run 返回）→ 停房间自驱动定时器 → defer 关闭存储依赖。
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	svcCore.Shutdown(shutdownCtx)
	cancelShutdown()
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		logx.Error(ctx, "房间服务退出异常", "err", runErr.Error())
		return 1
	}
	return 0
}
