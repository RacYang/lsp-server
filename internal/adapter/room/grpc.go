package roomadapter

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
)

// RoomGRPCHandlers 是 gRPC 注册桥接所需的方法集；区别于 service/room.RoomService（业务接口）。
type RoomGRPCHandlers interface {
	ApplyEvent(context.Context, *svcv1.ApplyEventRequest) (*svcv1.ApplyEventResponse, error)
	SnapshotRoom(context.Context, *svcv1.SnapshotRoomRequest) (*svcv1.SnapshotRoomResponse, error)
	GetRoomEvents(context.Context, *svcv1.GetRoomEventsRequest) (*svcv1.GetRoomEventsResponse, error)
}

// RegisterService 手工注册 ServiceDesc，避免命令层直接依赖生成 server 接口。
func RegisterService(s grpc.ServiceRegistrar, srv RoomGRPCHandlers) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "v1.RoomService",
		HandlerType: (*RoomGRPCHandlers)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "ApplyEvent", Handler: roomApplyEventHandler},
			{MethodName: "SnapshotRoom", Handler: roomSnapshotRoomHandler},
			{MethodName: "GetRoomEvents", Handler: roomGetRoomEventsHandler},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "v1/service.proto",
	}, srv)
}

// roomApplyEventHandler 为 unary ApplyEvent 做统一解包与拦截器桥接。
func roomApplyEventHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.ApplyEventRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RoomGRPCHandlers).ApplyEvent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.RoomService/ApplyEvent"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RoomGRPCHandlers).ApplyEvent(ctx, req.(*svcv1.ApplyEventRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func roomSnapshotRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.SnapshotRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RoomGRPCHandlers).SnapshotRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.RoomService/SnapshotRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RoomGRPCHandlers).SnapshotRoom(ctx, req.(*svcv1.SnapshotRoomRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func roomGetRoomEventsHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.GetRoomEventsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RoomGRPCHandlers).GetRoomEvents(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.RoomService/GetRoomEvents"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RoomGRPCHandlers).GetRoomEvents(ctx, req.(*svcv1.GetRoomEventsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// GetRoomEvents 返回指定游标之后的全部历史事件，供 gate 在重连时补齐遗漏帧。
// 实时事件由 Redis BLPOP 获取；本接口仅查询历史，不阻塞等待新事件。
func (s *GRPCServer) GetRoomEvents(ctx context.Context, req *svcv1.GetRoomEventsRequest) (*svcv1.GetRoomEventsResponse, error) {
	if s == nil {
		return nil, fmt.Errorf("nil room grpc server")
	}
	roomID := req.GetRoomId()
	sinceSeq := parseSinceSeq(roomID, req.GetSinceCursor())
	if s.ev == nil {
		return &svcv1.GetRoomEventsResponse{}, nil
	}
	rows, err := s.ev.ListEventsAfter(ctx, roomID, sinceSeq)
	if err != nil {
		return nil, err
	}
	evts := make([]*svcv1.RoomServiceStreamEventsResponse, 0, len(rows))
	for _, row := range rows {
		evt, err := mapPGRowToEvent(roomID, row)
		if err != nil {
			return nil, err
		}
		evts = append(evts, evt)
	}
	return &svcv1.GetRoomEventsResponse{Events: evts}, nil
}

func parseSinceSeq(roomID, since string) int64 {
	if since == "" {
		return 0
	}
	prefix := roomID + ":"
	if strings.HasPrefix(since, prefix) {
		rest := strings.TrimPrefix(since, prefix)
		n, err := strconv.ParseInt(rest, 10, 64)
		if err == nil {
			return n
		}
	}
	return 0
}
