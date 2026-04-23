package main

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	clusterv1 "racoo.cn/lsp/api/gen/go/cluster/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
)

// roomGRPCServer 将 room 节点事件流暴露为 cluster.v1.RoomService。
type roomGRPCServer struct {
	rooms   *roomsvc.Service
	mu      sync.Mutex
	streams map[string][]chan *clusterv1.RoomServiceStreamEventsResponse
}

func newRoomGRPCServer(rooms *roomsvc.Service) *roomGRPCServer {
	return &roomGRPCServer{
		rooms:   rooms,
		streams: make(map[string][]chan *clusterv1.RoomServiceStreamEventsResponse),
	}
}

// ApplyEvent 通过 room.Service 驱动真实房间 worker，并把产出的通知桥接到订阅流。
func (s *roomGRPCServer) ApplyEvent(ctx context.Context, req *clusterv1.ApplyEventRequest) (*clusterv1.ApplyEventResponse, error) {
	if s == nil {
		return nil, fmt.Errorf("nil room grpc server")
	}
	if s.rooms == nil {
		return nil, fmt.Errorf("nil room service")
	}
	roomID := req.GetRoomId()
	if roomID == "" {
		return &clusterv1.ApplyEventResponse{Accepted: false, Error: "empty room_id"}, nil
	}
	userID := req.GetUserId()
	if userID == "" {
		return &clusterv1.ApplyEventResponse{Accepted: false, Error: "empty user_id"}, nil
	}
	if _, err := s.rooms.Join(ctx, roomID, userID); err != nil && err.Error() != "room full" {
		return &clusterv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
	}
	switch req.GetBody().(type) {
	case *clusterv1.ApplyEventRequest_Ready:
		notifications, err := s.rooms.Ready(ctx, roomID, userID)
		if err != nil {
			return &clusterv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
		}
		for idx, notification := range notifications {
			evt, err := mapNotificationToEvent(roomID, idx, notification)
			if err != nil {
				return &clusterv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
			}
			s.publish(roomID, evt)
		}
		return &clusterv1.ApplyEventResponse{Accepted: true}, nil
	default:
		return &clusterv1.ApplyEventResponse{Accepted: false, Error: "unsupported room event"}, nil
	}
}

// StreamEvents 维护每个房间的订阅通道，供 gate 或测试回放消费。
func (s *roomGRPCServer) StreamEvents(req *clusterv1.StreamEventsRequest, stream clusterv1.RoomService_StreamEventsServer) error {
	if s == nil {
		return fmt.Errorf("nil room grpc server")
	}
	roomID := req.GetRoomId()
	// 一局自动回放会连续产出多条事件，缓冲区需覆盖完整 burst，避免结算尾包被慢消费者丢弃。
	ch := make(chan *clusterv1.RoomServiceStreamEventsResponse, 128)
	s.mu.Lock()
	s.streams[roomID] = append(s.streams[roomID], ch)
	s.mu.Unlock()
	defer s.removeStream(roomID, ch)

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case evt := <-ch:
			if err := stream.Send(evt); err != nil {
				return err
			}
		}
	}
}

// publish 采用有界投递，慢订阅者由后续背压策略进一步细化。
func (s *roomGRPCServer) publish(roomID string, evt *clusterv1.RoomServiceStreamEventsResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.streams[roomID] {
		select {
		case ch <- evt:
		default:
		}
	}
}

// removeStream 在客户端断开后回收订阅槽位。
func (s *roomGRPCServer) removeStream(roomID string, target chan *clusterv1.RoomServiceStreamEventsResponse) {
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

// mapNotificationToEvent 将 room worker 产出的 client 通知翻译为 cluster 抽象事件。
func mapNotificationToEvent(roomID string, idx int, notification roomsvc.Notification) (*clusterv1.RoomServiceStreamEventsResponse, error) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(notification.Payload, &env); err != nil {
		return nil, fmt.Errorf("unmarshal room notification: %w", err)
	}
	resp := &clusterv1.RoomServiceStreamEventsResponse{
		RoomId: roomID,
		Cursor: fmt.Sprintf("%s-%d", notification.Kind, idx),
	}
	switch notification.Kind {
	case roomsvc.KindExchangeThreeDone:
		perSeat := env.GetExchangeThreeDone().GetPerSeat()
		seatTiles := make([]*clusterv1.SeatTiles, 0, len(perSeat))
		for _, item := range perSeat {
			seatTiles = append(seatTiles, &clusterv1.SeatTiles{
				SeatIndex: item.GetSeatIndex(),
				Tiles:     append([]string(nil), item.GetTiles()...),
			})
		}
		resp.Body = &clusterv1.RoomServiceStreamEventsResponse_ExchangeThreeDone{
			ExchangeThreeDone: &clusterv1.ExchangeThreeDoneEvent{SeatTiles: seatTiles},
		}
	case roomsvc.KindQueMenDone:
		resp.Body = &clusterv1.RoomServiceStreamEventsResponse_QueMenDone{
			QueMenDone: &clusterv1.QueMenDoneEvent{QueSuitBySeat: append([]int32(nil), env.GetQueMenDone().GetQueSuitBySeat()...)},
		}
	case roomsvc.KindStartGame:
		resp.Body = &clusterv1.RoomServiceStreamEventsResponse_StartGame{
			StartGame: &clusterv1.StartGameEvent{DealerSeat: env.GetStartGame().GetDealerSeat()},
		}
	case roomsvc.KindDrawTile:
		resp.Body = &clusterv1.RoomServiceStreamEventsResponse_DrawTile{
			DrawTile: &clusterv1.DrawTileEvent{
				SeatIndex: env.GetDrawTile().GetSeatIndex(),
				Tile:      env.GetDrawTile().GetTile(),
			},
		}
	case roomsvc.KindAction:
		resp.Body = &clusterv1.RoomServiceStreamEventsResponse_Action{
			Action: &clusterv1.ActionEvent{
				SeatIndex: env.GetAction().GetSeatIndex(),
				Action:    env.GetAction().GetAction(),
				Tile:      env.GetAction().GetTile(),
			},
		}
	case roomsvc.KindSettlement:
		resp.Body = &clusterv1.RoomServiceStreamEventsResponse_Settlement{
			Settlement: &clusterv1.SettlementEvent{
				WinnerUserIds: append([]string(nil), env.GetSettlement().GetWinnerUserIds()...),
				TotalFan:      env.GetSettlement().GetTotalFan(),
				DetailText:    env.GetSettlement().GetDetailText(),
			},
		}
	default:
		return nil, fmt.Errorf("unsupported notification kind: %s", notification.Kind)
	}
	return resp, nil
}

type roomService interface {
	ApplyEvent(context.Context, *clusterv1.ApplyEventRequest) (*clusterv1.ApplyEventResponse, error)
	StreamEvents(*clusterv1.StreamEventsRequest, grpc.ServerStreamingServer[clusterv1.RoomServiceStreamEventsResponse]) error
}

// registerRoomService 手工注册 ServiceDesc，避免命令层直接依赖生成 server 接口。
func registerRoomService(s grpc.ServiceRegistrar, srv roomService) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "cluster.v1.RoomService",
		HandlerType: (*roomService)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "ApplyEvent", Handler: roomApplyEventHandler},
		},
		Streams: []grpc.StreamDesc{
			{StreamName: "StreamEvents", Handler: roomStreamEventsHandler, ServerStreams: true},
		},
		Metadata: "cluster/v1/room.proto",
	}, srv)
}

// roomApplyEventHandler 为 unary ApplyEvent 做统一解包与拦截器桥接。
func roomApplyEventHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(clusterv1.ApplyEventRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(roomService).ApplyEvent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.RoomService/ApplyEvent"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(roomService).ApplyEvent(ctx, req.(*clusterv1.ApplyEventRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// roomStreamEventsHandler 为服务端流式订阅建立请求与 stream 桥接。
func roomStreamEventsHandler(srv interface{}, stream grpc.ServerStream) error {
	in := new(clusterv1.StreamEventsRequest)
	if err := stream.RecvMsg(in); err != nil {
		return err
	}
	return srv.(roomService).StreamEvents(in, &grpc.GenericServerStream[clusterv1.StreamEventsRequest, clusterv1.RoomServiceStreamEventsResponse]{ServerStream: stream})
}
