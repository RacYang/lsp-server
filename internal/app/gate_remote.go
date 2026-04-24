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
	lobby                    clusterv1.LobbyServiceClient
	defaultRoomAddr          string
	defaultRoomClient        clusterv1.RoomServiceClient
	hub                      *session.Hub
	sess                     *session.Manager
	routeCache               *redis.Client
	settlementStore          *postgres.SettlementStore
	router                   *router.Etcd
	discovery                *discovery.Etcd
	currentGateAdvertiseAddr string

	streamCtx   context.Context
	streamMu    sync.Mutex
	roomStreams map[string]*roomStreamHandle

	connMu      sync.Mutex
	roomConnMap map[string]*grpc.ClientConn
	roomClients map[string]clusterv1.RoomServiceClient
}

func newRemoteRoomGateway(cfg config.Config, hub *session.Hub, sess *session.Manager, routeCache *redis.Client, settlementStore *postgres.SettlementStore, currentGateAdvertiseAddr string) (handler.RoomGateway, func(), error) {
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
		lobby:                    clusterv1.NewLobbyServiceClient(lobbyConn),
		defaultRoomAddr:          cfg.ClusterRoomAddr,
		defaultRoomClient:        clusterv1.NewRoomServiceClient(roomConn),
		hub:                      hub,
		sess:                     sess,
		routeCache:               routeCache,
		settlementStore:          settlementStore,
		router:                   roomRoute,
		discovery:                roomDisc,
		currentGateAdvertiseAddr: currentGateAdvertiseAddr,
		streamCtx:                streamCtx,
		roomStreams:              make(map[string]*roomStreamHandle),
		roomConnMap:              map[string]*grpc.ClientConn{cfg.ClusterRoomAddr: roomConn},
		roomClients:              map[string]clusterv1.RoomServiceClient{cfg.ClusterRoomAddr: clusterv1.NewRoomServiceClient(roomConn)},
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
		logx.Warn(ctx, "首次进房订阅房间事件流失败稍后重试", "trace_id", logx.TraceIDFromContext(ctx), "user_id", userID, "room_id", roomID, "err", err.Error())
	}
	return int(resp.GetSeatIndex()), nil
}

// Ready 将准备命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Ready(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil {
		return nil, fmt.Errorf("nil remote room gateway")
	}
	if err := g.EnsureRoomEventSubscription(ctx, roomID, ""); err != nil {
		logx.Warn(ctx, "准备前订阅房间事件流失败稍后重试", "trace_id", logx.TraceIDFromContext(ctx), "user_id", userID, "room_id", roomID, "err", err.Error())
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
	go g.consumeRoomStream(roomID, stream, handle)
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
	if srec.AdvertiseAddr != "" && g.currentGateAdvertiseAddr != "" && srec.AdvertiseAddr != g.currentGateAdvertiseAddr {
		return &handler.ResumeResult{
			UserID:   uid,
			RoomID:   srec.RoomID,
			Resumed:  false,
			Redirect: &clientv1.RouteRedirectNotify{WsUrl: "ws://" + srec.AdvertiseAddr + "/ws", Reason: "会话绑定在其他网关节点"},
		}, nil
	}
	if srec.RoomID == "" {
		return nil, fmt.Errorf("会话未绑定房间")
	}
	roomClient, _, err := g.roomClientForRoom(ctx, srec.RoomID)
	if err != nil {
		return nil, &handler.ResumeError{Code: clientv1.ErrorCode_ERROR_CODE_RECONNECTING, Message: err.Error()}
	}
	var snapResp *clusterv1.SnapshotRoomResponse
	err = retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		snapResp, callErr = roomClient.SnapshotRoom(withOutgoingTrace(callCtx), &clusterv1.SnapshotRoomRequest{RoomId: srec.RoomID})
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
		RoomId:        srec.RoomID,
		PlayerIds:     append([]string(nil), snapResp.GetPlayerIds()...),
		QueSuitBySeat: append([]int32(nil), snapResp.GetQueSuitBySeat()...),
		Cursor:        snapResp.GetCursor(),
		State:         snapResp.GetState(),
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

func (g *remoteRoomGateway) consumeRoomStream(roomID string, stream grpc.ServerStreamingClient[clusterv1.RoomServiceStreamEventsResponse], handle *roomStreamHandle) {
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
			logx.Warn(context.Background(), "房间事件转客户端推送失败", "trace_id", "", "user_id", "", "room_id", roomID, "err", err.Error())
			continue
		}
		if g.hub != nil {
			g.hub.Broadcast(roomID, msgID, payload)
		}
		if g.sess != nil && g.hub != nil && evt.GetCursor() != "" {
			cur := evt.GetCursor()
			g.hub.IterRoomUsers(roomID, func(uid string) {
				_ = g.sess.UpdateCursor(context.Background(), uid, cur)
			})
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
	case *clusterv1.RoomServiceStreamEventsResponse_StartGame:
		return marshalClientEnvelope(msgid.StartGame, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_StartGame{StartGame: &clientv1.StartGameNotify{
				RoomId:     evt.GetRoomId(),
				DealerSeat: body.StartGame.GetDealerSeat(),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_DrawTile:
		return marshalClientEnvelope(msgid.DrawTile, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{
				SeatIndex: body.DrawTile.GetSeatIndex(),
				Tile:      body.DrawTile.GetTile(),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_Action:
		return marshalClientEnvelope(msgid.ActionNotify, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{
				SeatIndex: body.Action.GetSeatIndex(),
				Action:    body.Action.GetAction(),
				Tile:      body.Action.GetTile(),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_Settlement:
		return marshalClientEnvelope(msgid.Settlement, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_Settlement{Settlement: &clientv1.SettlementNotify{
				RoomId:        evt.GetRoomId(),
				WinnerUserIds: append([]string(nil), body.Settlement.GetWinnerUserIds()...),
				TotalFan:      body.Settlement.GetTotalFan(),
				DetailText:    body.Settlement.GetDetailText(),
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
				PerSeat: perSeat,
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_QueMenDone:
		return marshalClientEnvelope(msgid.QueMenDone, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_QueMenDone{QueMenDone: &clientv1.QueMenDoneNotify{
				QueSuitBySeat: append([]int32(nil), body.QueMenDone.GetQueSuitBySeat()...),
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
