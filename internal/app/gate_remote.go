package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	clusterv1 "racoo.cn/lsp/api/gen/go/cluster/v1"
	"racoo.cn/lsp/internal/cluster/discovery"
	"racoo.cn/lsp/internal/cluster/nodeid"
	"racoo.cn/lsp/internal/cluster/router"
	"racoo.cn/lsp/internal/config"
	"racoo.cn/lsp/internal/handler"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

func withOutgoingTrace(ctx context.Context) context.Context {
	tid := logx.TraceIDFromContext(ctx)
	if tid == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "racoo-trace-id", tid)
}

// roomStreamHandle 绑定单次房间事件订阅，便于重连时取消旧流并带游标重建。
type roomStreamHandle struct {
	cancel context.CancelFunc
}

type remoteRoomGateway struct {
	lobby             clusterv1.LobbyServiceClient
	defaultRoomAddr   string
	defaultRoomClient clusterv1.RoomServiceClient
	hub               *session.Hub
	sess              *session.Manager
	routeCache        *redis.Client
	settlementStore   *postgres.SettlementStore
	router            *router.Etcd
	discovery         *discovery.Etcd

	streamCtx             context.Context
	streamMu              sync.Mutex
	roomStreams           map[string]*roomStreamHandle
	seatMu                sync.Mutex
	roomSeats             map[string]map[int32]string
	offlineSurrenderAfter time.Duration

	connMu      sync.Mutex
	roomConnMap map[string]*grpc.ClientConn
	roomClients map[string]clusterv1.RoomServiceClient
}

func newRemoteRoomGateway(cfg config.Config, hub *session.Hub, sess *session.Manager, routeCache *redis.Client, settlementStore *postgres.SettlementStore) (handler.RoomGateway, func(), error) {
	lobbyConn, err := grpc.NewClient(cfg.ClusterLobbyAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial lobby grpc: %w", err)
	}
	roomConn, err := grpc.NewClient(cfg.ClusterRoomAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = lobbyConn.Close()
		return nil, nil, fmt.Errorf("dial room grpc: %w", err)
	}
	var (
		etcdCli   *clientv3.Client
		roomRoute *router.Etcd
		roomDisc  *discovery.Etcd
	)
	if strings.TrimSpace(cfg.EtcdEndpoints) != "" {
		etcdCli, err = clientv3.New(clientv3.Config{
			Endpoints:   splitCommaSeparated(cfg.EtcdEndpoints),
			DialTimeout: 5 * time.Second,
		})
		if err != nil {
			_ = lobbyConn.Close()
			_ = roomConn.Close()
			return nil, nil, fmt.Errorf("dial etcd: %w", err)
		}
		roomRoute = router.NewEtcd(etcdCli, "/lsp")
		roomDisc = discovery.NewEtcd(etcdCli, "/lsp", 30)
	}
	streamCtx, cancel := context.WithCancel(context.Background())
	gateway := &remoteRoomGateway{
		lobby:                 clusterv1.NewLobbyServiceClient(lobbyConn),
		defaultRoomAddr:       cfg.ClusterRoomAddr,
		defaultRoomClient:     clusterv1.NewRoomServiceClient(roomConn),
		hub:                   hub,
		sess:                  sess,
		routeCache:            routeCache,
		settlementStore:       settlementStore,
		router:                roomRoute,
		discovery:             roomDisc,
		offlineSurrenderAfter: cfg.Runtime.RoomSurrenderAfterOffline,
		streamCtx:             streamCtx,
		roomStreams:           make(map[string]*roomStreamHandle),
		roomSeats:             make(map[string]map[int32]string),
		roomConnMap:           map[string]*grpc.ClientConn{cfg.ClusterRoomAddr: roomConn},
		roomClients:           map[string]clusterv1.RoomServiceClient{cfg.ClusterRoomAddr: clusterv1.NewRoomServiceClient(roomConn)},
	}
	cleanup := func() {
		cancel()
		_ = lobbyConn.Close()
		gateway.connMu.Lock()
		for addr, conn := range gateway.roomConnMap {
			if conn == nil {
				continue
			}
			_ = conn.Close()
			delete(gateway.roomConnMap, addr)
			delete(gateway.roomClients, addr)
		}
		gateway.connMu.Unlock()
		if etcdCli != nil {
			_ = etcdCli.Close()
		}
	}
	return gateway, cleanup, nil
}

// Join 通过 LobbyService 分配座位，并在首次进房时建立 room 事件订阅。
func (g *remoteRoomGateway) Join(ctx context.Context, roomID, userID string) (int, error) {
	if g == nil {
		return -1, fmt.Errorf("nil remote room gateway")
	}
	var resp *clusterv1.JoinRoomResponse
	err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = g.lobby.JoinRoom(withOutgoingTrace(callCtx), &clusterv1.JoinRoomRequest{
			RoomId: roomID,
			UserId: userID,
		})
		return callErr
	})
	if err != nil {
		return -1, err
	}
	if resp.GetError() != "" {
		return -1, errors.New(resp.GetError())
	}
	if err := g.EnsureRoomEventSubscription(ctx, roomID, ""); err != nil {
		logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), roomID)
		logx.Warn(logCtx, "首次进房订阅房间事件流失败稍后重试", "err", err.Error())
	}
	g.rememberRoomSeat(roomID, resp.GetSeatIndex(), userID)
	return int(resp.GetSeatIndex()), nil
}

func (g *remoteRoomGateway) ListRooms(ctx context.Context, pageSize int32, pageToken string) ([]*clientv1.RoomMeta, string, error) {
	if g == nil {
		return nil, "", fmt.Errorf("nil remote room gateway")
	}
	var resp *clusterv1.ListRoomsResponse
	err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = g.lobby.ListRooms(withOutgoingTrace(callCtx), &clusterv1.ListRoomsRequest{
			PageSize:  pageSize,
			PageToken: pageToken,
		})
		return callErr
	})
	if err != nil {
		return nil, "", err
	}
	if resp.GetError() != "" {
		return nil, "", errors.New(resp.GetError())
	}
	return clusterRoomMetasToClient(resp.GetRooms()), resp.GetNextPageToken(), nil
}

func (g *remoteRoomGateway) ListRules(ctx context.Context) ([]*clientv1.RuleMeta, error) {
	if g == nil {
		return nil, fmt.Errorf("nil remote room gateway")
	}
	var resp *clusterv1.ListRulesResponse
	err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = g.lobby.ListRules(withOutgoingTrace(callCtx), &clusterv1.ListRulesRequest{})
		return callErr
	})
	if err != nil {
		return nil, err
	}
	if resp.GetError() != "" {
		return nil, errors.New(resp.GetError())
	}
	return clusterRuleMetasToClient(resp.GetRules()), nil
}

func (g *remoteRoomGateway) AutoMatch(ctx context.Context, ruleID, userID string, padWithBots bool) (string, int, error) {
	if g == nil {
		return "", -1, fmt.Errorf("nil remote room gateway")
	}
	var resp *clusterv1.AutoMatchResponse
	err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = g.lobby.AutoMatch(withOutgoingTrace(callCtx), &clusterv1.AutoMatchRequest{
			RuleId:      ruleID,
			UserId:      userID,
			PadWithBots: padWithBots,
		})
		return callErr
	})
	if err != nil {
		return "", -1, err
	}
	if resp.GetError() != "" {
		return "", -1, errors.New(resp.GetError())
	}
	roomID := resp.GetRoomId()
	if err := g.EnsureRoomEventSubscription(ctx, roomID, ""); err != nil {
		logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), roomID)
		logx.Warn(logCtx, "自动匹配后订阅房间事件流失败稍后重试", "err", err.Error())
	}
	g.rememberRoomSeat(roomID, resp.GetSeatIndex(), userID)
	return roomID, int(resp.GetSeatIndex()), nil
}

func (g *remoteRoomGateway) AddBot(ctx context.Context, roomID, userID string, count int32, difficulty, opID string) ([]*clientv1.SeatInfo, func(), error) {
	if g == nil {
		return nil, nil, fmt.Errorf("nil remote room gateway")
	}
	resp, err := g.lobby.AddBot(withOutgoingTrace(ctx), &clusterv1.AddBotRequest{
		RoomId:     roomID,
		UserId:     userID,
		Count:      count,
		Difficulty: difficulty,
		OpId:       opID,
	})
	if err != nil {
		return nil, nil, err
	}
	if resp.GetError() != "" {
		return nil, nil, errors.New(resp.GetError())
	}
	added := clusterSeatsToClient(resp.GetAdded())
	after := func() {
		for _, seat := range resp.GetAdded() {
			if seat.GetUserId() == "" {
				continue
			}
			if _, err := g.Ready(context.Background(), roomID, seat.GetUserId()); err != nil {
				logCtx := logx.WithRoomID(logx.WithUserID(context.Background(), seat.GetUserId()), roomID)
				logx.Warn(logCtx, "机器人自动准备失败", "err", err.Error())
			}
			g.rememberRoomSeat(roomID, seat.GetSeatIndex(), seat.GetUserId())
		}
	}
	return added, after, nil
}

func (g *remoteRoomGateway) CreateRoom(ctx context.Context, ruleID, displayName string, private bool, userID string) (string, int, error) {
	if g == nil {
		return "", -1, fmt.Errorf("nil remote room gateway")
	}
	var resp *clusterv1.CreateRoomResponse
	err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = g.lobby.CreateRoom(withOutgoingTrace(callCtx), &clusterv1.CreateRoomRequest{
			RuleId:        ruleID,
			DisplayName:   displayName,
			Private:       private,
			CreatorUserId: userID,
		})
		return callErr
	})
	if err != nil {
		return "", -1, err
	}
	if resp.GetError() != "" {
		return "", -1, errors.New(resp.GetError())
	}
	roomID := resp.GetRoomId()
	if err := g.EnsureRoomEventSubscription(ctx, roomID, ""); err != nil {
		logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), roomID)
		logx.Warn(logCtx, "创建房间后订阅房间事件流失败稍后重试", "err", err.Error())
	}
	g.rememberRoomSeat(roomID, resp.GetSeatIndex(), userID)
	return roomID, int(resp.GetSeatIndex()), nil
}

// Ready 将准备命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Ready(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil {
		return nil, fmt.Errorf("nil remote room gateway")
	}
	if err := g.EnsureRoomEventSubscription(ctx, roomID, ""); err != nil {
		logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), roomID)
		logx.Warn(logCtx, "准备前订阅房间事件流失败稍后重试", "err", err.Error())
	}
	roomClient, _, err := g.roomClientForRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	resp, err := roomClient.ApplyEvent(withOutgoingTrace(ctx), &clusterv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &clusterv1.ApplyEventRequest_Ready{Ready: &clusterv1.ReadyEvent{}},
	})
	if err != nil {
		return nil, err
	}
	if resp.GetError() != "" {
		return nil, errors.New(resp.GetError())
	}
	if !resp.GetAccepted() {
		return nil, fmt.Errorf("room apply event rejected")
	}
	return nil, nil
}

func (g *remoteRoomGateway) Leave(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil {
		return nil, fmt.Errorf("nil remote room gateway")
	}
	if roomID == "" || userID == "" {
		return nil, fmt.Errorf("empty room_id or user_id")
	}
	lobbyResp, err := g.lobby.LeaveRoom(withOutgoingTrace(ctx), &clusterv1.LeaveRoomRequest{RoomId: roomID, UserId: userID})
	if err != nil {
		return nil, err
	}
	if lobbyResp.GetError() != "" {
		return nil, errors.New(lobbyResp.GetError())
	}
	roomClient, _, err := g.roomClientForRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	resp, err := roomClient.ApplyEvent(withOutgoingTrace(ctx), &clusterv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &clusterv1.ApplyEventRequest_Leave{Leave: &clusterv1.LeaveEvent{}},
	})
	if err != nil {
		return nil, err
	}
	if resp.GetError() != "" {
		return nil, errors.New(resp.GetError())
	}
	if !resp.GetAccepted() {
		return nil, fmt.Errorf("room apply leave rejected")
	}
	g.streamMu.Lock()
	if cur := g.roomStreams[roomID]; cur != nil {
		cur.cancel()
		delete(g.roomStreams, roomID)
	}
	g.streamMu.Unlock()
	return nil, nil
}

func (g *remoteRoomGateway) MarkSeatOffline(ctx context.Context, roomID, userID string) error {
	if g == nil || roomID == "" || userID == "" {
		return nil
	}
	delay := g.offlineSurrenderAfter
	if delay <= 0 {
		delay = 30 * time.Second
	}
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if g.hub != nil && g.hub.IsRegistered(userID, roomID) {
			return
		}
		roomClient, _, err := g.roomClientForRoom(context.Background(), roomID)
		if err != nil {
			return
		}
		_, _ = roomClient.ApplyEvent(context.Background(), &clusterv1.ApplyEventRequest{
			RoomId: roomID,
			UserId: userID,
			Body:   &clusterv1.ApplyEventRequest_Leave{Leave: &clusterv1.LeaveEvent{}},
		})
	}()
	return nil
}

// Discard 将当前轮次出牌命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Discard(ctx context.Context, roomID, userID, tile string) (func(), error) {
	return g.applyRoomEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &clusterv1.ApplyEventRequest_Discard{Discard: &clusterv1.DiscardEvent{Tile: tile}},
	})
}

// Pong 将碰牌命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Pong(ctx context.Context, roomID, userID string) (func(), error) {
	return g.applyRoomEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &clusterv1.ApplyEventRequest_Pong{Pong: &clusterv1.PongEvent{}},
	})
}

// Gang 将杠牌命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Gang(ctx context.Context, roomID, userID, tile string) (func(), error) {
	return g.applyRoomEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &clusterv1.ApplyEventRequest_Gang{Gang: &clusterv1.GangEvent{Tile: tile}},
	})
}

// Hu 将胡牌命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Hu(ctx context.Context, roomID, userID string) (func(), error) {
	return g.applyRoomEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &clusterv1.ApplyEventRequest_Hu{Hu: &clusterv1.HuEvent{}},
	})
}

func (g *remoteRoomGateway) Pass(ctx context.Context, roomID, userID string) (func(), error) {
	return g.applyRoomEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &clusterv1.ApplyEventRequest_Pass{Pass: &clusterv1.PassEvent{}},
	})
}

func (g *remoteRoomGateway) ExchangeThree(ctx context.Context, roomID, userID string, tiles []string, direction int32) (func(), error) {
	return g.applyRoomEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body: &clusterv1.ApplyEventRequest_ExchangeThree{ExchangeThree: &clusterv1.ExchangeThreeEvent{
			Tiles:     append([]string(nil), tiles...),
			Direction: direction,
		}},
	})
}

func (g *remoteRoomGateway) QueMen(ctx context.Context, roomID, userID string, suit int32) (func(), error) {
	return g.applyRoomEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &clusterv1.ApplyEventRequest_QueMen{QueMen: &clusterv1.QueMenEvent{Suit: suit}},
	})
}

func (g *remoteRoomGateway) applyRoomEvent(ctx context.Context, req *clusterv1.ApplyEventRequest) (func(), error) {
	if g == nil {
		return nil, fmt.Errorf("nil remote room gateway")
	}
	if req == nil {
		return nil, fmt.Errorf("nil apply event request")
	}
	if err := g.EnsureRoomEventSubscription(ctx, req.GetRoomId(), ""); err != nil {
		logCtx := logx.WithRoomID(logx.WithUserID(ctx, req.GetUserId()), req.GetRoomId())
		logx.Warn(logCtx, "动作前订阅房间事件流失败稍后重试", "err", err.Error())
	}
	roomClient, _, err := g.roomClientForRoom(ctx, req.GetRoomId())
	if err != nil {
		return nil, err
	}
	resp, err := roomClient.ApplyEvent(withOutgoingTrace(ctx), req)
	if err != nil {
		return nil, err
	}
	if resp.GetError() != "" {
		return nil, errors.New(resp.GetError())
	}
	if !resp.GetAccepted() {
		return nil, fmt.Errorf("room apply event rejected")
	}
	return nil, nil
}

// EnsureRoomEventSubscription 建立对房间 gRPC 事件流的订阅；sinceCursor 传给 room 用于 PG 重放。
func (g *remoteRoomGateway) EnsureRoomEventSubscription(ctx context.Context, roomID, sinceCursor string) error {
	if g == nil {
		return fmt.Errorf("nil remote room gateway")
	}
	_ = ctx
	return g.ensureRoomStream(ctx, roomID, sinceCursor)
}

func (g *remoteRoomGateway) ensureRoomStream(ctx context.Context, roomID, sinceCursor string) error {
	roomClient, _, roomErr := g.roomClientForRoom(ctx, roomID)
	if roomErr != nil {
		return roomErr
	}
	streamBase := g.streamCtx
	if tid := logx.TraceIDFromContext(ctx); tid != "" {
		streamBase = metadata.AppendToOutgoingContext(g.streamCtx, "racoo-trace-id", tid)
	}

	g.streamMu.Lock()
	cur := g.roomStreams[roomID]
	if sinceCursor == "" && cur != nil {
		g.streamMu.Unlock()
		return nil
	}
	if cur != nil {
		cur.cancel()
		delete(g.roomStreams, roomID)
	}
	subCtx, cancel := context.WithCancel(streamBase)
	handle := &roomStreamHandle{cancel: cancel}
	g.roomStreams[roomID] = handle
	g.streamMu.Unlock()

	var stream grpc.ServerStreamingClient[clusterv1.RoomServiceStreamEventsResponse]
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		stream, err = roomClient.StreamEvents(subCtx, &clusterv1.StreamEventsRequest{RoomId: roomID, SinceCursor: sinceCursor})
		if err == nil {
			break
		}
		st, ok := status.FromError(err)
		if !ok || (st.Code() != codes.Unavailable && st.Code() != codes.DeadlineExceeded) {
			break
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-subCtx.Done():
			timer.Stop()
			err = subCtx.Err()
		case <-timer.C:
		}
	}
	if err != nil {
		cancel()
		g.streamMu.Lock()
		if g.roomStreams[roomID] == handle {
			delete(g.roomStreams, roomID)
		}
		g.streamMu.Unlock()
		return fmt.Errorf("subscribe room stream: %w", err)
	}
	go g.consumeRoomStream(streamBase, roomID, stream, handle)
	return nil
}

// Resume 通过 Redis 会话与 room.SnapshotRoom 构造重连结果；不主动建立订阅（由 handler 在 Hub 注册后调用 EnsureRoomEventSubscription）。
func (g *remoteRoomGateway) Resume(ctx context.Context, sessionToken string) (*handler.ResumeResult, error) {
	if g == nil {
		return nil, fmt.Errorf("nil remote room gateway")
	}
	if g.sess == nil {
		return nil, fmt.Errorf("会话管理器未启用")
	}
	uid, srec, err := g.sess.Resume(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	if srec.RoomID == "" {
		return &handler.ResumeResult{UserID: uid, Resumed: false}, nil
	}
	roomClient, _, err := g.roomClientForRoom(ctx, srec.RoomID)
	if err != nil {
		return nil, &handler.ResumeError{Code: clientv1.ErrorCode_ERROR_CODE_RECONNECTING, Message: err.Error()}
	}
	var snapResp *clusterv1.SnapshotRoomResponse
	err = retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		snapResp, callErr = roomClient.SnapshotRoom(withOutgoingTrace(callCtx), &clusterv1.SnapshotRoomRequest{RoomId: srec.RoomID, UserId: uid})
		return callErr
	})
	if err != nil {
		if fallback, ok, ferr := g.loadSettlementFallback(ctx, uid, srec.RoomID); ferr != nil {
			return nil, ferr
		} else if ok {
			return fallback, nil
		}
		return nil, &handler.ResumeError{Code: clientv1.ErrorCode_ERROR_CODE_RECONNECTING, Message: fmt.Sprintf("快照房间失败: %v", err)}
	}
	if snapResp.GetError() != "" {
		if fallback, ok, ferr := g.loadSettlementFallback(ctx, uid, srec.RoomID); ferr != nil {
			return nil, ferr
		} else if ok {
			return fallback, nil
		}
		return nil, &handler.ResumeError{Code: clientv1.ErrorCode_ERROR_CODE_RECONNECTING, Message: snapResp.GetError()}
	}
	snap := &clientv1.SnapshotNotify{
		RoomId:           srec.RoomID,
		PlayerIds:        append([]string(nil), snapResp.GetPlayerIds()...),
		Seats:            clusterSeatsToClient(snapResp.GetSeats()),
		QueSuitBySeat:    append([]int32(nil), snapResp.GetQueSuitBySeat()...),
		Cursor:           snapResp.GetCursor(),
		State:            snapResp.GetState(),
		ActingSeat:       snapResp.GetActingSeat(),
		WaitingAction:    snapResp.GetWaitingAction(),
		PendingTile:      snapResp.GetPendingTile(),
		AvailableActions: append([]string(nil), snapResp.GetAvailableActions()...),
		ClaimCandidates:  clusterClaimCandidatesToClient(snapResp.GetClaimCandidates()),
		YourHandTiles:    append([]string(nil), snapResp.GetYourHandTiles()...),
		DiscardsBySeat:   clusterSeatTilesToClient(snapResp.GetDiscardsBySeat()),
		MeldsBySeat:      clusterSeatTilesToClient(snapResp.GetMeldsBySeat()),
		MeldInfosBySeat:  clusterSeatMeldsToClient(snapResp.GetMeldInfosBySeat()),
		LastAction:       clusterLastActionToClient(snapResp.GetLastAction()),
		WallRemaining:    snapResp.GetWallRemaining(),
		DeadlineUnixMs:   snapResp.GetDeadlineUnixMs(),
		RoundIndex:       snapResp.GetRoundIndex(),
		HandIndex:        snapResp.GetHandIndex(),
		TotalScores:      clusterSeatScoresToClient(snapResp.GetTotalScores()),
		RuleMeta:         clusterRuleMetaToClient(snapResp.GetRuleMeta()),
		Phase:            clusterPhaseToClient(snapResp.GetPhase()),
		ActingSeats:      append([]int32(nil), snapResp.GetActingSeats()...),
		LastStep:         snapResp.GetLastStep(),
	}
	if len(snapResp.GetSeats()) > 0 {
		g.rememberRoomSeatInfos(srec.RoomID, snapResp.GetSeats())
	} else {
		g.rememberRoomPlayers(srec.RoomID, snapResp.GetPlayerIds())
	}
	if snap.GetState() == "closed" {
		if fallback, ok, ferr := g.loadSettlementFallback(ctx, uid, srec.RoomID); ferr != nil {
			return nil, ferr
		} else if ok {
			return fallback, nil
		}
	}
	since := snapResp.GetCursor()
	return &handler.ResumeResult{
		UserID:              uid,
		RoomID:              srec.RoomID,
		Resumed:             true,
		Snapshot:            snap,
		SnapshotSinceCursor: since,
	}, nil
}

func (g *remoteRoomGateway) rememberRoomSeat(roomID string, seat int32, userID string) {
	if g == nil || roomID == "" || userID == "" || seat < 0 || seat > 3 {
		return
	}
	g.seatMu.Lock()
	defer g.seatMu.Unlock()
	if g.roomSeats[roomID] == nil {
		g.roomSeats[roomID] = make(map[int32]string)
	}
	g.roomSeats[roomID][seat] = userID
}

func (g *remoteRoomGateway) rememberRoomPlayers(roomID string, players []string) {
	if g == nil || roomID == "" || len(players) == 0 {
		return
	}
	g.seatMu.Lock()
	defer g.seatMu.Unlock()
	if g.roomSeats == nil {
		g.roomSeats = make(map[string]map[int32]string)
	}
	next := make(map[int32]string, 4)
	for seat, userID := range players {
		if seat >= 4 || userID == "" {
			continue
		}
		next[int32(seat)] = userID //nolint:gosec // seat 已限制在 0..3
	}
	g.roomSeats[roomID] = next
}

func (g *remoteRoomGateway) rememberRoomSeatInfos(roomID string, seats []*clusterv1.SeatInfo) {
	if g == nil || roomID == "" || len(seats) == 0 {
		return
	}
	g.seatMu.Lock()
	defer g.seatMu.Unlock()
	if g.roomSeats == nil {
		g.roomSeats = make(map[string]map[int32]string)
	}
	next := make(map[int32]string, 4)
	for _, seat := range seats {
		idx := seat.GetSeatIndex()
		userID := seat.GetUserId()
		if idx < 0 || idx > 3 || userID == "" {
			continue
		}
		next[idx] = userID
	}
	if len(next) < len(g.roomSeats[roomID]) {
		for idx, userID := range next {
			g.roomSeats[roomID][idx] = userID
		}
		return
	}
	g.roomSeats[roomID] = next
}

func (g *remoteRoomGateway) userForSeat(roomID string, seat int32) (string, bool) {
	if g == nil || seat < 0 {
		return "", false
	}
	g.seatMu.Lock()
	defer g.seatMu.Unlock()
	userID := g.roomSeats[roomID][seat]
	return userID, userID != ""
}

func clusterClaimCandidatesToClient(candidates []*clusterv1.ClaimCandidate) []*clientv1.ClaimCandidate {
	out := make([]*clientv1.ClaimCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, &clientv1.ClaimCandidate{
			SeatIndex: candidate.GetSeatIndex(),
			Actions:   append([]string(nil), candidate.GetActions()...),
		})
	}
	return out
}

func clusterSeatTilesToClient(items []*clusterv1.SeatTiles) []*clientv1.SeatTiles {
	out := make([]*clientv1.SeatTiles, 0, len(items))
	for _, item := range items {
		out = append(out, &clientv1.SeatTiles{
			SeatIndex: item.GetSeatIndex(),
			Tiles:     append([]string(nil), item.GetTiles()...),
		})
	}
	return out
}

func clusterSeatsToClient(items []*clusterv1.SeatInfo) []*clientv1.SeatInfo {
	out := make([]*clientv1.SeatInfo, 0, len(items))
	for _, item := range items {
		out = append(out, &clientv1.SeatInfo{
			SeatIndex:        item.GetSeatIndex(),
			UserId:           item.GetUserId(),
			Nickname:         item.GetNickname(),
			IsBot:            item.GetIsBot(),
			Surrendered:      item.GetSurrendered(),
			Online:           item.GetOnline(),
			AutoPlay:         item.GetAutoPlay(),
			DisconnectedAtMs: item.GetDisconnectedAtMs(),
			Status:           item.GetStatus(),
			TotalScore:       item.GetTotalScore(),
			HandCount:        item.GetHandCount(),
		})
	}
	return out
}

func clusterPhaseToClient(phase clusterv1.Phase) clientv1.Phase {
	return clientv1.Phase(phase.Number())
}

func clusterRoomMetasToClient(rooms []*clusterv1.RoomMeta) []*clientv1.RoomMeta {
	out := make([]*clientv1.RoomMeta, 0, len(rooms))
	for _, room := range rooms {
		out = append(out, &clientv1.RoomMeta{
			RoomId:      room.GetRoomId(),
			RuleId:      room.GetRuleId(),
			DisplayName: room.GetDisplayName(),
			SeatCount:   room.GetSeatCount(),
			MaxSeats:    room.GetMaxSeats(),
			CreatedAtMs: room.GetCreatedAtMs(),
			Stage:       room.GetStage(),
			RuleMeta:    clusterRuleMetaToClient(room.GetRuleMeta()),
		})
	}
	return out
}

func clusterRuleMetasToClient(rules []*clusterv1.RuleMeta) []*clientv1.RuleMeta {
	out := make([]*clientv1.RuleMeta, 0, len(rules))
	for _, rule := range rules {
		out = append(out, clusterRuleMetaToClient(rule))
	}
	return out
}

func clusterSeatScoresToClient(scores []*clusterv1.SeatScore) []*clientv1.SeatScore {
	out := make([]*clientv1.SeatScore, 0, len(scores))
	for _, score := range scores {
		out = append(out, &clientv1.SeatScore{
			SeatIndex: score.GetSeatIndex(),
			UserId:    score.GetUserId(),
			TotalFan:  score.GetTotalFan(),
			Skipped:   score.GetSkipped(),
		})
	}
	return out
}

func clusterRuleMetaToClient(meta *clusterv1.RuleMeta) *clientv1.RuleMeta {
	if meta == nil {
		return nil
	}
	return &clientv1.RuleMeta{
		RuleId:          meta.GetRuleId(),
		DisplayName:     meta.GetDisplayName(),
		ShortDesc:       meta.GetShortDesc(),
		EnabledFeatures: append([]string(nil), meta.GetEnabledFeatures()...),
		MaxHands:        meta.GetMaxHands(),
	}
}

func clusterActionDetailToClient(detail *clusterv1.ActionDetail) *clientv1.ActionDetail {
	if detail == nil {
		return nil
	}
	return &clientv1.ActionDetail{
		Step:        detail.GetStep(),
		ActorSeat:   detail.GetActorSeat(),
		Action:      detail.GetAction(),
		Tile:        detail.GetTile(),
		TargetSeat:  detail.GetTargetSeat(),
		SourceSeat:  detail.GetSourceSeat(),
		CreatedAtMs: detail.GetCreatedAtMs(),
	}
}

func clusterLastActionToClient(action *clusterv1.LastActionInfo) *clientv1.LastActionInfo {
	if action == nil {
		return nil
	}
	return &clientv1.LastActionInfo{
		Step:        action.GetStep(),
		ActorSeat:   action.GetActorSeat(),
		Action:      action.GetAction(),
		Tile:        action.GetTile(),
		TargetSeat:  action.GetTargetSeat(),
		SourceSeat:  action.GetSourceSeat(),
		CreatedAtMs: action.GetCreatedAtMs(),
	}
}

func clusterSeatMeldsToClient(items []*clusterv1.SeatMelds) []*clientv1.SeatMelds {
	out := make([]*clientv1.SeatMelds, 0, len(items))
	for _, item := range items {
		melds := make([]*clientv1.MeldInfo, 0, len(item.GetMelds()))
		for _, meld := range item.GetMelds() {
			melds = append(melds, &clientv1.MeldInfo{
				SeatIndex:       meld.GetSeatIndex(),
				Kind:            meld.GetKind(),
				Tiles:           append([]string(nil), meld.GetTiles()...),
				ClaimedFromSeat: meld.GetClaimedFromSeat(),
				Concealed:       meld.GetConcealed(),
				Step:            meld.GetStep(),
			})
		}
		out = append(out, &clientv1.SeatMelds{SeatIndex: item.GetSeatIndex(), Melds: melds})
	}
	return out
}

func clusterPenaltiesToClient(penalties []*clusterv1.PenaltyItem) []*clientv1.PenaltyItem {
	out := make([]*clientv1.PenaltyItem, 0, len(penalties))
	for _, penalty := range penalties {
		out = append(out, &clientv1.PenaltyItem{
			Reason:   penalty.GetReason(),
			FromSeat: penalty.GetFromSeat(),
			ToSeat:   penalty.GetToSeat(),
			Amount:   penalty.GetAmount(),
		})
	}
	return out
}

func clusterWinnerBreakdownsToClient(items []*clusterv1.WinnerBreakdown) []*clientv1.WinnerBreakdown {
	out := make([]*clientv1.WinnerBreakdown, 0, len(items))
	for _, item := range items {
		out = append(out, &clientv1.WinnerBreakdown{
			SeatIndex: item.GetSeatIndex(),
			UserId:    item.GetUserId(),
			Fan:       item.GetFan(),
			FanNames:  append([]string(nil), item.GetFanNames()...),
		})
	}
	return out
}

func (g *remoteRoomGateway) consumeRoomStream(streamCtx context.Context, roomID string, stream grpc.ServerStreamingClient[clusterv1.RoomServiceStreamEventsResponse], handle *roomStreamHandle) {
	defer func() {
		_ = stream.CloseSend()
		g.streamMu.Lock()
		if cur := g.roomStreams[roomID]; cur == handle {
			delete(g.roomStreams, roomID)
		}
		g.streamMu.Unlock()
	}()
	for {
		evt, err := stream.Recv()
		if err != nil {
			return
		}
		msgID, payload, err := encodeClusterRoomEvent(evt)
		if err != nil {
			logCtx := logx.WithRoomID(streamCtx, roomID)
			logx.Warn(logCtx, "房间事件转客户端推送失败", "err", err.Error())
			continue
		}
		var delivered []string
		if g.hub != nil {
			encoded := frame.Encode(msgID, payload)
			if evt.GetTargetSeat() < 0 {
				delivered = g.hub.BroadcastDeliveredUsers(roomID, encoded)
			} else if targetUserID, ok := g.userForSeat(roomID, evt.GetTargetSeat()); ok {
				if g.hub.SendToUser(targetUserID, encoded) {
					delivered = []string{targetUserID}
				}
			}
		}
		if g.sess != nil && evt.GetCursor() != "" {
			cur := evt.GetCursor()
			for _, uid := range delivered {
				_ = g.sess.UpdateCursor(streamCtx, uid, cur)
			}
		}
	}
}

func (g *remoteRoomGateway) loadSettlementFallback(ctx context.Context, userID, roomID string) (*handler.ResumeResult, bool, error) {
	if g == nil || g.settlementStore == nil || roomID == "" {
		return nil, false, nil
	}
	settlement, err := g.settlementStore.GetLatestSettlement(ctx, roomID)
	if err != nil {
		if errors.Is(err, postgres.ErrSettlementNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &handler.ResumeResult{
		UserID:     userID,
		RoomID:     roomID,
		Resumed:    false,
		Settlement: settlement,
	}, true, nil
}

func (g *remoteRoomGateway) roomClientForRoom(ctx context.Context, roomID string) (clusterv1.RoomServiceClient, string, error) {
	if g == nil {
		return nil, "", fmt.Errorf("nil remote room gateway")
	}
	addr, err := g.roomAddressForRoom(ctx, roomID)
	if err != nil {
		return nil, "", err
	}
	client, err := g.roomClientForAddr(addr)
	if err != nil {
		return nil, "", err
	}
	return client, addr, nil
}

func (g *remoteRoomGateway) roomAddressForRoom(ctx context.Context, roomID string) (string, error) {
	if g == nil {
		return "", fmt.Errorf("nil remote room gateway")
	}
	if g.router == nil || g.discovery == nil {
		if g.defaultRoomAddr == "" {
			return "", fmt.Errorf("room address unavailable")
		}
		return g.defaultRoomAddr, nil
	}
	cachedNodeID := ""
	if g.routeCache != nil {
		if rec, ok, err := g.routeCache.GetRoomRouteCache(ctx, roomID); err == nil && ok {
			cachedNodeID = rec.RoomNodeID
		}
	}
	resolvedNodeID, ok, err := g.router.ResolveRoomOwner(ctx, roomID)
	if err != nil {
		return "", err
	}
	if !ok {
		if g.routeCache != nil {
			_ = g.routeCache.DeleteRoomRouteCache(ctx, roomID)
		}
		return "", fmt.Errorf("room owner not found: %s", roomID)
	}
	nodeID := resolvedNodeID
	if g.routeCache != nil && cachedNodeID != resolvedNodeID {
		_ = g.routeCache.PutRoomRouteCache(ctx, roomID, redis.RouteRecord{RoomNodeID: resolvedNodeID}, 0)
	}
	nodeInfo, ok, err := g.discovery.ResolveNode(ctx, nodeid.KindRoom, nodeID)
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(nodeInfo.Meta.AdvertiseAddr) == "" {
		if g.defaultRoomAddr != "" && nodeID == "room-local" {
			return g.defaultRoomAddr, nil
		}
		return "", fmt.Errorf("room node not ready: %s", nodeID)
	}
	return nodeInfo.Meta.AdvertiseAddr, nil
}

func (g *remoteRoomGateway) roomClientForAddr(addr string) (clusterv1.RoomServiceClient, error) {
	g.connMu.Lock()
	defer g.connMu.Unlock()
	if client, ok := g.roomClients[addr]; ok && client != nil {
		return client, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial room grpc %s: %w", addr, err)
	}
	client := clusterv1.NewRoomServiceClient(conn)
	g.roomConnMap[addr] = conn
	g.roomClients[addr] = client
	return client, nil
}

func splitCommaSeparated(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func retryGRPC(ctx context.Context, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := fn(callCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		st, ok := status.FromError(err)
		if !ok || (st.Code() != codes.Unavailable && st.Code() != codes.DeadlineExceeded) {
			return err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func encodeClusterRoomEvent(evt *clusterv1.RoomServiceStreamEventsResponse) (uint16, []byte, error) {
	if evt == nil {
		return 0, nil, fmt.Errorf("nil room event")
	}
	switch body := evt.Body.(type) {
	case *clusterv1.RoomServiceStreamEventsResponse_InitialDeal:
		return marshalClientEnvelope(msgid.InitialDealNotify, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{
				SeatIndex: body.InitialDeal.GetSeatIndex(),
				Tiles:     append([]string(nil), body.InitialDeal.GetTiles()...),
				Step:      body.InitialDeal.GetStep(),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_StartGame:
		return marshalClientEnvelope(msgid.StartGame, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_StartGame{StartGame: &clientv1.StartGameNotify{
				RoomId:        evt.GetRoomId(),
				DealerSeat:    body.StartGame.GetDealerSeat(),
				Phase:         clusterPhaseToClient(body.StartGame.GetPhase()),
				Step:          body.StartGame.GetStep(),
				ActingSeats:   append([]int32(nil), body.StartGame.GetActingSeats()...),
				WallRemaining: body.StartGame.GetWallRemaining(),
				RoundIndex:    body.StartGame.GetRoundIndex(),
				HandIndex:     body.StartGame.GetHandIndex(),
				RuleMeta:      clusterRuleMetaToClient(body.StartGame.GetRuleMeta()),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_DrawTile:
		return marshalClientEnvelope(msgid.DrawTile, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{
				SeatIndex:      body.DrawTile.GetSeatIndex(),
				Tile:           body.DrawTile.GetTile(),
				Phase:          clusterPhaseToClient(body.DrawTile.GetPhase()),
				Step:           body.DrawTile.GetStep(),
				ActingSeats:    append([]int32(nil), body.DrawTile.GetActingSeats()...),
				WallRemaining:  body.DrawTile.GetWallRemaining(),
				DeadlineUnixMs: body.DrawTile.GetDeadlineUnixMs(),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_Action:
		return marshalClientEnvelope(msgid.ActionNotify, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{
				SeatIndex:      body.Action.GetSeatIndex(),
				Action:         body.Action.GetAction(),
				Tile:           body.Action.GetTile(),
				Phase:          clusterPhaseToClient(body.Action.GetPhase()),
				Step:           body.Action.GetStep(),
				ActingSeats:    append([]int32(nil), body.Action.GetActingSeats()...),
				Detail:         clusterActionDetailToClient(body.Action.GetDetail()),
				WallRemaining:  body.Action.GetWallRemaining(),
				DeadlineUnixMs: body.Action.GetDeadlineUnixMs(),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_Settlement:
		return marshalClientEnvelope(msgid.Settlement, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_Settlement{Settlement: &clientv1.SettlementNotify{
				RoomId:             evt.GetRoomId(),
				WinnerUserIds:      append([]string(nil), body.Settlement.GetWinnerUserIds()...),
				TotalFan:           body.Settlement.GetTotalFan(),
				SeatScores:         clusterSeatScoresToClient(body.Settlement.GetSeatScores()),
				Penalties:          clusterPenaltiesToClient(body.Settlement.GetPenalties()),
				DetailText:         body.Settlement.GetDetailText(),
				PerWinnerBreakdown: clusterWinnerBreakdownsToClient(body.Settlement.GetPerWinnerBreakdown()),
				RoundIndex:         body.Settlement.GetRoundIndex(),
				HandIndex:          body.Settlement.GetHandIndex(),
				TotalScores:        clusterSeatScoresToClient(body.Settlement.GetTotalScores()),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_ExchangeThreeDone:
		perSeat := make([]*clientv1.SeatTiles, 0, len(body.ExchangeThreeDone.GetSeatTiles()))
		for _, item := range body.ExchangeThreeDone.GetSeatTiles() {
			perSeat = append(perSeat, &clientv1.SeatTiles{
				SeatIndex: item.GetSeatIndex(),
				Tiles:     append([]string(nil), item.GetTiles()...),
			})
		}
		return marshalClientEnvelope(msgid.ExchangeThreeDone, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_ExchangeThreeDone{ExchangeThreeDone: &clientv1.ExchangeThreeDoneNotify{
				PerSeat:           perSeat,
				YourExchangedAway: append([]string(nil), body.ExchangeThreeDone.GetYourExchangedAway()...),
				Direction:         body.ExchangeThreeDone.GetDirection(),
				Phase:             clusterPhaseToClient(body.ExchangeThreeDone.GetPhase()),
				Step:              body.ExchangeThreeDone.GetStep(),
				ActingSeats:       append([]int32(nil), body.ExchangeThreeDone.GetActingSeats()...),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_QueMenDone:
		return marshalClientEnvelope(msgid.QueMenDone, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_QueMenDone{QueMenDone: &clientv1.QueMenDoneNotify{
				QueSuitBySeat: append([]int32(nil), body.QueMenDone.GetQueSuitBySeat()...),
				Phase:         clusterPhaseToClient(body.QueMenDone.GetPhase()),
				Step:          body.QueMenDone.GetStep(),
				ActingSeats:   append([]int32(nil), body.QueMenDone.GetActingSeats()...),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_RouteRedirect:
		return marshalClientEnvelope(msgid.RouteRedirectNotify, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{
				WsUrl:  body.RouteRedirect.GetWsUrl(),
				Reason: body.RouteRedirect.GetReason(),
			}},
		})
	default:
		return 0, nil, fmt.Errorf("unknown room event body")
	}
}

func marshalClientEnvelope(msgID uint16, env *clientv1.Envelope) (uint16, []byte, error) {
	payload, err := proto.Marshal(env)
	if err != nil {
		return 0, nil, err
	}
	return msgID, payload, nil
}
