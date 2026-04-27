// Package app 负责单体进程装配：HTTP/WebSocket、房间服务与会话 Hub。
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"

	"racoo.cn/lsp/internal/config"
	"racoo.cn/lsp/internal/handler"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

// App 为可启动的应用实例。
type App struct {
	srv     *http.Server
	ln      net.Listener
	rooms   *roomsvc.Service
	cleanup func()
}

// New 根据配置装配应用；当前等价于 gate 角色，保留以兼容 Phase 1 调用点。
func New(cfg config.Config) (*App, error) {
	return NewGate(cfg)
}

// NewAllInProcess 装配本地单进程聚合入口，供 `cmd/all` 冒烟和开发自测使用。
func NewAllInProcess(cfg config.Config) (*App, error) {
	return NewGate(cfg)
}

// NewGate 装配 gate 角色：WebSocket 接入、房间服务与会话 Hub。
func NewGate(cfg config.Config) (*App, error) {
	ln, err := net.Listen("tcp", cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("监听地址失败: %w", err)
	}
	hub := session.NewHub()
	var (
		rs           *roomsvc.Service
		cleanup      func()
		gateway      handler.RoomGateway
		sessMgr      *session.Manager
		redisCleanup func()
		redisClient  *redis.Client
		pgCleanup    func()
		settlements  *postgres.SettlementStore
	)
	if cfg.RedisAddr != "" {
		c, err := redis.NewClient(cfg.RedisAddr)
		if err != nil {
			_ = ln.Close()
			return nil, fmt.Errorf("redis 客户端初始化失败: %w", err)
		}
		redisClient = c
		sessMgr = session.NewManager(c)
		redisCleanup = func() { _ = c.Close() }
	}
	if cfg.ClusterLobbyAddr != "" && cfg.ClusterRoomAddr != "" {
		if cfg.PostgresDSN != "" {
			pool, err := postgres.OpenPool(context.Background(), cfg.PostgresDSN)
			if err != nil {
				if redisCleanup != nil {
					redisCleanup()
				}
				_ = ln.Close()
				return nil, fmt.Errorf("postgres 客户端初始化失败: %w", err)
			}
			settlements = postgres.NewSettlementStore(pool)
			pgCleanup = pool.Close
		}
		var gwCleanup func()
		gateway, gwCleanup, err = newRemoteRoomGateway(cfg, hub, sessMgr, redisClient, settlements, ln.Addr().String())
		if err != nil {
			if redisCleanup != nil {
				redisCleanup()
			}
			if pgCleanup != nil {
				pgCleanup()
			}
			_ = ln.Close()
			return nil, err
		}
		cleanup = func() {
			gwCleanup()
			if pgCleanup != nil {
				pgCleanup()
			}
			if redisCleanup != nil {
				redisCleanup()
			}
		}
	} else {
		lb := roomsvc.NewLobby()
		rs = roomsvc.NewServiceWithRule(lb, cfg.RuleID)
		gateway = handler.NewLocalRoomGateway(rs, hub, sessMgr)
		cleanup = func() {
			if redisCleanup != nil {
				redisCleanup()
			}
		}
	}
	deps := handler.Deps{Rooms: gateway, Hub: hub, Session: sessMgr, AllowedOrigins: append([]string(nil), cfg.WSAllowedOrigins...)}
	obsStop, errObs := StartObsHTTP(cfg.ObsAddr, redisClient)
	if errObs != nil {
		if redisCleanup != nil {
			redisCleanup()
		}
		_ = ln.Close()
		return nil, fmt.Errorf("可观测性 HTTP 启动失败: %w", errObs)
	}
	prevCleanup := cleanup
	cleanup = func() {
		obsStop()
		if prevCleanup != nil {
			prevCleanup()
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		tid := uuid.NewString()
		reqCtx := logx.WithTraceID(r.Context(), tid)
		handler.HandleWebSocket(reqCtx, deps, w, r)
	})
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &App{srv: srv, ln: ln, rooms: rs, cleanup: cleanup}, nil
}

// Addr 返回已绑定的监听地址（用于测试与运维探测）。
func (a *App) Addr() net.Addr {
	if a == nil || a.ln == nil {
		return nil
	}
	return a.ln.Addr()
}

// Run 启动 HTTP 服务并在 ctx 取消时优雅退出。
func (a *App) Run(ctx context.Context) error {
	if a == nil || a.srv == nil {
		return fmt.Errorf("nil app")
	}
	defer func() {
		if a.cleanup != nil {
			a.cleanup()
		}
	}()
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.srv.Serve(a.ln)
	}()
	select {
	case <-ctx.Done():
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.srv.Shutdown(shCtx)
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
