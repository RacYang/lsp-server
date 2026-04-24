package app

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"racoo.cn/lsp/pkg/logx"
)

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

func (s *ctxServerStream) Context() context.Context {
	return s.ctx
}

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
		wrapped := &ctxServerStream{ServerStream: ss, ctx: logx.WithTraceID(ss.Context(), tid)}
		return handler(srv, wrapped)
	}
}
