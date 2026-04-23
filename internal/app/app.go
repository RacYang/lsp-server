// Package app 负责单体进程装配：HTTP/WebSocket、房间服务与会话 Hub。
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"racoo.cn/lsp/internal/config"
	"racoo.cn/lsp/internal/handler"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
)

// App 为可启动的应用实例。
type App struct {
	srv   *http.Server
	ln    net.Listener
	rooms *roomsvc.Service
}

// New 根据配置装配应用。
func New(cfg config.Config) (*App, error) {
	ln, err := net.Listen("tcp", cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("监听地址失败: %w", err)
	}
	hub := session.NewHub()
	lb := roomsvc.NewLobby()
	rs := roomsvc.NewService(lb)
	deps := handler.Deps{Rooms: rs, Hub: hub}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(r.Context(), deps, w, r)
	})
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &App{srv: srv, ln: ln, rooms: rs}, nil
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
