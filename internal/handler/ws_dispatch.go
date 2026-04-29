package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"

	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
	"racoo.cn/lsp/pkg/logx"
)

// dispatchFrame 将一个 frame.Header 路由到对应 handler；保持原 ws.go 的 switch 行为，
// 但每个 case 只是命名调用，便于按职责定位与新增用例。
func dispatchFrame(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	r *http.Request,
	state *wsConnState,
	h frame.Header,
) {
	switch h.MsgID {
	case msgid.LoginReq:
		handleLogin(ctx, deps, conn, r, state, h.Payload)
	case msgid.JoinRoomReq:
		handleJoinRoom(ctx, deps, conn, state, h.Payload)
	case msgid.ListRoomsReq:
		handleListRooms(ctx, deps, conn, state, h.Payload)
	case msgid.AutoMatchReq:
		handleAutoMatch(ctx, deps, conn, state, h.MsgID, h.Payload)
	case msgid.CreateRoomReq:
		handleCreateRoom(ctx, deps, conn, state, h.MsgID, h.Payload)
	case msgid.ReadyReq:
		handleReady(ctx, deps, conn, state, h.MsgID, h.Payload)
	case msgid.HeartbeatReq:
		handleHeartbeat(deps, conn, state, h.Payload)
	case msgid.ExchangeThreeReq:
		handleExchangeThree(ctx, deps, conn, state, h.MsgID, h.Payload)
	case msgid.QueMenReq:
		handleQueMen(ctx, deps, conn, state, h.MsgID, h.Payload)
	case msgid.LeaveRoomReq:
		handleLeaveRoom(ctx, deps, conn, state, h.MsgID, h.Payload)
	case msgid.DiscardReq:
		handleDiscard(ctx, deps, conn, state, h.MsgID, h.Payload)
	case msgid.PongReq:
		handlePong(ctx, deps, conn, state, h.MsgID, h.Payload)
	case msgid.GangReq:
		handleGang(ctx, deps, conn, state, h.MsgID, h.Payload)
	case msgid.HuReq:
		handleHu(ctx, deps, conn, state, h.MsgID, h.Payload)
	default:
		unknownMsgTotal.Inc()
		logCtx := logx.WithRoomID(logx.WithUserID(ctx, state.userID), state.roomID)
		logx.Info(logCtx, "收到尚未实现的消息编号已跳过",
			"msg_id", fmt.Sprintf("%d", h.MsgID))
	}
}
