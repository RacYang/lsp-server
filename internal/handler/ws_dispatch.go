package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"

	"racoo.cn/lsp/internal/protocol"
	"racoo.cn/lsp/pkg/logx"
)

// dispatchFrame 将一个 protocol.Header 路由到对应 handler；保持原 ws.go 的 switch 行为，
// 但每个 case 只是命名调用，便于按职责定位与新增用例。
func dispatchFrame(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	r *http.Request,
	state *wsConnState,
	h protocol.Header,
) {
	switch h.MsgID {
	case protocol.LoginReq:
		handleLogin(ctx, deps, conn, r, state, h.Payload)
	case protocol.JoinRoomReq:
		handleJoinRoom(ctx, deps, conn, state, h.Payload)
	case protocol.ListRoomsReq:
		handleListRooms(ctx, deps, conn, state, h.Payload)
	case protocol.ListRulesReq:
		handleListRules(ctx, deps, conn, state, h.Payload)
	case protocol.AutoMatchReq:
		handleAutoMatch(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.CreateRoomReq:
		handleCreateRoom(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.AddBotReq:
		handleAddBot(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.ReadyReq:
		handleReady(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.HeartbeatReq:
		handleHeartbeat(deps, conn, state, h.Payload)
	case protocol.RenameReq:
		handleRename(ctx, deps, conn, state, h.Payload)
	case protocol.OpeningActionReq:
		handleOpeningAction(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.LeaveRoomReq:
		handleLeaveRoom(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.DiscardReq:
		handleDiscard(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.PongReq:
		handlePong(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.ChiReq:
		handleChi(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.GangReq:
		handleGang(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.HuReq:
		handleHu(ctx, deps, conn, state, h.MsgID, h.Payload)
	case protocol.PassReq:
		handlePass(ctx, deps, conn, state, h.MsgID, h.Payload)
	default:
		unknownMsgTotal.Inc()
		logCtx := logx.WithRoomID(logx.WithUserID(ctx, state.userID), state.roomID)
		logx.Info(logCtx, "收到尚未实现的消息编号已跳过",
			"msg_id", fmt.Sprintf("%d", h.MsgID))
	}
}
