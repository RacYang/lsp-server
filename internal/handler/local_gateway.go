package handler

import (
	"context"
	"fmt"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	lobbysvc "racoo.cn/lsp/internal/service/lobby"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
)

// LocalRoomGateway 适配进程内房间服务，供 `cmd/all` 与本地 gate 冒烟复用。
type LocalRoomGateway struct {
	rooms                 *roomsvc.Service
	lobby                 *lobbysvc.Service
	hub                   *session.Hub
	sess                  *session.Manager
	offlineSurrenderAfter time.Duration
}

// NewLocalRoomGateway 创建进程内房间网关；sess 可为 nil 表示不启用 Redis 会话。
func NewLocalRoomGateway(rooms *roomsvc.Service, hub *session.Hub, sess *session.Manager) *LocalRoomGateway {
	g := &LocalRoomGateway{rooms: rooms, lobby: lobbysvc.New(), hub: hub, sess: sess, offlineSurrenderAfter: 30 * time.Second}
	if rooms != nil {
		rooms.SetAutoTimeoutHandler(func(_ context.Context, roomID string, notifications []roomsvc.Notification) {
			g.broadcastAfter(roomID, notifications)()
		})
	}
	return g
}

func (g *LocalRoomGateway) SetOfflineSurrenderAfter(d time.Duration) {
	if g == nil || d <= 0 {
		return
	}
	g.offlineSurrenderAfter = d
}

// Join 直接走本地房间服务加入逻辑。
func (g *LocalRoomGateway) Join(ctx context.Context, roomID, userID string) (int, error) {
	if g == nil || g.rooms == nil {
		return -1, fmt.Errorf("nil local room gateway")
	}
	if g.lobby != nil {
		if _, err := g.lobby.JoinRoom(ctx, roomID, userID); err != nil {
			return -1, err
		}
	}
	return g.rooms.Join(ctx, roomID, userID)
}

func (g *LocalRoomGateway) ListRooms(ctx context.Context, pageSize int32, pageToken string) ([]*clientv1.RoomMeta, string, error) {
	if g == nil || g.lobby == nil {
		return nil, "", fmt.Errorf("nil local lobby gateway")
	}
	rooms, next, err := g.lobby.ListRooms(ctx, pageSize, pageToken)
	if err != nil {
		return nil, "", err
	}
	return lobbyRoomMetasToClient(rooms), next, nil
}

func (g *LocalRoomGateway) ListRules(ctx context.Context) ([]*clientv1.RuleMeta, error) {
	if g == nil || g.lobby == nil {
		return nil, fmt.Errorf("nil local lobby gateway")
	}
	rules, err := g.lobby.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	return lobbyRuleMetasToClient(rules), nil
}

func (g *LocalRoomGateway) AutoMatch(ctx context.Context, ruleID, userID string, padWithBots bool) (string, int, error) {
	if g == nil || g.lobby == nil || g.rooms == nil {
		return "", -1, fmt.Errorf("nil local lobby gateway")
	}
	roomID, _, err := g.lobby.AutoMatch(ctx, ruleID, userID)
	if err != nil {
		return "", -1, err
	}
	seat, err := g.rooms.Join(ctx, roomID, userID)
	if err != nil {
		return "", -1, err
	}
	_ = padWithBots
	return roomID, seat, nil
}

func (g *LocalRoomGateway) CreateRoom(ctx context.Context, ruleID, displayName string, private bool, userID string) (string, int, error) {
	if g == nil || g.lobby == nil || g.rooms == nil {
		return "", -1, fmt.Errorf("nil local lobby gateway")
	}
	roomID, _, err := g.lobby.CreateRoomWithMeta(ctx, ruleID, displayName, private, userID)
	if err != nil {
		return "", -1, err
	}
	seat, err := g.rooms.Join(ctx, roomID, userID)
	if err != nil {
		return "", -1, err
	}
	return roomID, seat, nil
}

func (g *LocalRoomGateway) AddBot(ctx context.Context, roomID, userID string, count int32, _ string, _ string) ([]*clientv1.SeatInfo, func(), error) {
	if g == nil || g.lobby == nil || g.rooms == nil {
		return nil, nil, fmt.Errorf("nil local lobby gateway")
	}
	added, err := g.lobby.AddBot(ctx, roomID, count, 3)
	if err != nil {
		return nil, nil, err
	}
	out := make([]*clientv1.SeatInfo, 0, len(added))
	var notifications []roomsvc.Notification
	for _, bot := range added {
		if _, err := g.rooms.Join(ctx, roomID, bot.UserID); err != nil {
			return nil, nil, err
		}
		readyNotifications, err := g.rooms.Ready(ctx, roomID, bot.UserID)
		if err != nil {
			return nil, nil, err
		}
		notifications = append(notifications, readyNotifications...)
		out = append(out, &clientv1.SeatInfo{
			SeatIndex: bot.SeatIndex,
			UserId:    bot.UserID,
			Nickname:  "机器人",
			IsBot:     true,
			Online:    true,
			AutoPlay:  true,
			Status:    "ready",
		})
	}
	return out, g.broadcastAfter(roomID, notifications), nil
}

// Ready 触发本地 worker，并返回一个在 ReadyResp 之后执行的广播回调。
func (g *LocalRoomGateway) Ready(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Ready(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Leave(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	if g.lobby != nil {
		_ = g.lobby.LeaveRoom(ctx, roomID, userID)
	}
	if err := g.rooms.Leave(ctx, roomID, userID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (g *LocalRoomGateway) MarkSeatOffline(ctx context.Context, roomID, userID string) error {
	if g == nil || g.rooms == nil || roomID == "" || userID == "" {
		return nil
	}
	delay := g.offlineSurrenderAfter
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if g.hub != nil && g.hub.IsRegistered(userID, roomID) {
				return
			}
			_ = g.rooms.Leave(context.Background(), roomID, userID)
		}
	}()
	return nil
}

// Discard 触发本地房间推进一轮，并返回在响应之后执行的广播回调。
func (g *LocalRoomGateway) Discard(ctx context.Context, roomID, userID, tile string) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Discard(ctx, roomID, userID, tile)
	if err != nil {
		return nil, err
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Pong(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Pong(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Gang(ctx context.Context, roomID, userID, tile string) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Gang(ctx, roomID, userID, tile)
	if err != nil {
		return nil, err
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Hu(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Hu(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Pass(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Pass(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) ExchangeThree(ctx context.Context, roomID, userID string, tiles []string, direction int32) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.ExchangeThree(ctx, roomID, userID, tiles, direction)
	if err != nil {
		return nil, err
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) QueMen(ctx context.Context, roomID, userID string, suit int32) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.QueMen(ctx, roomID, userID, suit)
	if err != nil {
		return nil, err
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) broadcastAfter(roomID string, notifications []roomsvc.Notification) func() {
	return func() {
		for _, notification := range notifications {
			g.sendNotification(roomID, notification)
		}
	}
}

// BroadcastNotifications 将 room.Service 返回的权威通知推送给房间内客户端。
//
// 常规玩家动作通过各 handler 在响应后调用 broadcastAfter；进程内 bot supervisor 不经过
// WebSocket handler，因此需要这个显式入口复用同一套隐私投影与座位定向逻辑。
func (g *LocalRoomGateway) BroadcastNotifications(_ context.Context, roomID string, notifications []roomsvc.Notification) {
	if g == nil || roomID == "" || len(notifications) == 0 {
		return
	}
	g.broadcastAfter(roomID, notifications)()
}

func (g *LocalRoomGateway) sendNotification(roomID string, notification roomsvc.Notification) {
	outMsgID, ok := outboundMsgID(notification.Kind)
	if !ok || g == nil || g.hub == nil {
		return
	}
	encoded := frame.Encode(outMsgID, notification.Payload)
	if notification.Privacy == roomsvc.PrivacyPerSeat && notification.Project != nil {
		players, _, _, ok := g.rooms.RoomSnapshot(roomID)
		if !ok {
			return
		}
		for seat := 0; seat < len(players) && seat < 4; seat++ {
			projected := notification.Project(roomsvc.Seat(seat))
			if len(projected) == 0 {
				continue
			}
			g.hub.SendToUser(players[seat], frame.Encode(outMsgID, projected))
		}
		return
	}
	if notification.TargetSeat == roomsvc.BroadcastSeat {
		g.hub.Broadcast(roomID, encoded)
		return
	}
	players, _, _, ok := g.rooms.RoomSnapshot(roomID)
	targetSeat := int(notification.TargetSeat)
	if !ok || targetSeat >= len(players) || targetSeat < 0 {
		return
	}
	g.hub.SendToUser(players[targetSeat], encoded)
}

// EnsureRoomEventSubscription 本地进程内无 gRPC 事件流，由 Hub 广播承担。
func (g *LocalRoomGateway) EnsureRoomEventSubscription(_ context.Context, _, _ string) error {
	return nil
}

// Resume 基于 Redis 会话与内存房间视图构造快照；无持久化游标时以会话 LastCursor 为准。
func (g *LocalRoomGateway) Resume(ctx context.Context, sessionToken string) (*ResumeResult, error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	if g.sess == nil {
		return nil, fmt.Errorf("会话管理器未启用")
	}
	uid, srec, err := g.sess.Resume(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	if srec.RoomID == "" {
		return &ResumeResult{UserID: uid, Resumed: false}, nil
	}
	players, state, _, ok := g.rooms.RoomSnapshot(srec.RoomID)
	if !ok {
		return nil, fmt.Errorf("房间不存在或已回收")
	}
	view, _, _ := g.rooms.RoundView(ctx, srec.RoomID)
	mySeat := seatIndexForUser(players, uid)
	var queSuits []int32
	if roundJSON, err := g.rooms.RoundPersistSnapshot(ctx, srec.RoomID); err == nil && len(roundJSON) > 0 {
		queSuits, _ = roomsvc.QueSuitsFromPersistJSON(roundJSON)
	}
	snap := &clientv1.SnapshotNotify{
		RoomId:           srec.RoomID,
		PlayerIds:        append([]string(nil), players...),
		Seats:            clientSeatsFromPlayerIDs(players),
		QueSuitBySeat:    append([]int32(nil), queSuits...),
		Cursor:           srec.LastCursor,
		State:            state,
		ActingSeat:       view.ActingSeat,
		ActingSeats:      append([]int32(nil), view.ActingSeats...),
		WaitingAction:    view.WaitingAction,
		Phase:            view.Phase,
		LastStep:         view.LastStep,
		PendingTile:      view.PendingTile,
		AvailableActions: append([]string(nil), view.AvailableActions...),
		ClaimCandidates:  roomClaimCandidatesToClient(view.ClaimCandidates),
		YourHandTiles:    handForSeat(view.HandsBySeat, mySeat),
		DiscardsBySeat:   stringMatrixToClientSeatTiles(view.DiscardsBySeat),
		MeldsBySeat:      stringMatrixToClientSeatTiles(view.MeldsBySeat),
		MeldInfosBySeat:  view.MeldInfosBySeat,
		LastAction:       view.LastAction,
		WallRemaining:    view.WallRemaining,
		DeadlineUnixMs:   view.DeadlineUnixMs,
		RoundIndex:       view.RoundIndex,
		HandIndex:        view.HandIndex,
		TotalScores:      view.TotalScores,
		RuleMeta:         view.RuleMeta,
	}
	for seat := 0; seat < len(snap.Seats) && seat < len(view.HandsBySeat); seat++ {
		snap.Seats[seat].HandCount = int32(len(view.HandsBySeat[seat])) //nolint:gosec // 座位手牌数量小于 20。
	}
	return &ResumeResult{
		UserID:              uid,
		RoomID:              srec.RoomID,
		Resumed:             true,
		Snapshot:            snap,
		SnapshotSinceCursor: srec.LastCursor,
	}, nil
}

func clientSeatsFromPlayerIDs(players []string) []*clientv1.SeatInfo {
	seats := make([]*clientv1.SeatInfo, 0, 4)
	for i := 0; i < 4; i++ {
		info := &clientv1.SeatInfo{SeatIndex: int32(i), Status: "empty"} //nolint:gosec // 固定座位范围 0..3
		if i < len(players) {
			info.UserId = players[i]
			if players[i] != "" {
				info.Online = true
				info.Status = "online"
			}
		}
		seats = append(seats, info)
	}
	return seats
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

func roomClaimCandidatesToClient(candidates []roomsvc.RoundClaimCandidate) []*clientv1.ClaimCandidate {
	out := make([]*clientv1.ClaimCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, &clientv1.ClaimCandidate{
			SeatIndex: candidate.Seat,
			Actions:   append([]string(nil), candidate.Actions...),
		})
	}
	return out
}

func lobbyRoomMetasToClient(rooms []lobbysvc.RoomMeta) []*clientv1.RoomMeta {
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
			RuleMeta:    lobbyRuleMetaToClient(room.RuleMeta),
		})
	}
	return out
}

func lobbyRuleMetasToClient(rules []lobbysvc.RuleMeta) []*clientv1.RuleMeta {
	out := make([]*clientv1.RuleMeta, 0, len(rules))
	for _, rule := range rules {
		out = append(out, lobbyRuleMetaToClient(rule))
	}
	return out
}

func lobbyRuleMetaToClient(meta lobbysvc.RuleMeta) *clientv1.RuleMeta {
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
