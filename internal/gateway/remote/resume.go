package remote

import (
	"context"
	"errors"
	"fmt"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	"racoo.cn/lsp/internal/handler"
	"racoo.cn/lsp/internal/store/postgres"
)

// Resume 通过 Redis 会话与 room.SnapshotRoom 构造重连结果；
// 不主动建立订阅（由 handler 在 Hub 注册后调用 EnsureRoomEventSubscription）。
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
	var snapResp *svcv1.SnapshotRoomResponse
	err = retryGRPC(ctx, func(callCtx context.Context) error {
		var callErr error
		snapResp, callErr = roomClient.SnapshotRoom(withOutgoingTrace(callCtx), &svcv1.SnapshotRoomRequest{RoomId: srec.RoomID, UserId: uid})
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
	// SnapshotRoomResponse 所有字段已与 client.v1 类型统一，无须转译。
	snap := &clientv1.SnapshotNotify{
		RoomId:           srec.RoomID,
		PlayerIds:        append([]string(nil), snapResp.GetPlayerIds()...),
		Seats:            snapResp.GetSeats(),
		QueSuitBySeat:    append([]int32(nil), snapResp.GetQueSuitBySeat()...),
		Cursor:           snapResp.GetCursor(),
		State:            snapResp.GetState(),
		ActingSeat:       snapResp.GetActingSeat(),
		WaitingAction:    snapResp.GetWaitingAction(),
		PendingTile:      snapResp.GetPendingTile(),
		AvailableActions: append([]string(nil), snapResp.GetAvailableActions()...),
		ClaimCandidates:  snapResp.GetClaimCandidates(),
		YourHandTiles:    append([]string(nil), snapResp.GetYourHandTiles()...),
		DiscardsBySeat:   snapResp.GetDiscardsBySeat(),
		MeldsBySeat:      snapResp.GetMeldsBySeat(),
		MeldInfosBySeat:  snapResp.GetMeldInfosBySeat(),
		LastAction:       snapResp.GetLastAction(),
		WallRemaining:    snapResp.GetWallRemaining(),
		DeadlineUnixMs:   snapResp.GetDeadlineUnixMs(),
		RoundIndex:       snapResp.GetRoundIndex(),
		HandIndex:        snapResp.GetHandIndex(),
		TotalScores:      snapResp.GetTotalScores(),
		RuleMeta:         snapResp.GetRuleMeta(),
		Phase:            snapResp.GetPhase(),
		ActingSeats:      append([]int32(nil), snapResp.GetActingSeats()...),
		LastStep:         snapResp.GetLastStep(),
		PhaseUpdate:      snapResp.GetPhaseUpdate(),
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
