package roomadapter

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
)

// RoomService 为 gRPC 注册桥接所需的接口。
type RoomService interface {
	ApplyEvent(context.Context, *svcv1.ApplyEventRequest) (*svcv1.ApplyEventResponse, error)
	StreamEvents(*svcv1.StreamEventsRequest, grpc.ServerStreamingServer[svcv1.RoomServiceStreamEventsResponse]) error
	SnapshotRoom(context.Context, *svcv1.SnapshotRoomRequest) (*svcv1.SnapshotRoomResponse, error)
}

// RegisterService 手工注册 ServiceDesc，避免命令层直接依赖生成 server 接口。
func RegisterService(s grpc.ServiceRegistrar, srv RoomService) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "v1.RoomService",
		HandlerType: (*RoomService)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "ApplyEvent", Handler: roomApplyEventHandler},
			{MethodName: "SnapshotRoom", Handler: roomSnapshotRoomHandler},
		},
		Streams: []grpc.StreamDesc{
			{StreamName: "StreamEvents", Handler: roomStreamEventsHandler, ServerStreams: true},
		},
		Metadata: "cluster/v1/room.proto",
	}, srv)
}

// roomApplyEventHandler 为 unary ApplyEvent 做统一解包与拦截器桥接。
func roomApplyEventHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.ApplyEventRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RoomService).ApplyEvent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.RoomService/ApplyEvent"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RoomService).ApplyEvent(ctx, req.(*svcv1.ApplyEventRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func roomSnapshotRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.SnapshotRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RoomService).SnapshotRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.RoomService/SnapshotRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RoomService).SnapshotRoom(ctx, req.(*svcv1.SnapshotRoomRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// roomStreamEventsHandler 为服务端流式订阅建立请求与 stream 桥接。
func roomStreamEventsHandler(srv interface{}, stream grpc.ServerStream) error {
	in := new(svcv1.StreamEventsRequest)
	if err := stream.RecvMsg(in); err != nil {
		return err
	}
	return srv.(RoomService).StreamEvents(in, &grpc.GenericServerStream[svcv1.StreamEventsRequest, svcv1.RoomServiceStreamEventsResponse]{ServerStream: stream})
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

// StreamEvents 先按游标从 PostgreSQL 重放，再订阅实时通道。
// 已废弃（deprecated）：实时事件路径改用 Redis List + BLPOP；本 RPC 仅保留向后兼容。
func (s *GRPCServer) StreamEvents(req *svcv1.StreamEventsRequest, stream svcv1.RoomService_StreamEventsServer) error {
	if s == nil {
		return fmt.Errorf("nil room grpc server")
	}
	if !s.ready.Load() {
		return fmt.Errorf("recovering")
	}
	roomID := req.GetRoomId()
	ctx := stream.Context()
	sinceSeq := parseSinceSeq(roomID, req.GetSinceCursor())
	ch := make(chan *svcv1.RoomServiceStreamEventsResponse, 128)
	s.mu.Lock()
	s.streams[roomID] = append(s.streams[roomID], ch)
	s.mu.Unlock()
	defer s.removeStream(roomID, ch)

	lastSentSeq := sinceSeq
	if s.ev != nil {
		rows, err := s.ev.ListEventsAfter(ctx, roomID, sinceSeq)
		if err != nil {
			return err
		}
		for _, row := range rows {
			evt, err := mapPGRowToEvent(roomID, row)
			if err != nil {
				return err
			}
			if err := stream.Send(evt); err != nil {
				return err
			}
			if row.Seq > lastSentSeq {
				lastSentSeq = row.Seq
			}
		}
	}
	for {
		select {
		case evt := <-ch:
			if evt == nil {
				continue
			}
			evtSeq := parseSinceSeq(roomID, evt.GetCursor())
			if evtSeq > 0 && evtSeq <= lastSentSeq {
				continue
			}
			if err := stream.Send(evt); err != nil {
				return err
			}
			if evtSeq > lastSentSeq {
				lastSentSeq = evtSeq
			}
		default:
			goto liveLoop
		}
	}

liveLoop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt := <-ch:
			if evt == nil {
				continue
			}
			evtSeq := parseSinceSeq(roomID, evt.GetCursor())
			if evtSeq > 0 && evtSeq <= lastSentSeq {
				continue
			}
			if err := stream.Send(evt); err != nil {
				return err
			}
			if evtSeq > lastSentSeq {
				lastSentSeq = evtSeq
			}
		}
	}
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

// publish 对恢复链路采用阻塞投递，避免 snapshot/replay cutover 后静默丢帧。
func (s *GRPCServer) publish(roomID string, evt *svcv1.RoomServiceStreamEventsResponse) {
	s.mu.Lock()
	subs := append([]chan *svcv1.RoomServiceStreamEventsResponse(nil), s.streams[roomID]...)
	s.mu.Unlock()
	for _, ch := range subs {
		ch <- evt
	}
}

// removeStream 在客户端断开后回收订阅槽位。
func (s *GRPCServer) removeStream(roomID string, target chan *svcv1.RoomServiceStreamEventsResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.streams[roomID]
	out := cur[:0]
	for _, ch := range cur {
		if ch != target {
			out = append(out, ch)
		}
	}
	if len(out) == 0 {
		delete(s.streams, roomID)
		return
	}
	s.streams[roomID] = out
}
