package remote

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	"racoo.cn/lsp/pkg/logx"
)

// ListRooms 向 LobbyService 查询房间列表。
func (g *remoteRoomGateway) ListRooms(ctx context.Context, pageSize int32, pageToken string) ([]*clientv1.RoomMeta, string, error) {
	if g == nil {
		return nil, "", fmt.Errorf("nil remote room gateway")
	}
	var resp *svcv1.ListRoomsResponse
	err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = g.lobby.ListRooms(withOutgoingTrace(callCtx), &svcv1.ListRoomsRequest{
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
	return resp.GetRooms(), resp.GetNextPageToken(), nil
}

// ListRules 向 LobbyService 查询规则列表。
func (g *remoteRoomGateway) ListRules(ctx context.Context) ([]*clientv1.RuleMeta, error) {
	if g == nil {
		return nil, fmt.Errorf("nil remote room gateway")
	}
	var resp *svcv1.ListRulesResponse
	err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = g.lobby.ListRules(withOutgoingTrace(callCtx), &svcv1.ListRulesRequest{})
		return callErr
	})
	if err != nil {
		return nil, err
	}
	if resp.GetError() != "" {
		return nil, errors.New(resp.GetError())
	}
	return resp.GetRules(), nil
}

// AutoMatch 从现有房间中找到可加入的同规则房间，找不到则自动创建。
func (g *remoteRoomGateway) AutoMatch(ctx context.Context, ruleID, userID string, padWithBots bool) (string, int, error) {
	if g == nil {
		return "", -1, fmt.Errorf("nil remote room gateway")
	}
	ruleID = strings.TrimSpace(ruleID)
	rooms, _, err := g.ListRooms(ctx, 100, "")
	if err != nil {
		return "", -1, err
	}
	for _, room := range rooms {
		if !matchRuleID(ruleID, room.GetRuleId()) {
			continue
		}
		roomID := room.GetRoomId()
		seat, joinErr := g.joinLobbyRoom(ctx, roomID, userID)
		if joinErr != nil {
			continue
		}
		ok, probeErr := g.roomAcceptsAutoMatch(ctx, roomID)
		if probeErr != nil {
			logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), roomID)
			logx.Warn(logCtx, "自动匹配校验房间状态失败按不可加入处理", "err", probeErr.Error())
		}
		if !ok || probeErr != nil {
			_ = g.leaveLobbyRoom(ctx, roomID, userID)
			continue
		}
		if err := g.EnsureRoomEventSubscription(ctx, roomID, ""); err != nil {
			logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), roomID)
			logx.Warn(logCtx, "自动匹配后订阅房间事件流失败稍后重试", "err", err.Error())
		}
		g.rememberRoomSeat(roomID, int32(seat), userID) //nolint:gosec // 座位号由 lobby 限制为 0..3。
		if err := g.joinRoomService(ctx, roomID, userID); err != nil {
			logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), roomID)
			logx.Warn(logCtx, "自动匹配后加入房间服务失败稍后重试", "err", err.Error())
		}
		return roomID, seat, nil
	}
	return g.CreateRoom(ctx, ruleID, "", false, userID)
}

func matchRuleID(want, got string) bool {
	want = strings.TrimSpace(want)
	got = strings.TrimSpace(got)
	return want == "" || got == "" || want == got
}

func (g *remoteRoomGateway) joinLobbyRoom(ctx context.Context, roomID, userID string) (int, error) {
	var resp *svcv1.JoinRoomResponse
	err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = g.lobby.JoinRoom(withOutgoingTrace(callCtx), &svcv1.JoinRoomRequest{RoomId: roomID, UserId: userID})
		return callErr
	})
	if err != nil {
		return -1, err
	}
	if resp.GetError() != "" {
		return -1, errors.New(resp.GetError())
	}
	return int(resp.GetSeatIndex()), nil
}

func (g *remoteRoomGateway) leaveLobbyRoom(ctx context.Context, roomID, userID string) error {
	resp, err := g.lobby.LeaveRoom(withOutgoingTrace(ctx), &svcv1.LeaveRoomRequest{RoomId: roomID, UserId: userID})
	if err != nil {
		return err
	}
	if resp.GetError() != "" {
		return errors.New(resp.GetError())
	}
	return nil
}

func (g *remoteRoomGateway) roomAcceptsAutoMatch(ctx context.Context, roomID string) (bool, error) {
	roomClient, _, err := g.roomClientForRoom(ctx, roomID)
	if err != nil {
		return false, err
	}
	resp, err := roomClient.SnapshotRoom(withOutgoingTrace(ctx), &svcv1.SnapshotRoomRequest{RoomId: roomID})
	if err != nil {
		return false, err
	}
	if resp.GetError() != "" {
		if strings.Contains(strings.ToLower(resp.GetError()), "room not found") {
			return true, nil
		}
		return false, errors.New(resp.GetError())
	}
	switch strings.ToLower(resp.GetState()) {
	case "", "waiting", "ready":
		return true, nil
	default:
		return false, nil
	}
}

// AddBot 向大厅服务请求添加机器人，并在回调中自动就绪。
func (g *remoteRoomGateway) AddBot(ctx context.Context, roomID, userID string, count int32, difficulty, opID string) ([]*clientv1.SeatInfo, func(), error) {
	if g == nil {
		return nil, nil, fmt.Errorf("nil remote room gateway")
	}
	resp, err := g.lobby.AddBot(withOutgoingTrace(ctx), &svcv1.AddBotRequest{
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
	added := resp.GetAdded()
	after := func() {
		// after 在请求 ctx 外执行，使用独立超时而非 context.Background()。
		botCtx, botCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer botCancel()
		for _, seat := range resp.GetAdded() {
			if seat.GetUserId() == "" {
				continue
			}
			if _, err := g.Ready(botCtx, roomID, seat.GetUserId()); err != nil {
				logCtx := logx.WithRoomID(logx.WithUserID(botCtx, seat.GetUserId()), roomID)
				logx.Warn(logCtx, "机器人自动准备失败", "err", err.Error())
			}
			g.rememberRoomSeat(roomID, seat.GetSeatIndex(), seat.GetUserId())
		}
	}
	return added, after, nil
}

// CreateRoom 创建新房间并自动订阅事件流。
func (g *remoteRoomGateway) CreateRoom(ctx context.Context, ruleID, displayName string, private bool, userID string) (string, int, error) {
	if g == nil {
		return "", -1, fmt.Errorf("nil remote room gateway")
	}
	var resp *svcv1.CreateRoomResponse
	err := retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		resp, callErr = g.lobby.CreateRoom(withOutgoingTrace(callCtx), &svcv1.CreateRoomRequest{
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
	if err := g.joinRoomService(ctx, roomID, userID); err != nil {
		logCtx := logx.WithRoomID(logx.WithUserID(ctx, userID), roomID)
		logx.Warn(logCtx, "创建房间后加入房间服务失败稍后重试", "err", err.Error())
	}
	return roomID, int(resp.GetSeatIndex()), nil
}

// joinRoomService 向 RoomService 发送 JoinEvent，仅让玩家占座，不标记就绪。
// 对应 LocalRoomGateway 中的 rooms.Join() 调用，用于在 AutoMatch/CreateRoom 时确保
// 人类玩家先于机器人 Ready 事件占住正确座位，避免大厅与房间服务座位错位。
func (g *remoteRoomGateway) joinRoomService(ctx context.Context, roomID, userID string) error {
	roomClient, _, err := g.roomClientForRoom(ctx, roomID)
	if err != nil {
		return err
	}
	resp, err := roomClient.ApplyEvent(withOutgoingTrace(ctx), &svcv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &svcv1.ApplyEventRequest_Join{Join: &svcv1.JoinEvent{}},
	})
	if err != nil {
		return err
	}
	if resp.GetError() != "" {
		return errors.New(resp.GetError())
	}
	if !resp.GetAccepted() {
		return fmt.Errorf("room join rejected")
	}
	return nil
}
