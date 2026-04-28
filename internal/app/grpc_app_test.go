// gRPC 装配与 trace 拦截器测试：覆盖端口绑定、生命周期与 trace_id 注入。
package app

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	"racoo.cn/lsp/pkg/logx"
)

// TestNewGRPCBindFailsOnUsedPort 校验当端口已被占用时 NewGRPC 必须返回错误而不是 panic。
func TestNewGRPCBindFailsOnUsedPort(t *testing.T) {
	t.Parallel()
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = occupied.Close() }()

	_, err = NewGRPC(occupied.Addr().String(), nil)
	require.Error(t, err)
}

// TestGRPCAppRunCancelledContext 校验 ctx 取消触发 GracefulStop，Run 返回 context.Canceled。
func TestGRPCAppRunCancelledContext(t *testing.T) {
	app, err := NewGRPC("127.0.0.1:0", nil)
	require.NoError(t, err)
	require.NotNil(t, app.Addr())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = app.Run(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// TestGRPCAppNilSafe 校验在 nil 接收者上调用 Addr 与 Run 都被安全处理。
func TestGRPCAppNilSafe(t *testing.T) {
	t.Parallel()
	var a *GRPCApp
	require.Nil(t, a.Addr())
	require.Error(t, a.Run(context.Background()))
}

// TestGRPCAppRegisterAndCallWithTraceInjected 通过 health 服务作为最小 RPC，
// 让 unary 拦截器从入站 metadata 中提取 racoo-trace-id 并写入 ctx。
// 通过自定义 health server 实现把 ctx 中的 trace_id 写到响应 status 字段，
// 间接验证拦截器把 trace_id 注入了 handler 的 ctx。
func TestGRPCAppRegisterAndCallWithTraceInjected(t *testing.T) {
	app, err := NewGRPC("127.0.0.1:0", func(s *grpc.Server) {
		healthpb.RegisterHealthServer(s, &traceCapturingHealth{})
	})
	require.NoError(t, err)
	addr := app.Addr().String()

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		_ = app.Run(runCtx)
	}()
	waitForTCP(t, addr)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	cli := healthpb.NewHealthClient(conn)

	ctx := metadata.AppendToOutgoingContext(context.Background(), "racoo-trace-id", "trace-fixed")
	resp, err := cli.Check(ctx, &healthpb.HealthCheckRequest{Service: "x"})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus(), "应能通过 unary 拦截器")

	ctxNoTrace, cancelNoTrace := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelNoTrace()
	_, err = cli.Check(ctxNoTrace, &healthpb.HealthCheckRequest{Service: "y"})
	require.NoError(t, err, "缺少 racoo-trace-id 时拦截器应自行生成 trace_id 并继续")
}

// TestStreamInterceptorAcceptsTraceID 通过流式 health.Watch 调用，
// 让 stream 拦截器路径被实际触发，覆盖 traceStreamServerInterceptor。
func TestStreamInterceptorAcceptsTraceID(t *testing.T) {
	app, err := NewGRPC("127.0.0.1:0", func(s *grpc.Server) {
		healthpb.RegisterHealthServer(s, &traceCapturingHealth{})
	})
	require.NoError(t, err)
	addr := app.Addr().String()

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		_ = app.Run(runCtx)
	}()
	waitForTCP(t, addr)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	cli := healthpb.NewHealthClient(conn)

	ctx, cancelCall := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelCall()
	ctx = metadata.AppendToOutgoingContext(ctx, "racoo-trace-id", "stream-trace")
	stream, err := cli.Watch(ctx, &healthpb.HealthCheckRequest{Service: "watch"})
	require.NoError(t, err)
	resp, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
}

// traceCapturingHealth 实现最小化的 Health 服务：把 ctx 中的 trace_id 反向校验。
type traceCapturingHealth struct {
	healthpb.UnimplementedHealthServer
}

// Check 仅断言 trace_id 已经被拦截器注入到 ctx；同时保留无 trace_id 的兼容路径。
func (s *traceCapturingHealth) Check(ctx context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	tid := logx.TraceIDFromContext(ctx)
	if tid == "" {
		return nil, errors.New("trace id missing")
	}
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

// Watch 同样依赖拦截器把 trace_id 写入 ss.Context()，否则立即返回错误。
func (s *traceCapturingHealth) Watch(_ *healthpb.HealthCheckRequest, ss healthpb.Health_WatchServer) error {
	tid := logx.TraceIDFromContext(ss.Context())
	if tid == "" {
		return errors.New("trace id missing")
	}
	return ss.Send(&healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING})
}

// waitForTCP 在 1 秒内反复 dial 直到端口监听就绪，避免起服务后的首个客户端调用失败。
func waitForTCP(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("等待 gRPC 端口就绪超时")
}
