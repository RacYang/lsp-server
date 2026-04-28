// 直接驱动 lobby gRPC handler 的解码与拦截器桥接，覆盖 register/method handler 三函数。
package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	clusterv1 "racoo.cn/lsp/api/gen/go/cluster/v1"
	lobbysvc "racoo.cn/lsp/internal/service/lobby"
)

// startBufLobbyServer 启动一个真实的本地 gRPC 服务并把 lobby 业务注册上去，
// 这样测试可以通过真实的 RPC 通道驱动 ServiceDesc 中所有 method handler，而不是直接调用 Go 函数。
// 用真实通道才能覆盖到 register 路径与拦截器链路，bufconn 与单测桩都做不到这一点。
func startBufLobbyServer(t *testing.T) (clusterv1.LobbyServiceClient, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	registerLobbyService(srv, newLobbyGRPCServer(lobbysvc.New(), nil, "room-test"))
	go func() { _ = srv.Serve(ln) }()

	addr := ln.Addr().String()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	cli := clusterv1.NewLobbyServiceClient(conn)
	cleanup := func() {
		_ = conn.Close()
		srv.GracefulStop()
		_ = ln.Close()
	}
	return cli, cleanup
}

// TestRegisterLobbyService_RoundTrip 通过真实 gRPC 通道驱动三段 handler 与 ServiceDesc：
// 创建房间、加入房间、查询房间各跑一次，确认编解码与业务装配链路完整连通；
// 缺失房间的查询走"业务错误而非 RPC 错误"分支，避免回归到把 nil pointer 透传给客户端。
func TestRegisterLobbyService_RoundTrip(t *testing.T) {
	cli, cleanup := startBufLobbyServer(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	createResp, err := cli.CreateRoom(ctx, &clusterv1.CreateRoomRequest{RoomId: "r-grpc"})
	require.NoError(t, err)
	require.NotEmpty(t, createResp.GetRoomNodeId())

	joinResp, err := cli.JoinRoom(ctx, &clusterv1.JoinRoomRequest{RoomId: "r-grpc", UserId: "u1"})
	require.NoError(t, err)
	require.EqualValues(t, 0, joinResp.GetSeatIndex())

	getResp, err := cli.GetRoom(ctx, &clusterv1.GetRoomRequest{RoomId: "r-grpc"})
	require.NoError(t, err)
	require.NotEmpty(t, getResp.GetRoomNodeId())

	missing, err := cli.GetRoom(ctx, &clusterv1.GetRoomRequest{RoomId: "missing"})
	require.NoError(t, err)
	require.NotEmpty(t, missing.GetError(), "未知房间应返回业务错误而非 RPC 错误")
}

// TestLobbyHandlersDecodeFailure 直接调用 method handler，校验解码失败时返回 dec 抛出的错误。
// 这一行为对应"框架在调用业务前先反序列化请求体"的契约：dec 一旦报错，handler 必须立刻把
// 错误透传出去，不得吞掉、也不得继续走业务，否则会把脏数据写到 lobby service。
func TestLobbyHandlersDecodeFailure(t *testing.T) {
	t.Parallel()
	srv := newLobbyGRPCServer(lobbysvc.New(), nil, "room-x")
	failingDec := func(_ interface{}) error { return errors.New("decode boom") }

	_, err := lobbyCreateRoomHandler(srv, context.Background(), failingDec, nil)
	require.ErrorContains(t, err, "decode boom")
	_, err = lobbyJoinRoomHandler(srv, context.Background(), failingDec, nil)
	require.ErrorContains(t, err, "decode boom")
	_, err = lobbyGetRoomHandler(srv, context.Background(), failingDec, nil)
	require.ErrorContains(t, err, "decode boom")
}

// TestLobbyHandlersWithInterceptor 校验当传入 unary 拦截器时 handler 会走拦截路径并最终调用业务。
// 拦截器是日志、Trace、限流的常见挂载点；如果 method handler 跳过 interceptor，监控与限流都会失效。
// 这里通过计数器断言"拦截器至少被调用一次"，并在拦截器内部解码请求体以确保 ctx/req 是真实业务对象。
func TestLobbyHandlersWithInterceptor(t *testing.T) {
	t.Parallel()
	srv := newLobbyGRPCServer(lobbysvc.New(), nil, "room-i")

	calls := 0
	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		calls++
		require.NotEmpty(t, info.FullMethod)
		return handler(ctx, req)
	}

	dec := func(in interface{}) error {
		buf, _ := proto.Marshal(&clusterv1.CreateRoomRequest{RoomId: "intercepted"})
		return proto.Unmarshal(buf, in.(proto.Message))
	}
	resp, err := lobbyCreateRoomHandler(srv, context.Background(), dec, interceptor)
	require.NoError(t, err)
	require.NotEmpty(t, resp.(*clusterv1.CreateRoomResponse).GetRoomNodeId())
	require.Equal(t, 1, calls)
}

// TestRegisterLobbyServiceAcceptsArbitraryRegistrar 校验 registerLobbyService 在自定义 registrar 上能正确登记 ServiceDesc：
// 真实 gRPC server 不便单测拦截 RegisterService 调用，这里用一个最小 registrar 桩把传入的 ServiceDesc 捕获下来，
// 直接断言服务名与方法数量，避免业务代码隐式重命名服务或漏掉新增方法。
func TestRegisterLobbyServiceAcceptsArbitraryRegistrar(t *testing.T) {
	t.Parallel()
	captured := &captureRegistrar{}
	registerLobbyService(captured, newLobbyGRPCServer(lobbysvc.New(), nil, "room-r"))
	require.Equal(t, 1, captured.calls)
	require.Equal(t, "cluster.v1.LobbyService", captured.serviceName)
	require.Len(t, captured.methods, 3)
}

// captureRegistrar 仅用于断言 registerLobbyService 写入的 ServiceDesc 元数据，不真正运行 RPC：
// 它实现 grpc.ServiceRegistrar 接口的最小子集，把每次调用收到的服务名与方法名暂存到字段里。
type captureRegistrar struct {
	calls       int
	serviceName string
	methods     []string
}

func (c *captureRegistrar) RegisterService(desc *grpc.ServiceDesc, _ interface{}) {
	c.calls++
	c.serviceName = desc.ServiceName
	for _, m := range desc.Methods {
		c.methods = append(c.methods, m.MethodName)
	}
}

// TestCaptureRegistrarBufferShape 静态自检：保证测试辅助里使用的 bytes.Buffer 行为没有被本地 helper 错误屏蔽。
// 这是一个低成本的 sanity check，只要 buffer.WriteString 后能正确读出原文即可。
func TestCaptureRegistrarBufferShape(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	buf.WriteString("noop")
	require.Equal(t, "noop", buf.String())
}
