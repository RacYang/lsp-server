package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"racoo.cn/lsp/pkg/logx"
)

// GRPCApp 为 gRPC 进程装配：lobby/room 等角色可复用同一生命周期管理。
type GRPCApp struct {
	srv *grpc.Server
	ln  net.Listener
}

// NewGRPC 根据监听地址与注册回调装配 gRPC 服务。
func NewGRPC(ctx context.Context, addr string, register func(*grpc.Server)) (*GRPCApp, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
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

// traceUnaryServerInterceptor 从 gRPC metadata 中提取 trace-id 并注入 Context。
func traceUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		tid := ""
		if vals := md.Get("racoo-trace-id"); len(vals) > 0 {
			tid = vals[0]
		}
		if tid == "" {
			tid = uuid.NewString()
		}
		return handler(logx.WithTraceID(ctx, tid), req)
	}
}

type ctxServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *ctxServerStream) Context() context.Context { return s.ctx }

// traceStreamServerInterceptor 为流式 RPC 同样注入 trace-id，保持与 unary 拦截器的一致性。
func traceStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		md, _ := metadata.FromIncomingContext(ss.Context())
		tid := ""
		if vals := md.Get("racoo-trace-id"); len(vals) > 0 {
			tid = vals[0]
		}
		if tid == "" {
			tid = uuid.NewString()
		}
		return handler(srv, &ctxServerStream{ServerStream: ss, ctx: logx.WithTraceID(ss.Context(), tid)})
	}
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
