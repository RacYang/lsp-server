package localadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"

	"racoo.cn/lsp/internal/contract"
	"racoo.cn/lsp/internal/protocol"
	lobbysvc "racoo.cn/lsp/internal/service/lobby"
	reconnectsvc "racoo.cn/lsp/internal/service/reconnect"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/pkg/logx"
)

// LocalRoomGateway 适配进程内房间服务，供 `cmd/all` 与本地 gate 冒烟复用。
type LocalRoomGateway struct {
	rooms                 roomsvc.RoomService
	lobby                 *lobbysvc.Service
	hub                   *session.Hub
	sess                  *session.Manager
	reconnect             *reconnectsvc.Service
	offlineSurrenderAfter time.Duration
}

// NewLocalRoomGateway 创建进程内房间网关；sess 可为 nil 表示不启用 Redis 会话。
// rooms 接收 *roomsvc.Service（构造时需注册超时回调），运行期以 RoomService 接口访问。
func NewLocalRoomGateway(rooms *roomsvc.Service, hub *session.Hub, sess *session.Manager) *LocalRoomGateway {
	g := &LocalRoomGateway{
		rooms:                 rooms,
		lobby:                 lobbysvc.New(),
		hub:                   hub,
		sess:                  sess,
		reconnect:             reconnectsvc.New(rooms, sess),
		offlineSurrenderAfter: 30 * time.Second,
	}
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
	if padWithBots {
		logx.Warn(logx.WithUserID(ctx, userID), "本地网关不支持机器人填充，padWithBots 已忽略")
	}
	rooms, _, err := g.lobby.ListRooms(ctx, 100, "")
	if err != nil {
		return "", -1, err
	}
	for _, room := range rooms {
		if ruleID != "" && room.RuleID != ruleID {
			continue
		}
		if !roomAcceptsAutoMatchLocal(g.rooms, room.RoomID) {
			continue
		}
		seat, joinErr := g.lobby.JoinRoom(ctx, room.RoomID, userID)
		if joinErr != nil {
			continue
		}
		// 再次复核：lobby.JoinRoom 与 rooms.Join 之间存在 FSM 推进的窗口，
		// 极端竞态下房间可能已 → playing，按 [L3.1] 必须显式退出。
		if !roomAcceptsAutoMatchLocal(g.rooms, room.RoomID) {
			if err := g.lobby.LeaveRoom(ctx, room.RoomID, userID); err != nil {
				logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), room.RoomID)
				logx.Warn(logCtx, "自动匹配竞态退出大厅房间失败", "err", err.Error())
			}
			continue
		}
		if _, err := g.rooms.Join(ctx, room.RoomID, userID); err != nil {
			if leaveErr := g.lobby.LeaveRoom(ctx, room.RoomID, userID); leaveErr != nil {
				logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), room.RoomID)
				logx.Warn(logCtx, "自动匹配加入失败后退出大厅房间失败", "err", leaveErr.Error())
			}
			continue
		}
		return room.RoomID, int(seat), nil
	}
	roomID, _, err := g.lobby.CreateRoomWithMeta(ctx, ruleID, "", false, userID)
	if err != nil {
		return "", -1, err
	}
	seat, err := g.rooms.Join(ctx, roomID, userID)
	if err != nil {
		if leaveErr := g.lobby.LeaveRoom(ctx, roomID, userID); leaveErr != nil {
			logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), roomID)
			logx.Warn(logCtx, "创建房间加入失败后退出大厅房间失败", "err", leaveErr.Error())
		}
		return "", -1, err
	}
	return roomID, seat, nil
}

// roomStateProbe 隔离 rooms.RoomSnapshot 依赖，便于 [L3.1] 单元测试构造五类 FSM 状态。
type roomStateProbe interface {
	RoomSnapshot(roomID string) (playerIDs []string, fsmState string, ready [4]bool, ok bool)
}

// roomAcceptsAutoMatchLocal 与 remoteRoomGateway.roomAcceptsAutoMatch 同语义：
// 仅 waiting/ready 接受自动匹配；playing/settling/closed 必须跳过。
// 房间尚未在房服层登记（RoomSnapshot 返回 ok=false）时视为"未开局可加入"。
func roomAcceptsAutoMatchLocal(rooms roomStateProbe, roomID string) bool {
	if rooms == nil {
		return false
	}
	_, state, _, ok := rooms.RoomSnapshot(roomID)
	if !ok {
		return true
	}
	switch state {
	case "", "waiting", "ready":
		return true
	default:
		return false
	}
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
		return nil, wrapActionErr(err)
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Leave(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	if g.lobby != nil {
		if err := g.lobby.LeaveRoom(ctx, roomID, userID); err != nil {
			logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), roomID)
			logx.Warn(logCtx, "离开房间时同步大厅状态失败", "err", err.Error())
		}
	}
	if err := g.rooms.Leave(ctx, roomID, userID); err != nil {
		return nil, wrapActionErr(err)
	}
	return nil, nil
}

func (g *LocalRoomGateway) MarkSeatOffline(_ context.Context, roomID, userID string) error {
	if g == nil || g.rooms == nil || roomID == "" || userID == "" {
		return nil
	}
	// 投降计时器由 Actor 持有，消除 TOCTOU 竞争；Gateway 只做信号投递。
	g.rooms.MarkSeatOffline(roomID, userID)
	return nil
}

func (g *LocalRoomGateway) CancelOfflineSurrender(_ context.Context, roomID, userID string) error {
	if g == nil || g.rooms == nil || roomID == "" || userID == "" {
		return nil
	}
	g.rooms.CancelOfflineSurrender(roomID, userID)
	return nil
}

// wrapActionErr 将 roomsvc.ErrRateLimited 转换为 contract.ErrRateLimited，
// 使 handler 无需引用 service/room 即可识别限流错误。
func wrapActionErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, roomsvc.ErrRateLimited) {
		return contract.ErrRateLimited
	}
	return err
}

// Discard 触发本地房间推进一轮，并返回在响应之后执行的广播回调。
func (g *LocalRoomGateway) Discard(ctx context.Context, roomID, userID, tile string, tok *clientv1.PhaseToken) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Discard(ctx, roomID, userID, tile, roomsvc.PhaseTokenFromProto(tok))
	if err != nil {
		return nil, wrapActionErr(err)
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Pong(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Pong(ctx, roomID, userID, roomsvc.PhaseTokenFromProto(tok))
	if err != nil {
		return nil, wrapActionErr(err)
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Chi(ctx context.Context, roomID, userID string, tiles []string, tok *clientv1.PhaseToken) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Chi(ctx, roomID, userID, tiles, roomsvc.PhaseTokenFromProto(tok))
	if err != nil {
		return nil, wrapActionErr(err)
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Gang(ctx context.Context, roomID, userID, tile string, tok *clientv1.PhaseToken) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Gang(ctx, roomID, userID, tile, roomsvc.PhaseTokenFromProto(tok))
	if err != nil {
		return nil, wrapActionErr(err)
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Hu(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Hu(ctx, roomID, userID, roomsvc.PhaseTokenFromProto(tok))
	if err != nil {
		return nil, wrapActionErr(err)
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) Pass(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.Pass(ctx, roomID, userID, roomsvc.PhaseTokenFromProto(tok))
	if err != nil {
		return nil, wrapActionErr(err)
	}
	return g.broadcastAfter(roomID, notifications), nil
}

func (g *LocalRoomGateway) OpeningAction(ctx context.Context, roomID, userID, action string, tiles []string, direction, suit int32, params map[string]string, tok *clientv1.PhaseToken) (func(), error) {
	if g == nil || g.rooms == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	notifications, err := g.rooms.OpeningAction(ctx, roomID, userID, action, tiles, direction, suit, params, roomsvc.PhaseTokenFromProto(tok))
	if err != nil {
		return nil, wrapActionErr(err)
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
	outMsgID, ok := localOutboundMsgID(notification.Kind)
	if !ok || g == nil || g.hub == nil {
		return
	}
	encoded, encErr := protocol.Encode(outMsgID, notification.Payload)
	if encErr != nil {
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

// Resume 委托重连服务完成会话校验与房间状态读取，本层仅负责 proto 投影。
func (g *LocalRoomGateway) Resume(ctx context.Context, sessionToken string) (*contract.ResumeResult, error) {
	if g == nil || g.reconnect == nil {
		return nil, fmt.Errorf("nil local room gateway")
	}
	r, err := g.reconnect.Resume(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	if !r.Resumed {
		return &contract.ResumeResult{UserID: r.UserID, Resumed: false}, nil
	}
	return &contract.ResumeResult{
		UserID:              r.UserID,
		RoomID:              r.RoomID,
		Resumed:             true,
		Snapshot:            buildSnapshotFromReconnect(r),
		SnapshotSinceCursor: r.LastCursor,
	}, nil
}

// localOutboundMsgID 映射 room 通知种类到 WebSocket 协议消息 ID。
func localOutboundMsgID(kind roomsvc.Kind) (uint16, bool) {
	switch kind {
	case roomsvc.KindInitialDeal:
		return protocol.InitialDealNotify, true
	case roomsvc.KindOpeningDone:
		return protocol.OpeningDone, true
	case roomsvc.KindStartGame:
		return protocol.StartGame, true
	case roomsvc.KindDrawTile:
		return protocol.DrawTile, true
	case roomsvc.KindAction:
		return protocol.ActionNotify, true
	case roomsvc.KindSettlement:
		return protocol.Settlement, true
	default:
		return 0, false
	}
}
