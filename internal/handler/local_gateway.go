package handler

import (
	"context"
	"fmt"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
)

// LocalRoomGateway 适配进程内房间服务，供 `cmd/all` 与本地 gate 冒烟复用。
type LocalRoomGateway struct {
	rooms *roomsvc.Service
	hub   *session.Hub
	sess  *session.Manager
}

// NewLocalRoomGateway 创建进程内房间网关；sess 可为 nil 表示不启用 Redis 会话。
func NewLocalRoomGateway(rooms *roomsvc.Service, hub *session.Hub, sess *session.Manager) *LocalRoomGateway {
	g := &LocalRoomGateway{rooms: rooms, hub: hub, sess: sess}
	if rooms != nil {
		rooms.SetAutoTimeoutHandler(func(_ context.Context, roomID string, notifications []roomsvc.Notification) {
			g.broadcastAfter(roomID, notifications)()
		})
	}
	return g
}

// Join 直接走本地房间服务加入逻辑。
func (g *LocalRoomGateway) Join(ctx context.Context, roomID, userID string) (int, error) {
	if g == nil || g.rooms == nil {
		return -1, fmt.Errorf("nil local room gateway")
	}
	return g.rooms.Join(ctx, roomID, userID)
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
	if err := g.rooms.Leave(ctx, roomID, userID); err != nil {
		return nil, err
	}
	return nil, nil
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

func (g *LocalRoomGateway) sendNotification(roomID string, notification roomsvc.Notification) {
	outMsgID, ok := outboundMsgID(notification.Kind)
	if !ok || g == nil || g.hub == nil {
		return
	}
	encoded := frame.Encode(outMsgID, notification.Payload)
	if notification.TargetSeat == roomsvc.BroadcastSeat {
		g.hub.Broadcast(roomID, encoded)
		return
	}
	players, _, ok := g.rooms.RoomSnapshot(roomID)
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
		return nil, fmt.Errorf("会话未绑定房间")
	}
	players, state, ok := g.rooms.RoomSnapshot(srec.RoomID)
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
		QueSuitBySeat:    append([]int32(nil), queSuits...),
		Cursor:           srec.LastCursor,
		State:            state,
		ActingSeat:       view.ActingSeat,
		WaitingAction:    view.WaitingAction,
		PendingTile:      view.PendingTile,
		AvailableActions: append([]string(nil), view.AvailableActions...),
		ClaimCandidates:  roomClaimCandidatesToClient(view.ClaimCandidates),
		YourHandTiles:    handForSeat(view.HandsBySeat, mySeat),
		DiscardsBySeat:   stringMatrixToClientSeatTiles(view.DiscardsBySeat),
		MeldsBySeat:      stringMatrixToClientSeatTiles(view.MeldsBySeat),
	}
	return &ResumeResult{
		UserID:              uid,
		RoomID:              srec.RoomID,
		Resumed:             true,
		Snapshot:            snap,
		SnapshotSinceCursor: srec.LastCursor,
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
