package main

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	clusterv1 "racoo.cn/lsp/api/gen/go/cluster/v1"
	"racoo.cn/lsp/internal/cluster/router"
	lobbysvc "racoo.cn/lsp/internal/service/lobby"
)

// lobbyGRPCServer 将 lobby 业务服务适配为 cluster.v1.LobbyService。
type lobbyGRPCServer struct {
	svc        *lobbysvc.Service
	claimer    *router.Etcd
	roomNodeID string
}

func newLobbyGRPCServer(svc *lobbysvc.Service, claimer *router.Etcd, roomNodeID string) *lobbyGRPCServer {
	return &lobbyGRPCServer{svc: svc, claimer: claimer, roomNodeID: roomNodeID}
}

func (s *lobbyGRPCServer) ensureClaim(ctx context.Context, roomID string) error {
	if s == nil || s.claimer == nil || roomID == "" || s.roomNodeID == "" {
		return nil
	}
	if err := s.claimer.ClaimRoom(ctx, roomID, s.roomNodeID, 0); err != nil {
		return fmt.Errorf("claim room owner: %w", err)
	}
	return nil
}

// CreateRoom 将 gRPC 请求翻译为大厅服务创建房间调用。
func (s *lobbyGRPCServer) CreateRoom(ctx context.Context, req *clusterv1.CreateRoomRequest) (*clusterv1.CreateRoomResponse, error) {
	if req.GetCreatorUserId() != "" || req.GetRoomId() == "" {
		roomID, seat, err := s.svc.CreateRoomWithMeta(ctx, req.GetRuleId(), req.GetDisplayName(), req.GetPrivate(), req.GetCreatorUserId())
		if err != nil {
			return &clusterv1.CreateRoomResponse{Error: err.Error()}, nil
		}
		if err := s.ensureClaim(ctx, roomID); err != nil {
			return &clusterv1.CreateRoomResponse{Error: err.Error()}, nil
		}
		return &clusterv1.CreateRoomResponse{RoomId: roomID, RoomNodeId: s.roomNodeIDOrLocal(), SeatIndex: seat}, nil
	}
	nodeID, err := s.svc.CreateRoom(ctx, req.GetRoomId())
	if err != nil {
		return &clusterv1.CreateRoomResponse{Error: err.Error()}, nil
	}
	if err := s.ensureClaim(ctx, req.GetRoomId()); err != nil {
		return &clusterv1.CreateRoomResponse{Error: err.Error()}, nil
	}
	return &clusterv1.CreateRoomResponse{RoomId: req.GetRoomId(), RoomNodeId: nodeID}, nil
}

// JoinRoom 在基线阶段返回本地座位分配结果，后续再替换为真实跨进程调度。
func (s *lobbyGRPCServer) JoinRoom(ctx context.Context, req *clusterv1.JoinRoomRequest) (*clusterv1.JoinRoomResponse, error) {
	seat, err := s.svc.JoinRoom(ctx, req.GetRoomId(), req.GetUserId())
	if err != nil {
		return &clusterv1.JoinRoomResponse{Error: err.Error()}, nil
	}
	if err := s.ensureClaim(ctx, req.GetRoomId()); err != nil {
		return &clusterv1.JoinRoomResponse{Error: err.Error()}, nil
	}
	return &clusterv1.JoinRoomResponse{SeatIndex: seat}, nil
}

// GetRoom 查询房间当前归属的 room 节点。
func (s *lobbyGRPCServer) GetRoom(ctx context.Context, req *clusterv1.GetRoomRequest) (*clusterv1.GetRoomResponse, error) {
	nodeID, err := s.svc.GetRoom(ctx, req.GetRoomId())
	if err != nil {
		if errors.Is(err, lobbysvc.ErrRoomNotFound) {
			return &clusterv1.GetRoomResponse{Error: err.Error()}, nil
		}
		return nil, err
	}
	return &clusterv1.GetRoomResponse{RoomId: req.GetRoomId(), RoomNodeId: nodeID}, nil
}

// ListRooms 返回可加入的公开等待房间摘要。
func (s *lobbyGRPCServer) ListRooms(ctx context.Context, req *clusterv1.ListRoomsRequest) (*clusterv1.ListRoomsResponse, error) {
	rooms, next, err := s.svc.ListRooms(ctx, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return &clusterv1.ListRoomsResponse{Error: err.Error()}, nil
	}
	return &clusterv1.ListRoomsResponse{Rooms: lobbyRoomMetasToCluster(rooms), NextPageToken: next}, nil
}

// AutoMatch 选择一个公开未满房，或在无候选时创建新公开房。
func (s *lobbyGRPCServer) AutoMatch(ctx context.Context, req *clusterv1.AutoMatchRequest) (*clusterv1.AutoMatchResponse, error) {
	roomID, seat, err := s.svc.AutoMatch(ctx, req.GetRuleId(), req.GetUserId())
	if err != nil {
		return &clusterv1.AutoMatchResponse{Error: err.Error()}, nil
	}
	if err := s.ensureClaim(ctx, roomID); err != nil {
		return &clusterv1.AutoMatchResponse{Error: err.Error()}, nil
	}
	return &clusterv1.AutoMatchResponse{RoomId: roomID, RoomNodeId: s.roomNodeIDOrLocal(), SeatIndex: seat}, nil
}

func (s *lobbyGRPCServer) roomNodeIDOrLocal() string {
	if s != nil && s.roomNodeID != "" {
		return s.roomNodeID
	}
	return "room-local"
}

type lobbyService interface {
	CreateRoom(context.Context, *clusterv1.CreateRoomRequest) (*clusterv1.CreateRoomResponse, error)
	JoinRoom(context.Context, *clusterv1.JoinRoomRequest) (*clusterv1.JoinRoomResponse, error)
	GetRoom(context.Context, *clusterv1.GetRoomRequest) (*clusterv1.GetRoomResponse, error)
	ListRooms(context.Context, *clusterv1.ListRoomsRequest) (*clusterv1.ListRoomsResponse, error)
	AutoMatch(context.Context, *clusterv1.AutoMatchRequest) (*clusterv1.AutoMatchResponse, error)
}

// registerLobbyService 手工注册 ServiceDesc，避免命令层直接绑定生成的 server 接口。
func registerLobbyService(s grpc.ServiceRegistrar, srv lobbyService) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "cluster.v1.LobbyService",
		HandlerType: (*lobbyService)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "CreateRoom", Handler: lobbyCreateRoomHandler},
			{MethodName: "JoinRoom", Handler: lobbyJoinRoomHandler},
			{MethodName: "GetRoom", Handler: lobbyGetRoomHandler},
			{MethodName: "ListRooms", Handler: lobbyListRoomsHandler},
			{MethodName: "AutoMatch", Handler: lobbyAutoMatchHandler},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "cluster/v1/lobby.proto",
	}, srv)
}

// lobbyCreateRoomHandler 为 unary RPC 解包并透传到本地服务接口。
func lobbyCreateRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(clusterv1.CreateRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(lobbyService).CreateRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.LobbyService/CreateRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(lobbyService).CreateRoom(ctx, req.(*clusterv1.CreateRoomRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// lobbyJoinRoomHandler 为加入房间 RPC 提供统一的解码与拦截器桥接。
func lobbyJoinRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(clusterv1.JoinRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(lobbyService).JoinRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.LobbyService/JoinRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(lobbyService).JoinRoom(ctx, req.(*clusterv1.JoinRoomRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// lobbyGetRoomHandler 为查询房间路由 RPC 提供统一桥接。
func lobbyGetRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(clusterv1.GetRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(lobbyService).GetRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.LobbyService/GetRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(lobbyService).GetRoom(ctx, req.(*clusterv1.GetRoomRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// lobbyListRoomsHandler 为大厅房间列表 RPC 提供统一桥接。
func lobbyListRoomsHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(clusterv1.ListRoomsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(lobbyService).ListRooms(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.LobbyService/ListRooms"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(lobbyService).ListRooms(ctx, req.(*clusterv1.ListRoomsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// lobbyAutoMatchHandler 为自动匹配 RPC 提供统一桥接。
func lobbyAutoMatchHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(clusterv1.AutoMatchRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(lobbyService).AutoMatch(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.LobbyService/AutoMatch"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(lobbyService).AutoMatch(ctx, req.(*clusterv1.AutoMatchRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func lobbyRoomMetasToCluster(rooms []lobbysvc.RoomMeta) []*clusterv1.RoomMeta {
	out := make([]*clusterv1.RoomMeta, 0, len(rooms))
	for _, room := range rooms {
		out = append(out, &clusterv1.RoomMeta{
			RoomId:      room.RoomID,
			RuleId:      room.RuleID,
			DisplayName: room.DisplayName,
			SeatCount:   room.SeatCount,
			MaxSeats:    room.MaxSeats,
			CreatedAtMs: room.CreatedAtMs,
			Stage:       room.Stage,
		})
	}
	return out
}
