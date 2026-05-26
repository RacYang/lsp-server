package roomadapter

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

// GRPCServer 将 room 节点事件流暴露为 v1.RoomService。
type GRPCServer struct {
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

// NewGRPCServer 构造 room gRPC 适配器，并向 room.Service 注册超时回调。
func NewGRPCServer(rooms *roomsvc.Service, ev *postgres.RoomEventStore, gs *postgres.GameSummaryStore, st *postgres.SettlementStore, rdb *redis.Client) *GRPCServer {
	srv := &GRPCServer{
		rooms:   rooms,
		ev:      ev,
		gs:      gs,
		st:      st,
		rdb:     rdb,
		streams: make(map[string][]chan *svcv1.RoomServiceStreamEventsResponse),
	}
	if rooms != nil {
		rooms.SetAutoTimeoutHandler(func(ctx context.Context, roomID string, notifications []roomsvc.Notification) {
			if err := srv.PersistPublishAndFinalize(ctx, roomID, "", notifications); err != nil {
				logx.Error(logx.WithRoomID(ctx, roomID), "超时回调持久化失败", "err", err.Error())
			}
		})
	}
	srv.ready.Store(true)
	return srv
}

// SetIdempotencyTTL 设置幂等键在 Redis 中的存活时长。
func (s *GRPCServer) SetIdempotencyTTL(ttl time.Duration) {
	if s == nil {
		return
	}
	s.idempotencyTTL = ttl
}

// SetReady 控制服务就绪标志，节点恢复时先置 false 再启动。
func (s *GRPCServer) SetReady(v bool) {
	if s == nil {
		return
	}
	s.ready.Store(v)
}

// SnapshotRoom 返回快照游标与房间摘要；无持久化时退化为内存视图。
func (s *GRPCServer) SnapshotRoom(ctx context.Context, req *svcv1.SnapshotRoomRequest) (*svcv1.SnapshotRoomResponse, error) {
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
