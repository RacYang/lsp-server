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
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

// roomGRPCServer 将 room 节点事件流暴露为 v1.RoomService。
type roomGRPCServer struct {
	rooms          *roomsvc.Service
	ev             *postgres.RoomEventStore
	gs             *postgres.GameSummaryStore
	st             *postgres.SettlementStore
	rdb            *redis.Client
	idempotencyTTL time.Duration

	ready   atomic.Bool
	mu      sync.Mutex
	streams map[string][]chan *svcv1.RoomServiceStreamEventsResponse
}

func newRoomGRPCServer(rooms *roomsvc.Service, ev *postgres.RoomEventStore, gs *postgres.GameSummaryStore, st *postgres.SettlementStore, rdb *redis.Client) *roomGRPCServer {
	srv := &roomGRPCServer{
		rooms:   rooms,
		ev:      ev,
		gs:      gs,
		st:      st,
		rdb:     rdb,
		streams: make(map[string][]chan *svcv1.RoomServiceStreamEventsResponse),
	}
	if rooms != nil {
		rooms.SetAutoTimeoutHandler(func(ctx context.Context, roomID string, notifications []roomsvc.Notification) {
			if err := srv.persistPublishAndFinalize(ctx, roomID, "", notifications); err != nil {
				logx.Error(logx.WithRoomID(ctx, roomID), "超时回调持久化失败", "err", err.Error())
			}
		})
	}
	srv.ready.Store(true)
	return srv
}

func (s *roomGRPCServer) setIdempotencyTTL(ttl time.Duration) {
	if s == nil {
		return
	}
	s.idempotencyTTL = ttl
}

func (s *roomGRPCServer) setReady(v bool) {
	if s == nil {
		return
	}
	s.ready.Store(v)
}

func (s *roomGRPCServer) persistPublishAndFinalize(ctx context.Context, roomID, idemKey string, notifications []roomsvc.Notification) error {
	notifications = expandPerSeatNotifications(notifications)
	events, err := s.persistNotifications(ctx, roomID, notifications)
	if err != nil {
		return err
	}
	s.markIdempotency(ctx, roomID, idemKey)
	for idx, event := range events {
		s.afterEventSideEffects(ctx, roomID, notifications[idx], event.evt, event.cursor)
		s.publish(roomID, event.evt)
	}
	return nil
}

type persistedEvent struct {
	cursor string
	evt    *svcv1.RoomServiceStreamEventsResponse
}

func (s *roomGRPCServer) persistNotifications(ctx context.Context, roomID string, notifications []roomsvc.Notification) ([]persistedEvent, error) {
	if len(notifications) == 0 {
		return nil, nil
	}
	cursors := make([]string, len(notifications))
	if s.ev == nil {
		for idx, notification := range notifications {
			cursors[idx] = fmt.Sprintf("%s-%d", notification.Kind, idx)
		}
	} else {
		rows := make([]postgres.RoomEventRow, 0, len(notifications))
		for _, notification := range notifications {
			rows = append(rows, postgres.RoomEventRow{
				Kind:       string(notification.Kind),
				Payload:    append([]byte(nil), notification.Payload...),
				TargetSeat: notification.TargetSeat.Proto(),
			})
		}
		persistedRows, err := s.ev.AppendEvents(ctx, roomID, rows)
		if err != nil {
			return nil, err
		}
		for idx, row := range persistedRows {
			cursors[idx] = fmt.Sprintf("%s:%d", roomID, row.Seq)
		}
	}

	out := make([]persistedEvent, 0, len(notifications))
	for idx, notification := range notifications {
		evt, err := mapNotificationToEvent(roomID, cursors[idx], notification)
		if err != nil {
			return nil, err
		}
		out = append(out, persistedEvent{cursor: cursors[idx], evt: evt})
	}
	return out, nil
}

func expandPerSeatNotifications(notifications []roomsvc.Notification) []roomsvc.Notification {
	out := make([]roomsvc.Notification, 0, len(notifications))
	for _, notification := range notifications {
		if notification.Privacy != roomsvc.PrivacyPerSeat || notification.Project == nil {
			out = append(out, notification)
			continue
		}
		for seat := 0; seat < 4; seat++ {
			projected := notification
			projected.TargetSeat = roomsvc.Seat(seat)
			projected.Payload = notification.Project(roomsvc.Seat(seat))
			projected.Privacy = roomsvc.PrivacyPublic
			projected.Project = nil
			out = append(out, projected)
		}
	}
	return out
}

func (s *roomGRPCServer) afterEventSideEffects(ctx context.Context, roomID string, notification roomsvc.Notification, evt *svcv1.RoomServiceStreamEventsResponse, cursor string) {
	s.persistRoomMeta(ctx, roomID, parseSinceSeq(roomID, cursor), &notification)
	if s.gs != nil {
		players, _, _, ok := s.rooms.RoomSnapshot(roomID)
		if ok && len(players) > 0 {
			if err := s.gs.CreateGameSummary(ctx, roomID, s.rooms.RuleID(), append([]string(nil), players...)); err != nil {
				logx.Error(logx.WithRoomID(ctx, roomID), "创建对局摘要失败", "err", err.Error())
			}
		}
	}
	if notification.Kind == roomsvc.KindSettlement && s.st != nil && evt.GetSettlement() != nil {
		// evt.GetSettlement() 已是 *clientv1.SettlementNotify，直接持久化。
		if err := s.st.AppendSettlement(ctx, evt.GetSettlement()); err != nil {
			logx.Error(logx.WithRoomID(ctx, roomID), "结算数据持久化失败", "err", err.Error())
		}
	}
	if notification.Kind == roomsvc.KindSettlement && s.gs != nil {
		if err := s.gs.EndGameSummary(ctx, roomID, time.Now().UTC()); err != nil {
			logx.Error(logx.WithRoomID(ctx, roomID), "结束对局摘要失败", "err", err.Error())
		}
	}
}

func (s *roomGRPCServer) persistRoomMeta(ctx context.Context, roomID string, seq int64, notification *roomsvc.Notification) {
	if s == nil || s.rdb == nil {
		return
	}
	var prev redis.RoomSnapMeta
	if meta, ok, err := s.rdb.GetRoomSnapMeta(ctx, roomID); err == nil && ok {
		prev = meta
	}
	players, state, _, ok := s.rooms.RoomSnapshot(roomID)
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
		QueSuits:  append([]int32(nil), prev.QueSuits...),
	}
	if notification != nil {
		if qs := queSuitsFromNotification(*notification); len(qs) > 0 {
			meta.QueSuits = qs
		}
	}
	if roundJSON, err := s.rooms.RoundPersistSnapshot(ctx, roomID); err == nil && len(roundJSON) > 0 {
		meta.RoundJSON = string(roundJSON)
	}
	if err := s.rdb.PutRoomSnapMeta(ctx, roomID, meta, 0); err != nil {
		logx.Warn(logx.WithRoomID(ctx, roomID), "房间快照元数据写入 Redis 失败", "err", err.Error())
	}
}

func queSuitsFromNotification(n roomsvc.Notification) []int32 {
	if n.Kind != roomsvc.KindOpeningDone {
		return nil
	}
	var env clientv1.Envelope
	if err := proto.Unmarshal(n.Payload, &env); err != nil {
		return nil
	}
	return openingSeatInts(env.GetOpeningDone(), "que_suit")
}

func openingSeatInts(done *clientv1.OpeningDoneNotify, key string) []int32 {
	if done == nil {
		return nil
	}
	for _, group := range done.GetSeatInts() {
		if group.GetKey() == key {
			return append([]int32(nil), group.GetValues()...)
		}
	}
	return nil
}

// SnapshotRoom - 返回快照游标与房间摘要；无持久化时退化为内存视图。
func (s *roomGRPCServer) SnapshotRoom(ctx context.Context, req *svcv1.SnapshotRoomRequest) (*svcv1.SnapshotRoomResponse, error) {
	if s == nil || s.rooms == nil {
		return nil, fmt.Errorf("nil room grpc server")
	}
	if !s.ready.Load() {
		return &svcv1.SnapshotRoomResponse{Error: "recovering"}, nil
	}
	roomID := req.GetRoomId()
	if roomID == "" {
		return &svcv1.SnapshotRoomResponse{Error: "empty room_id"}, nil
	}
	players, state, ready, ok := s.rooms.RoomSnapshot(roomID)
	if !ok {
		if s.rdb != nil {
			if meta, okm, _ := s.rdb.GetRoomSnapMeta(ctx, roomID); okm {
				view := roomsvc.RoundView{}
				if meta.RoundJSON != "" {
					if v, err := roomsvc.RoundViewFromPersistJSON(roomID, []byte(meta.RoundJSON)); err == nil {
						view = v
					}
				}
				cur := ""
				if s.ev != nil {
					if m, err := s.ev.MaxSeq(ctx, roomID); err == nil && m > 0 {
						cur = fmt.Sprintf("%s:%d", roomID, m)
					}
				}
				mySeat := seatIndexForUser(meta.PlayerIDs, req.GetUserId())
				seats := clientSeatsFromPlayerIDs(meta.PlayerIDs, [4]bool{}, meta.State)
				applyHandCountsToClientSeats(seats, view.HandsBySeat)
				return &svcv1.SnapshotRoomResponse{
					Cursor:           cur,
					PlayerIds:        append([]string(nil), meta.PlayerIDs...),
					Seats:            seats,
					QueSuitBySeat:    append([]int32(nil), meta.QueSuits...),
					State:            meta.State,
					ActingSeat:       view.ActingSeat,
					WaitingAction:    view.WaitingAction,
					Phase:            view.Phase,
					ActingSeats:      append([]int32(nil), view.ActingSeats...),
					LastStep:         view.LastStep,
					PendingTile:      view.PendingTile,
					AvailableActions: append([]string(nil), view.AvailableActions...),
					ClaimCandidates:  clientClaimCandidates(view.ClaimCandidates),
					YourHandTiles:    handForSeat(view.HandsBySeat, mySeat),
					DiscardsBySeat:   stringMatrixToClientSeatTiles(view.DiscardsBySeat),
					MeldsBySeat:      stringMatrixToClientSeatTiles(view.MeldsBySeat),
					MeldInfosBySeat:  view.MeldInfosBySeat,
					LastAction:       view.LastAction,
					WallRemaining:    view.WallRemaining,
					DeadlineUnixMs:   view.DeadlineUnixMs,
					PhaseUpdate:      phaseUpdateFromRoundView(view),
					RoundIndex:       view.RoundIndex,
					HandIndex:        view.HandIndex,
					TotalScores:      view.TotalScores,
					RuleMeta:         view.RuleMeta,
				}, nil
			}
		}
		return &svcv1.SnapshotRoomResponse{Error: "room not found"}, nil
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
	view, _, _ := s.rooms.RoundView(ctx, roomID)
	mySeat := seatIndexForUser(players, req.GetUserId())
	seats := clientSeatsFromPlayerIDs(players, ready, state)
	applyHandCountsToClientSeats(seats, view.HandsBySeat)
	return &svcv1.SnapshotRoomResponse{
		Cursor:           cur,
		PlayerIds:        players,
		Seats:            seats,
		QueSuitBySeat:    qs,
		State:            state,
		ActingSeat:       view.ActingSeat,
		WaitingAction:    view.WaitingAction,
		Phase:            view.Phase,
		ActingSeats:      append([]int32(nil), view.ActingSeats...),
		LastStep:         view.LastStep,
		PendingTile:      view.PendingTile,
		AvailableActions: append([]string(nil), view.AvailableActions...),
		ClaimCandidates:  clientClaimCandidates(view.ClaimCandidates),
		YourHandTiles:    handForSeat(view.HandsBySeat, mySeat),
		DiscardsBySeat:   stringMatrixToClientSeatTiles(view.DiscardsBySeat),
		MeldsBySeat:      stringMatrixToClientSeatTiles(view.MeldsBySeat),
		MeldInfosBySeat:  view.MeldInfosBySeat,
		LastAction:       view.LastAction,
		WallRemaining:    view.WallRemaining,
		DeadlineUnixMs:   view.DeadlineUnixMs,
		PhaseUpdate:      phaseUpdateFromRoundView(view),
		RoundIndex:       view.RoundIndex,
		HandIndex:        view.HandIndex,
		TotalScores:      view.TotalScores,
		RuleMeta:         view.RuleMeta,
	}, nil
}

func seatIndexForUser(players []string, userID string) int {
	for seat, current := range players {
		if current == userID {
			return seat
		}
	}
	return -1
}

func handForSeat(hands [][]string, seat int) []string {
	if seat < 0 || seat >= len(hands) {
		return nil
	}
	return append([]string(nil), hands[seat]...)
}

// phaseUpdateFromRoundView 从 RoundView 派生 PhaseUpdate；
// view.Phase 与 waitingReasonFromRoundView 均返回 clientv1 类型，无须转译。
func phaseUpdateFromRoundView(view roomsvc.RoundView) *clientv1.PhaseUpdate {
	return &clientv1.PhaseUpdate{
		Phase:            view.Phase,
		Step:             view.LastStep,
		Reason:           waitingReasonFromRoundView(view),
		DeadlineUnixMs:   view.DeadlineUnixMs,
		ServerNowUnixMs:  time.Now().UnixMilli(),
		ActingSeats:      append([]int32(nil), view.ActingSeats...),
		AvailableActions: append([]string(nil), view.AvailableActions...),
	}
}

func waitingReasonFromRoundView(view roomsvc.RoundView) clientv1.WaitingReason {
	if view.Phase == clientv1.Phase_PHASE_OPENING {
		return clientv1.WaitingReason_WAITING_REASON_OPENING
	}
	switch view.WaitingAction {
	case "claim_window":
		return clientv1.WaitingReason_WAITING_REASON_CLAIM_WINDOW
	case "tsumo_window":
		return clientv1.WaitingReason_WAITING_REASON_TSUMO
	case "discard":
		return clientv1.WaitingReason_WAITING_REASON_DISCARD
	default:
		return clientv1.WaitingReason_WAITING_REASON_NONE
	}
}

// clientSeatsFromPlayerIDs 将玩家列表映射为座位信息切片，供快照响应使用；空座位填占位结构。
func clientSeatsFromPlayerIDs(players []string, ready [4]bool, fsmState string) []*clientv1.SeatInfo {
	seats := make([]*clientv1.SeatInfo, 0, 4)
	for i := 0; i < 4; i++ {
		info := &clientv1.SeatInfo{SeatIndex: int32(i), Status: "empty"} //nolint:gosec // 固定座位范围 0..3
		if i < len(players) {
			info.UserId = players[i]
			if players[i] != "" {
				info.Online = true
				info.Status = "online"
				if ready[i] && (fsmState == "" || fsmState == "waiting" || fsmState == "ready") {
					info.Status = "ready"
				}
			}
		}
		seats = append(seats, info)
	}
	return seats
}

func applyHandCountsToClientSeats(seats []*clientv1.SeatInfo, hands [][]string) {
	for _, seat := range seats {
		idx := int(seat.GetSeatIndex())
		if idx >= 0 && idx < len(hands) {
			seat.HandCount = int32(len(hands[idx])) //nolint:gosec // hand length is bounded by Mahjong deck size.
		}
	}
}

func stringMatrixToClientSeatTiles(items [][]string) []*clientv1.SeatTiles {
	out := make([]*clientv1.SeatTiles, 0, 4)
	for seat := 0; seat < 4; seat++ {
		var tiles []string
		if seat < len(items) {
			tiles = append([]string(nil), items[seat]...)
		}
		out = append(out, &clientv1.SeatTiles{
			SeatIndex: int32(seat), //nolint:gosec // 座位范围固定
			Tiles:     tiles,
		})
	}
	return out
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
		if rows[i].Kind != string(roomsvc.KindOpeningDone) {
			continue
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(rows[i].Payload, &env); err != nil {
			return nil
		}
		if qs := openingSeatInts(env.GetOpeningDone(), "que_suit"); len(qs) > 0 {
			return qs
		}
	}
	return nil
}

// StreamEvents 先按游标从 PostgreSQL 重放，再订阅实时通道。
func (s *roomGRPCServer) StreamEvents(req *svcv1.StreamEventsRequest, stream svcv1.RoomService_StreamEventsServer) error {
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
func (s *roomGRPCServer) publish(roomID string, evt *svcv1.RoomServiceStreamEventsResponse) {
	s.mu.Lock()
	subs := append([]chan *svcv1.RoomServiceStreamEventsResponse(nil), s.streams[roomID]...)
	s.mu.Unlock()
	for _, ch := range subs {
		ch <- evt
	}
}

// removeStream 在客户端断开后回收订阅槽位。
func (s *roomGRPCServer) removeStream(roomID string, target chan *svcv1.RoomServiceStreamEventsResponse) {
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

func mapPGRowToEvent(roomID string, row postgres.RoomEventRow) (*svcv1.RoomServiceStreamEventsResponse, error) {
	n := roomsvc.Notification{Kind: roomsvc.Kind(row.Kind), Payload: append([]byte(nil), row.Payload...), TargetSeat: roomsvc.Seat(row.TargetSeat)}
	cur := fmt.Sprintf("%s:%d", roomID, row.Seq)
	return mapNotificationToEvent(roomID, cur, n)
}

// mapNotificationToEvent 将 room worker 产出的通知封装为流式事件帧；
// proto 统一后 body 字段直接使用客户端消息类型，无须字段转译。
func mapNotificationToEvent(roomID string, cursor string, notification roomsvc.Notification) (*svcv1.RoomServiceStreamEventsResponse, error) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(notification.Payload, &env); err != nil {
		return nil, fmt.Errorf("unmarshal room notification: %w", err)
	}
	resp := &svcv1.RoomServiceStreamEventsResponse{
		RoomId:     roomID,
		Cursor:     cursor,
		TargetSeat: notification.TargetSeat.Proto(),
	}
	switch notification.Kind {
	case roomsvc.KindInitialDeal:
		resp.Body = &svcv1.RoomServiceStreamEventsResponse_InitialDeal{
			InitialDeal: env.GetInitialDeal(),
		}
	case roomsvc.KindOpeningDone:
		resp.Body = &svcv1.RoomServiceStreamEventsResponse_OpeningDone{
			OpeningDone: env.GetOpeningDone(),
		}
	case roomsvc.KindStartGame:
		resp.Body = &svcv1.RoomServiceStreamEventsResponse_StartGame{
			StartGame: env.GetStartGame(),
		}
	case roomsvc.KindDrawTile:
		resp.Body = &svcv1.RoomServiceStreamEventsResponse_DrawTile{
			DrawTile: env.GetDrawTile(),
		}
	case roomsvc.KindAction:
		resp.Body = &svcv1.RoomServiceStreamEventsResponse_Action{
			Action: env.GetAction(),
		}
	case roomsvc.KindSettlement:
		resp.Body = &svcv1.RoomServiceStreamEventsResponse_Settlement{
			Settlement: env.GetSettlement(),
		}
	default:
		return nil, fmt.Errorf("unsupported notification kind: %s", notification.Kind)
	}
	return resp, nil
}

// clientClaimCandidates 把当前回合可碰/杠/胡的候选列表转换为协议格式。
func clientClaimCandidates(candidates []roomsvc.RoundClaimCandidate) []*clientv1.ClaimCandidate {
	out := make([]*clientv1.ClaimCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, &clientv1.ClaimCandidate{
			SeatIndex: candidate.Seat,
			Actions:   append([]string(nil), candidate.Actions...),
		})
	}
	return out
}

type roomService interface {
	ApplyEvent(context.Context, *svcv1.ApplyEventRequest) (*svcv1.ApplyEventResponse, error)
	StreamEvents(*svcv1.StreamEventsRequest, grpc.ServerStreamingServer[svcv1.RoomServiceStreamEventsResponse]) error
	SnapshotRoom(context.Context, *svcv1.SnapshotRoomRequest) (*svcv1.SnapshotRoomResponse, error)
}

// registerRoomService 手工注册 ServiceDesc，避免命令层直接依赖生成 server 接口。
func registerRoomService(s grpc.ServiceRegistrar, srv roomService) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "v1.RoomService",
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
	in := new(svcv1.ApplyEventRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(roomService).ApplyEvent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.RoomService/ApplyEvent"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(roomService).ApplyEvent(ctx, req.(*svcv1.ApplyEventRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func roomSnapshotRoomHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(svcv1.SnapshotRoomRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(roomService).SnapshotRoom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v1.RoomService/SnapshotRoom"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(roomService).SnapshotRoom(ctx, req.(*svcv1.SnapshotRoomRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// roomStreamEventsHandler 为服务端流式订阅建立请求与 stream 桥接。
func roomStreamEventsHandler(srv interface{}, stream grpc.ServerStream) error {
	in := new(svcv1.StreamEventsRequest)
	if err := stream.RecvMsg(in); err != nil {
		return err
	}
	return srv.(roomService).StreamEvents(in, &grpc.GenericServerStream[svcv1.StreamEventsRequest, svcv1.RoomServiceStreamEventsResponse]{ServerStream: stream})
}
