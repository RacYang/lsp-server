package remote

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"

	"racoo.cn/lsp/internal/protocol"
	"racoo.cn/lsp/pkg/logx"
)

// Join 通过 LobbyService 分配座位，并在首次进房时建立 room 事件订阅。
func (g *remoteRoomGateway) Join(ctx context.Context, roomID, userID string) (int, error) {
	if g == nil {
		return -1, fmt.Errorf("nil remote room gateway")
	}
	var resp *svcv1.JoinRoomResponse
	// JoinRoom 不携带幂等键；重试时若请求已被处理，Lobby.JoinRoom 会因同一 userID 已在房间
	// 而返回现有座位（幂等语义由 Lobby 侧 ensureRoomLocked 保证）。
	err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = g.lobby.JoinRoom(withOutgoingTrace(callCtx), &svcv1.JoinRoomRequest{
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
	resp, err := roomClient.ApplyEvent(withOutgoingTrace(ctx), &svcv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &svcv1.ApplyEventRequest_Ready{Ready: &svcv1.ReadyEvent{}},
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

// Leave 向 LobbyService 和 RoomService 发送离开事件，并取消对应的 Redis 轮询。
func (g *remoteRoomGateway) Leave(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil {
		return nil, fmt.Errorf("nil remote room gateway")
	}
	if roomID == "" || userID == "" {
		return nil, fmt.Errorf("empty room_id or user_id")
	}
	lobbyResp, err := g.lobby.LeaveRoom(withOutgoingTrace(ctx), &svcv1.LeaveRoomRequest{RoomId: roomID, UserId: userID})
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
	resp, err := roomClient.ApplyEvent(withOutgoingTrace(ctx), &svcv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &svcv1.ApplyEventRequest_Leave{Leave: &svcv1.LeaveEvent{}},
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
	g.pollMu.Lock()
	if cancel := g.pollHandles[roomID]; cancel != nil {
		cancel()
		delete(g.pollHandles, roomID)
	}
	g.pollMu.Unlock()
	return nil, nil
}

// MarkSeatOffline 延迟后若用户未重连则发送离开事件，实现离线投降。
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
		// 使用有限超时的 context，避免进程关闭时半游离 goroutine 长期阻塞。
		applyCtx, applyCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer applyCancel()
		logCtx := logx.WithRoomID(logx.WithUserID(applyCtx, userID), roomID)
		roomClient, _, err := g.roomClientForRoom(applyCtx, roomID)
		if err != nil {
			logx.Warn(logCtx, "离线投降获取 room client 失败", "err", err.Error())
			return
		}
		if _, applyErr := roomClient.ApplyEvent(applyCtx, &svcv1.ApplyEventRequest{
			RoomId: roomID,
			UserId: userID,
			Body:   &svcv1.ApplyEventRequest_Leave{Leave: &svcv1.LeaveEvent{}},
		}); applyErr != nil {
			logx.Warn(logCtx, "离线超时投降事件发送失败", "err", applyErr.Error())
		}
	}()
	return nil
}

// Discard 将当前轮次出牌命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Discard(ctx context.Context, roomID, userID, tile string, tok *clientv1.PhaseToken) (func(), error) {
	return g.applyRoomEvent(ctx, &svcv1.ApplyEventRequest{
		RoomId:     roomID,
		UserId:     userID,
		PhaseToken: tok,
		Body:       &svcv1.ApplyEventRequest_Discard{Discard: &svcv1.DiscardEvent{Tile: tile}},
	})
}

// Pong 将碰牌命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Pong(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error) {
	return g.applyRoomEvent(ctx, &svcv1.ApplyEventRequest{
		RoomId:     roomID,
		UserId:     userID,
		PhaseToken: tok,
		Body:       &svcv1.ApplyEventRequest_Pong{Pong: &svcv1.PongEvent{}},
	})
}

// Chi 将吃牌命令发给 RoomService；四川血战默认规则不会开放该动作。
func (g *remoteRoomGateway) Chi(ctx context.Context, roomID, userID string, tiles []string, tok *clientv1.PhaseToken) (func(), error) {
	return g.applyRoomEvent(ctx, &svcv1.ApplyEventRequest{
		RoomId:     roomID,
		UserId:     userID,
		PhaseToken: tok,
		Body:       &svcv1.ApplyEventRequest_Chi{Chi: &svcv1.ChiEvent{Tiles: append([]string(nil), tiles...)}},
	})
}

// Gang 将杠牌命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Gang(ctx context.Context, roomID, userID, tile string, tok *clientv1.PhaseToken) (func(), error) {
	return g.applyRoomEvent(ctx, &svcv1.ApplyEventRequest{
		RoomId:     roomID,
		UserId:     userID,
		PhaseToken: tok,
		Body:       &svcv1.ApplyEventRequest_Gang{Gang: &svcv1.GangEvent{Tile: tile}},
	})
}

// Hu 将胡牌命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Hu(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error) {
	return g.applyRoomEvent(ctx, &svcv1.ApplyEventRequest{
		RoomId:     roomID,
		UserId:     userID,
		PhaseToken: tok,
		Body:       &svcv1.ApplyEventRequest_Hu{Hu: &svcv1.HuEvent{}},
	})
}

// Pass 将过牌命令发给 RoomService。
func (g *remoteRoomGateway) Pass(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error) {
	return g.applyRoomEvent(ctx, &svcv1.ApplyEventRequest{
		RoomId:     roomID,
		UserId:     userID,
		PhaseToken: tok,
		Body:       &svcv1.ApplyEventRequest_Pass{Pass: &svcv1.PassEvent{}},
	})
}

// OpeningAction 将开局动作发给 RoomService。
func (g *remoteRoomGateway) OpeningAction(ctx context.Context, roomID, userID, action string, tiles []string, direction, suit int32, params map[string]string, tok *clientv1.PhaseToken) (func(), error) {
	return g.applyRoomEvent(ctx, &svcv1.ApplyEventRequest{
		RoomId:     roomID,
		UserId:     userID,
		PhaseToken: tok,
		Body: &svcv1.ApplyEventRequest_OpeningAction{OpeningAction: &svcv1.OpeningActionEvent{
			Action:    action,
			Tiles:     append([]string(nil), tiles...),
			Direction: direction,
			Suit:      suit,
			Params:    cloneStringMap(params),
		}},
	})
}

func (g *remoteRoomGateway) applyRoomEvent(ctx context.Context, req *svcv1.ApplyEventRequest) (func(), error) {
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

// EnsureRoomEventSubscription 启动对房间实时事件的 Redis BLPOP 轮询订阅。
// sinceCursor != "" 时会先通过 GetRoomEvents 向 hub 补齐历史帧，再启动轮询 goroutine。
// 对同一房间重复调用（sinceCursor == ""）时，若轮询已在运行则直接返回。
func (g *remoteRoomGateway) EnsureRoomEventSubscription(ctx context.Context, roomID, sinceCursor string) error {
	if g == nil {
		return fmt.Errorf("nil remote room gateway")
	}
	if g.routeCache == nil {
		return fmt.Errorf("redis 客户端未配置，无法订阅房间事件")
	}

	g.pollMu.Lock()
	_, alreadyPolling := g.pollHandles[roomID]
	if sinceCursor == "" && alreadyPolling {
		g.pollMu.Unlock()
		return nil
	}
	if alreadyPolling {
		// 重连时需重建订阅（带游标），先取消旧 goroutine
		g.pollHandles[roomID]()
		delete(g.pollHandles, roomID)
	}
	pollSubCtx, pollCancel := context.WithCancel(g.pollCtx)
	g.pollHandles[roomID] = pollCancel
	g.pollMu.Unlock()

	// 历史事件重放仅在重连时执行（sinceCursor != ""）
	if sinceCursor != "" {
		if err := g.replayHistoricalEvents(ctx, roomID, sinceCursor); err != nil {
			logCtx := logx.WithRoomID(ctx, roomID)
			logx.Warn(logCtx, "历史事件重放失败", "err", err.Error())
		}
	}

	go g.pollRoomEvents(pollSubCtx, roomID)
	return nil
}

// replayHistoricalEvents 向 room 节点查询游标之后的历史事件，并通过 hub 广播给已连接用户。
func (g *remoteRoomGateway) replayHistoricalEvents(ctx context.Context, roomID, sinceCursor string) error {
	roomClient, _, err := g.roomClientForRoom(ctx, roomID)
	if err != nil {
		return err
	}
	var resp *svcv1.GetRoomEventsResponse
	if err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = roomClient.GetRoomEvents(withOutgoingTrace(callCtx), &svcv1.GetRoomEventsRequest{
			RoomId:      roomID,
			SinceCursor: sinceCursor,
		})
		return callErr
	}); err != nil {
		return fmt.Errorf("获取历史事件: %w", err)
	}
	for _, evt := range resp.GetEvents() {
		g.forwardEventToHub(ctx, roomID, evt)
	}
	return nil
}

// pollRoomEvents 通过 Redis BLPOP 长阻塞拉取房间实时事件，并推送给已连接的 WebSocket 客户端。
// ctx 取消（房间退出或 gateway 关闭）时 goroutine 自动退出。
func (g *remoteRoomGateway) pollRoomEvents(ctx context.Context, roomID string) {
	for {
		if ctx.Err() != nil {
			return
		}
		data, err := g.routeCache.RoomEventQueuePop(ctx, roomID, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logCtx := logx.WithRoomID(ctx, roomID)
			logx.Warn(logCtx, "拉取房间事件队列失败", "err", err.Error())
			continue
		}
		if data == nil {
			continue // BLPOP 超时，检查 ctx 后继续
		}
		var evt svcv1.RoomServiceStreamEventsResponse
		if err := proto.Unmarshal(data, &evt); err != nil {
			logCtx := logx.WithRoomID(ctx, roomID)
			logx.Warn(logCtx, "反序列化房间事件失败", "err", err.Error())
			continue
		}
		g.forwardEventToHub(ctx, roomID, &evt)
	}
}

// forwardEventToHub 将房间事件编码为帧后广播（或单播）给 hub 内的连接，并更新会话游标。
func (g *remoteRoomGateway) forwardEventToHub(ctx context.Context, roomID string, evt *svcv1.RoomServiceStreamEventsResponse) {
	msgID, payload, err := encodeClusterRoomEvent(evt)
	if err != nil {
		logCtx := logx.WithRoomID(ctx, roomID)
		logx.Warn(logCtx, "房间事件编码失败", "err", err.Error())
		return
	}
	var delivered []string
	if g.hub != nil {
		encoded, encErr := protocol.Encode(msgID, payload)
		if encErr != nil {
			logx.Warn(logx.WithRoomID(ctx, roomID), "帧编码失败", "err", encErr.Error())
		} else if evt.GetTargetSeat() < 0 {
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
			if err := g.sess.UpdateCursor(ctx, uid, cur); err != nil {
				logCtx := logx.WithRoomID(logx.WithUserID(ctx, uid), roomID)
				logx.Warn(logCtx, "更新会话游标失败", "cursor", cur, "err", err.Error())
			}
		}
	}
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

func (g *remoteRoomGateway) rememberRoomSeatInfos(roomID string, seats []*clientv1.SeatInfo) {
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
