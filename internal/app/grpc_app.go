package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
)

// GRPCApp 为 gRPC 进程装配：lobby/room 等角色可复用同一生命周期管理。
type GRPCApp struct {
	srv *grpc.Server
	ln  net.Listener
}

// NewGRPC 根据监听地址与注册回调装配 gRPC 服务。
func NewGRPC(addr string, register func(*grpc.Server)) (*GRPCApp, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("监听 gRPC 地址失败: %w", err)
	}
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(traceUnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(traceStreamServerInterceptor()),
	)
	if register != nil {
		register(srv)
	}
	return &GRPCApp{srv: srv, ln: ln}, nil
}

// Addr 返回已绑定监听地址。
func (a *GRPCApp) Addr() net.Addr {
	if a == nil || a.ln == nil {
		return nil
	}
	return a.ln.Addr()
}

// Run 启动 gRPC 服务并在 ctx 取消时优雅退出。
func (a *GRPCApp) Run(ctx context.Context) error {
	if a == nil || a.srv == nil {
		return fmt.Errorf("nil grpc app")
	}
	errCh := make(chan error, 1)
	go func() { errCh <- a.srv.Serve(a.ln) }()
	select {
	case <-ctx.Done():
		done := make(chan struct{})
		go func() {
			a.srv.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			a.srv.Stop()
		}
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
}
