package lobbyadapter

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"

	"racoo.cn/lsp/internal/cluster"
	lobbysvc "racoo.cn/lsp/internal/service/lobby"
)

// GRPCServer 将 lobby 业务服务适配为 v1.LobbyService。
type GRPCServer struct {
	svc        *lobbysvc.Service
	claimer    *cluster.EtcdRouter
	roomNodeID string
}

// NewGRPCServer 构造 lobby gRPC 适配器，可选传入 etcd 路由器用于 room 归属声明。
func NewGRPCServer(svc *lobbysvc.Service, claimer *cluster.EtcdRouter, roomNodeID string) *GRPCServer {
	return &GRPCServer{svc: svc, claimer: claimer, roomNodeID: roomNodeID}
}

func (s *GRPCServer) ensureClaim(ctx context.Context, roomID string) error {
	if s == nil || s.claimer == nil || roomID == "" || s.roomNodeID == "" {
		return nil
	}
	if err := s.claimer.ClaimRoom(ctx, roomID, s.roomNodeID, 0); err != nil {
		return fmt.Errorf("claim room owner: %w", err)
	}
	return nil
}

// CreateRoom 将 gRPC 请求翻译为大厅服务创建房间调用。
func (s *GRPCServer) CreateRoom(ctx context.Context, req *svcv1.CreateRoomRequest) (*svcv1.CreateRoomResponse, error) {
	if req.GetCreatorUserId() != "" || req.GetRoomId() == "" {
		roomID, seat, err := s.svc.CreateRoomWithMeta(ctx, req.GetRuleId(), req.GetDisplayName(), req.GetPrivate(), req.GetCreatorUserId())
		if err != nil {
			return &svcv1.CreateRoomResponse{Error: err.Error()}, nil
		}
		if err := s.ensureClaim(ctx, roomID); err != nil {
			return &svcv1.CreateRoomResponse{Error: err.Error()}, nil
		}
		return &svcv1.CreateRoomResponse{RoomId: roomID, RoomNodeId: s.roomNodeIDOrLocal(), SeatIndex: seat}, nil
	}
	nodeID, err := s.svc.CreateRoom(ctx, req.GetRoomId())
	if err != nil {
		return &svcv1.CreateRoomResponse{Error: err.Error()}, nil
	}
	if err := s.ensureClaim(ctx, req.GetRoomId()); err != nil {
		return &svcv1.CreateRoomResponse{Error: err.Error()}, nil
	}
	return &svcv1.CreateRoomResponse{RoomId: req.GetRoomId(), RoomNodeId: nodeID}, nil
}

// JoinRoom 在基线阶段返回本地座位分配结果，后续再替换为真实跨进程调度。
func (s *GRPCServer) JoinRoom(ctx context.Context, req *svcv1.JoinRoomRequest) (*svcv1.JoinRoomResponse, error) {
	seat, err := s.svc.JoinRoom(ctx, req.GetRoomId(), req.GetUserId())
	if err != nil {
		return &svcv1.JoinRoomResponse{Error: err.Error()}, nil
	}
	if err := s.ensureClaim(ctx, req.GetRoomId()); err != nil {
		return &svcv1.JoinRoomResponse{Error: err.Error()}, nil
	}
	return &svcv1.JoinRoomResponse{SeatIndex: seat}, nil
}

// LeaveRoom 立即清理大厅座位索引，让玩家离桌后可立刻加入新房。
func (s *GRPCServer) LeaveRoom(ctx context.Context, req *svcv1.LeaveRoomRequest) (*svcv1.LeaveRoomResponse, error) {
	if err := s.svc.LeaveRoom(ctx, req.GetRoomId(), req.GetUserId()); err != nil {
		return &svcv1.LeaveRoomResponse{Error: err.Error()}, nil
	}
	return &svcv1.LeaveRoomResponse{}, nil
}

// GetRoom 查询房间当前归属的 room 节点。
func (s *GRPCServer) GetRoom(ctx context.Context, req *svcv1.GetRoomRequest) (*svcv1.GetRoomResponse, error) {
	nodeID, err := s.svc.GetRoom(ctx, req.GetRoomId())
	if err != nil {
		if errors.Is(err, lobbysvc.ErrRoomNotFound) {
			return &svcv1.GetRoomResponse{Error: err.Error()}, nil
		}
		return nil, err
	}
	return &svcv1.GetRoomResponse{RoomId: req.GetRoomId(), RoomNodeId: nodeID}, nil
}

// ListRooms 返回可加入的公开等待房间摘要。
func (s *GRPCServer) ListRooms(ctx context.Context, req *svcv1.ListRoomsRequest) (*svcv1.ListRoomsResponse, error) {
	rooms, next, err := s.svc.ListRooms(ctx, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return &svcv1.ListRoomsResponse{Error: err.Error()}, nil
	}
	return &svcv1.ListRoomsResponse{Rooms: RoomMetasToProto(rooms), NextPageToken: next}, nil
}

// ListRules 返回当前后端可创建的规则清单。
func (s *GRPCServer) ListRules(ctx context.Context, _ *svcv1.ListRulesRequest) (*svcv1.ListRulesResponse, error) {
	rules, err := s.svc.ListRules(ctx)
	if err != nil {
		return &svcv1.ListRulesResponse{Error: err.Error()}, nil
	}
	return &svcv1.ListRulesResponse{Rules: RuleMetasToProto(rules)}, nil
}

// AutoMatch 选择一个公开未满房，或在无候选时创建新公开房。
func (s *GRPCServer) AutoMatch(ctx context.Context, req *svcv1.AutoMatchRequest) (*svcv1.AutoMatchResponse, error) {
	roomID, seat, err := s.svc.AutoMatch(ctx, req.GetRuleId(), req.GetUserId())
	if err != nil {
		return &svcv1.AutoMatchResponse{Error: err.Error()}, nil
	}
	if err := s.ensureClaim(ctx, roomID); err != nil {
		return &svcv1.AutoMatchResponse{Error: err.Error()}, nil
	}
	return &svcv1.AutoMatchResponse{RoomId: roomID, RoomNodeId: s.roomNodeIDOrLocal(), SeatIndex: seat}, nil
}

// AddBot 向房间补充占位机器人，返回新增的座位信息列表。
func (s *GRPCServer) AddBot(ctx context.Context, req *svcv1.AddBotRequest) (*svcv1.AddBotResponse, error) {
	added, err := s.svc.AddBot(ctx, req.GetRoomId(), req.GetCount(), 3)
	if err != nil {
		return &svcv1.AddBotResponse{Error: err.Error()}, nil
	}
	out := make([]*clientv1.SeatInfo, 0, len(added))
	for _, bot := range added {
		out = append(out, &clientv1.SeatInfo{
			SeatIndex: bot.SeatIndex,
			UserId:    bot.UserID,
			Nickname:  "机器人",
			IsBot:     true,
			Online:    true,
			AutoPlay:  true,
			Status:    "online",
		})
	}
	return &svcv1.AddBotResponse{Added: out}, nil
}

func (s *GRPCServer) roomNodeIDOrLocal() string {
	if s != nil && s.roomNodeID != "" {
		return s.roomNodeID
	}
	return "room-local"
}

// LobbyService 为 gRPC 注册桥接所需的接口。
type LobbyService interface {
	CreateRoom(context.Context, *svcv1.CreateRoomRequest) (*svcv1.CreateRoomResponse, error)
	JoinRoom(context.Context, *svcv1.JoinRoomRequest) (*svcv1.JoinRoomResponse, error)
	GetRoom(context.Context, *svcv1.GetRoomRequest) (*svcv1.GetRoomResponse, error)
	ListRooms(context.Context, *svcv1.ListRoomsRequest) (*svcv1.ListRoomsResponse, error)
	ListRules(context.Context, *svcv1.ListRulesRequest) (*svcv1.ListRulesResponse, error)
	AutoMatch(context.Context, *svcv1.AutoMatchRequest) (*svcv1.AutoMatchResponse, error)
	LeaveRoom(context.Context, *svcv1.LeaveRoomRequest) (*svcv1.LeaveRoomResponse, error)
	AddBot(context.Context, *svcv1.AddBotRequest) (*svcv1.AddBotResponse, error)
}

// RegisterService 手工注册 ServiceDesc，避免命令层直接绑定生成的 server 接口。
func RegisterService(s grpc.ServiceRegistrar, srv LobbyService) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "v1.LobbyService",
		HandlerType: (*LobbyService)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "CreateRoom", Handler: createRoomHandler},
			{MethodName: "JoinRoom", Handler: joinRoomHandler},
			{MethodName: "GetRoom", Handler: getRoomHandler},
			{MethodName: "ListRooms", Handler: listRoomsHandler},
			{MethodName: "ListRules", Handler: listRulesHandler},
			{MethodName: "AutoMatch", Handler: autoMatchHandler},
			{MethodName: "LeaveRoom", Handler: leaveRoomHandler},
			{MethodName: "AddBot", Handler: addBotHandler},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "v1/service.proto",
	}, srv)
}

// createRoomHandler 为 unary RPC 解包并透传到本地服务接口。
func createRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.CreateRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LobbyService).CreateRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.LobbyService/CreateRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LobbyService).CreateRoom(ctx, req.(*svcv1.CreateRoomRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// joinRoomHandler 为加入房间 RPC 提供统一的解码与拦截器桥接。
func joinRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.JoinRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LobbyService).JoinRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.LobbyService/JoinRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LobbyService).JoinRoom(ctx, req.(*svcv1.JoinRoomRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// getRoomHandler 为查询房间路由 RPC 提供统一桥接。
func getRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.GetRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LobbyService).GetRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.LobbyService/GetRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LobbyService).GetRoom(ctx, req.(*svcv1.GetRoomRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// listRoomsHandler 为大厅房间列表 RPC 提供统一桥接。
func listRoomsHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.ListRoomsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LobbyService).ListRooms(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.LobbyService/ListRooms"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LobbyService).ListRooms(ctx, req.(*svcv1.ListRoomsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// listRulesHandler 为规则列表 RPC 提供统一桥接。
func listRulesHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.ListRulesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LobbyService).ListRules(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.LobbyService/ListRules"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LobbyService).ListRules(ctx, req.(*svcv1.ListRulesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// autoMatchHandler 为自动匹配 RPC 提供统一桥接。
func autoMatchHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.AutoMatchRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LobbyService).AutoMatch(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.LobbyService/AutoMatch"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LobbyService).AutoMatch(ctx, req.(*svcv1.AutoMatchRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func leaveRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.LeaveRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LobbyService).LeaveRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.LobbyService/LeaveRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LobbyService).LeaveRoom(ctx, req.(*svcv1.LeaveRoomRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func addBotHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.AddBotRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(LobbyService).AddBot(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.LobbyService/AddBot"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(LobbyService).AddBot(ctx, req.(*svcv1.AddBotRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// RoomMetasToProto 把大厅内部 RoomMeta 投影为 client.v1.RoomMeta；
// proto 统一后 ListRoomsResponse.rooms 直接使用客户端类型，无须 cluster 层中转。
func RoomMetasToProto(rooms []lobbysvc.RoomMeta) []*clientv1.RoomMeta {
	out := make([]*clientv1.RoomMeta, 0, len(rooms))
	for _, room := range rooms {
		out = append(out, &clientv1.RoomMeta{
			RoomId:      room.RoomID,
			RuleId:      room.RuleID,
			DisplayName: room.DisplayName,
			SeatCount:   room.SeatCount,
			MaxSeats:    room.MaxSeats,
			CreatedAtMs: room.CreatedAtMs,
			Stage:       room.Stage,
			RuleMeta:    RuleMetaToProto(room.RuleMeta),
		})
	}
	return out
}

// RuleMetasToProto 批量投影规则摘要为 client.v1.RuleMeta。
func RuleMetasToProto(rules []lobbysvc.RuleMeta) []*clientv1.RuleMeta {
	out := make([]*clientv1.RuleMeta, 0, len(rules))
	for _, rule := range rules {
		out = append(out, RuleMetaToProto(rule))
	}
	return out
}

// RuleMetaToProto 投影单条规则摘要；字段为空时返回 nil。
func RuleMetaToProto(meta lobbysvc.RuleMeta) *clientv1.RuleMeta {
	if meta.RuleID == "" && meta.DisplayName == "" {
		return nil
	}
	return &clientv1.RuleMeta{
		RuleId:          meta.RuleID,
		DisplayName:     meta.DisplayName,
		ShortDesc:       meta.ShortDesc,
		EnabledFeatures: append([]string(nil), meta.EnabledFeatures...),
		MaxHands:        meta.MaxHands,
	}
}
