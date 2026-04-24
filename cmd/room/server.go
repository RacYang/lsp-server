package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	clusterv1 "racoo.cn/lsp/api/gen/go/cluster/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
)

// roomGRPCServer 将 room 节点事件流暴露为 cluster.v1.RoomService。
type roomGRPCServer struct {
	rooms *roomsvc.Service
	ev    *postgres.RoomEventStore
	gs    *postgres.GameSummaryStore
	st    *postgres.SettlementStore
	rdb   *redis.Client

	ready   atomic.Bool
	mu      sync.Mutex
	streams map[string][]chan *clusterv1.RoomServiceStreamEventsResponse
}

func newRoomGRPCServer(rooms *roomsvc.Service, ev *postgres.RoomEventStore, gs *postgres.GameSummaryStore, st *postgres.SettlementStore, rdb *redis.Client) *roomGRPCServer {
	srv := &roomGRPCServer{
		rooms:   rooms,
		ev:      ev,
		gs:      gs,
		st:      st,
		rdb:     rdb,
		streams: make(map[string][]chan *clusterv1.RoomServiceStreamEventsResponse),
	}
	srv.ready.Store(true)
	return srv
}

func (s *roomGRPCServer) setReady(v bool) {
	if s == nil {
		return
	}
	s.ready.Store(v)
}

// ApplyEvent 通过 room.Service 驱动真实房间 worker，并把产出的通知桥接到订阅流。
func (s *roomGRPCServer) ApplyEvent(ctx context.Context, req *clusterv1.ApplyEventRequest) (*clusterv1.ApplyEventResponse, error) {
	if s == nil {
		return nil, fmt.Errorf("nil room grpc server")
	}
	if !s.ready.Load() {
		return &clusterv1.ApplyEventResponse{Accepted: false, Error: "recovering"}, nil
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
	idemKey := strings.TrimSpace(req.GetIdempotencyKey())
	if idemKey != "" && s.rdb != nil {
		scope := "room_apply_event"
		fullKey := roomID + ":" + idemKey
		rec, ok, err := s.rdb.GetIdempotency(ctx, scope, fullKey)
		if err != nil {
			return &clusterv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
		}
		if ok && rec.Result == "ok" {
			return &clusterv1.ApplyEventResponse{Accepted: true}, nil
		}
	}
	if _, err := s.rooms.Join(ctx, roomID, userID); err != nil && err.Error() != "room full" {
		return &clusterv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
	}
	s.persistRoomMeta(ctx, roomID, 0, nil)
	switch req.GetBody().(type) {
	case *clusterv1.ApplyEventRequest_Ready:
		notifications, err := s.rooms.Ready(ctx, roomID, userID)
		if err != nil {
			return &clusterv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
		}
		for idx, notification := range notifications {
			cursor, err := s.persistAndCursor(ctx, roomID, idx, notification)
			if err != nil {
				return &clusterv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
			}
			evt, err := mapNotificationToEvent(roomID, cursor, notification)
			if err != nil {
				return &clusterv1.ApplyEventResponse{Accepted: false, Error: err.Error()}, nil
			}
			s.publish(roomID, evt)
			s.afterEventSideEffects(ctx, roomID, notification, evt, cursor)
		}
		if idemKey != "" && s.rdb != nil {
			scope := "room_apply_event"
			fullKey := roomID + ":" + idemKey
			// 业务已成功：幂等键落库尽力而为，失败也不回滚已发布事件。
			_, _ = s.rdb.PutIdempotencyAbsent(ctx, scope, fullKey, redis.IdempotencyRecord{Result: "ok"}, 0)
		}
		return &clusterv1.ApplyEventResponse{Accepted: true}, nil
	default:
		return &clusterv1.ApplyEventResponse{Accepted: false, Error: "unsupported room event"}, nil
	}
}

func (s *roomGRPCServer) persistAndCursor(ctx context.Context, roomID string, idx int, notification roomsvc.Notification) (string, error) {
	if s.ev == nil {
		return fmt.Sprintf("%s-%d", notification.Kind, idx), nil
	}
	_, cursor, err := s.ev.AppendEvent(ctx, roomID, string(notification.Kind), notification.Payload)
	return cursor, err
}

func (s *roomGRPCServer) afterEventSideEffects(ctx context.Context, roomID string, notification roomsvc.Notification, evt *clusterv1.RoomServiceStreamEventsResponse, cursor string) {
	s.persistRoomMeta(ctx, roomID, parseSinceSeq(roomID, cursor), &notification)
	if s.gs != nil {
		players, _, ok := s.rooms.RoomSnapshot(roomID)
		if ok && len(players) > 0 {
			_ = s.gs.CreateGameSummary(ctx, roomID, s.rooms.RuleID(), append([]string(nil), players...))
		}
	}
	if notification.Kind == roomsvc.KindSettlement && s.st != nil && evt.GetSettlement() != nil {
		st := evt.GetSettlement()
		_ = s.st.AppendSettlement(ctx, &clientv1.SettlementNotify{
			RoomId:        roomID,
			WinnerUserIds: append([]string(nil), st.GetWinnerUserIds()...),
			TotalFan:      st.GetTotalFan(),
			DetailText:    st.GetDetailText(),
		})
	}
	if notification.Kind == roomsvc.KindSettlement && s.gs != nil {
		_ = s.gs.EndGameSummary(ctx, roomID, time.Now().UTC())
	}
}

func (s *roomGRPCServer) persistRoomMeta(ctx context.Context, roomID string, seq int64, notification *roomsvc.Notification) {
	if s == nil || s.rdb == nil {
		return
	}
	players, state, ok := s.rooms.RoomSnapshot(roomID)
	if !ok {
		state = ""
	}
	if seq == 0 && s.ev != nil {
		if maxSeq, err := s.ev.MaxSeq(ctx, roomID); err == nil {
			seq = maxSeq
		}
	}
	meta := redis.RoomSnapMeta{
		Seq:       seq,
		PlayerIDs: append([]string(nil), players...),
		State:     state,
	}
	if notification != nil {
		meta.QueSuits = queSuitsFromNotification(*notification)
	}
	_ = s.rdb.PutRoomSnapMeta(ctx, roomID, meta, 0)
}

func queSuitsFromNotification(n roomsvc.Notification) []int32 {
	if n.Kind != roomsvc.KindQueMenDone {
		return nil
	}
	var env clientv1.Envelope
	if err := proto.Unmarshal(n.Payload, &env); err != nil {
		return nil
	}
	return append([]int32(nil), env.GetQueMenDone().GetQueSuitBySeat()...)
}

// SnapshotRoom 返回快照游标与房间摘要；无持久化时退化为内存视图。
func (s *roomGRPCServer) SnapshotRoom(ctx context.Context, req *clusterv1.SnapshotRoomRequest) (*clusterv1.SnapshotRoomResponse, error) {
	if s == nil || s.rooms == nil {
		return nil, fmt.Errorf("nil room grpc server")
	}
	if !s.ready.Load() {
		return &clusterv1.SnapshotRoomResponse{Error: "recovering"}, nil
	}
	roomID := req.GetRoomId()
	if roomID == "" {
		return &clusterv1.SnapshotRoomResponse{Error: "empty room_id"}, nil
	}
	players, state, ok := s.rooms.RoomSnapshot(roomID)
	if !ok {
		if s.rdb != nil {
			if meta, okm, _ := s.rdb.GetRoomSnapMeta(ctx, roomID); okm {
				cur := ""
				if s.ev != nil {
					if m, err := s.ev.MaxSeq(ctx, roomID); err == nil && m > 0 {
						cur = fmt.Sprintf("%s:%d", roomID, m)
					}
				}
				return &clusterv1.SnapshotRoomResponse{
					Cursor:        cur,
					PlayerIds:     append([]string(nil), meta.PlayerIDs...),
					QueSuitBySeat: append([]int32(nil), meta.QueSuits...),
					State:         meta.State,
				}, nil
			}
		}
		return &clusterv1.SnapshotRoomResponse{Error: "room not found"}, nil
	}
	var maxSeq int64
	if s.ev != nil {
		if m, err := s.ev.MaxSeq(ctx, roomID); err == nil {
			maxSeq = m
		}
	}
	cur := ""
	if maxSeq > 0 {
		cur = fmt.Sprintf("%s:%d", roomID, maxSeq)
	}
	qs := pickLastQueSuits(ctx, s, roomID)
	return &clusterv1.SnapshotRoomResponse{
		Cursor:        cur,
		PlayerIds:     players,
		QueSuitBySeat: qs,
		State:         state,
	}, nil
}

func pickLastQueSuits(ctx context.Context, s *roomGRPCServer, roomID string) []int32 {
	if s.ev == nil {
		return nil
	}
	rows, err := s.ev.ListEventsAfter(ctx, roomID, 0)
	if err != nil {
		return nil
	}
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Kind != string(roomsvc.KindQueMenDone) {
			continue
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(rows[i].Payload, &env); err != nil {
			return nil
		}
		return append([]int32(nil), env.GetQueMenDone().GetQueSuitBySeat()...)
	}
	return nil
}

// StreamEvents 先按游标从 PostgreSQL 重放，再订阅实时通道。
func (s *roomGRPCServer) StreamEvents(req *clusterv1.StreamEventsRequest, stream clusterv1.RoomService_StreamEventsServer) error {
	if s == nil {
		return fmt.Errorf("nil room grpc server")
	}
	if !s.ready.Load() {
		return fmt.Errorf("recovering")
	}
	roomID := req.GetRoomId()
	ctx := stream.Context()
	sinceSeq := parseSinceSeq(roomID, req.GetSinceCursor())
	ch := make(chan *clusterv1.RoomServiceStreamEventsResponse, 128)
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
func (s *roomGRPCServer) publish(roomID string, evt *clusterv1.RoomServiceStreamEventsResponse) {
	s.mu.Lock()
	subs := append([]chan *clusterv1.RoomServiceStreamEventsResponse(nil), s.streams[roomID]...)
	s.mu.Unlock()
	for _, ch := range subs {
		ch <- evt
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

func mapPGRowToEvent(roomID string, row postgres.RoomEventRow) (*clusterv1.RoomServiceStreamEventsResponse, error) {
	n := roomsvc.Notification{Kind: roomsvc.Kind(row.Kind), Payload: append([]byte(nil), row.Payload...)}
	cur := fmt.Sprintf("%s:%d", roomID, row.Seq)
	return mapNotificationToEvent(roomID, cur, n)
}

// mapNotificationToEvent 将 room worker 产出的 client 通知翻译为 cluster 抽象事件。
func mapNotificationToEvent(roomID string, cursor string, notification roomsvc.Notification) (*clusterv1.RoomServiceStreamEventsResponse, error) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(notification.Payload, &env); err != nil {
		return nil, fmt.Errorf("unmarshal room notification: %w", err)
	}
	resp := &clusterv1.RoomServiceStreamEventsResponse{
		RoomId: roomID,
		Cursor: cursor,
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
	SnapshotRoom(context.Context, *clusterv1.SnapshotRoomRequest) (*clusterv1.SnapshotRoomResponse, error)
}

// registerRoomService 手工注册 ServiceDesc，避免命令层直接依赖生成 server 接口。
func registerRoomService(s grpc.ServiceRegistrar, srv roomService) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "cluster.v1.RoomService",
		HandlerType: (*roomService)(nil),
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

func roomSnapshotRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(clusterv1.SnapshotRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(roomService).SnapshotRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.RoomService/SnapshotRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(roomService).SnapshotRoom(ctx, req.(*clusterv1.SnapshotRoomRequest))
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
