package handler

import (
	"context"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
	"racoo.cn/lsp/internal/session"
)

// handleJoinRoom 走"加入房间 → 绑定 session 的房间字段 → Hub 注册"三步；
// 任意失败都立即把 ErrorCode 与原始错误消息回送给客户端。
func handleJoinRoom(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	payload []byte,
) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	jr := env.GetJoinRoomReq()
	if jr == nil || state.userID == "" {
		return
	}
	state.roomID = jr.RoomId
	seat, err := deps.Rooms.Join(ctx, state.roomID, state.userID)
	if err != nil {
		writeJoinRoomError(conn, env.ReqId, joinRoomErrorCode(err), err.Error())
		return
	}
	if deps.Session != nil {
		if err := deps.Session.BindRoom(ctx, state.userID, state.roomID); err != nil {
			writeJoinRoomError(conn, env.ReqId, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, err.Error())
			return
		}
	}
	deps.Hub.Register(state.userID, state.roomID, conn)
	resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_JoinRoomResp{JoinRoomResp: &clientv1.JoinRoomResponse{
		SeatIndex: int32(seat), //nolint:gosec // G115：座位号 0..3
	}}}
	b, _ := proto.Marshal(resp)
	_ = session.WriteBinary(conn, frame.Encode(msgid.JoinRoomResp, b))
}

// handleReady：仅在已进入房间时执行；命中限流/幂等时静默丢弃，避免重复推进 FSM。
func handleReady(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	msgID uint16,
	payload []byte,
) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	if state.roomID == "" || state.userID == "" {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	afterReady, err := deps.Rooms.Ready(ctx, state.roomID, state.userID)
	if err != nil {
		resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_ReadyResp{ReadyResp: &clientv1.ReadyResponse{
			ErrorCode:    clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
			ErrorMessage: err.Error(),
		}}}
		b, _ := proto.Marshal(resp)
		_ = session.WriteBinary(conn, frame.Encode(msgid.ReadyResp, b))
		return
	}
	resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_ReadyResp{ReadyResp: &clientv1.ReadyResponse{}}}
	b, _ := proto.Marshal(resp)
	_ = session.WriteBinary(conn, frame.Encode(msgid.ReadyResp, b))
	if afterReady != nil {
		afterReady()
	}
}

// handleLeaveRoom 主动离开房间：在告知服务层后清理 hub/session 与本地状态，最后回 LeaveRoomResp。
func handleLeaveRoom(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	msgID uint16,
	payload []byte,
) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	if env.GetLeaveRoomReq() == nil {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	if state.roomID == "" || state.userID == "" {
		writeLeaveRoomError(conn, env.ReqId, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, "尚未进入房间")
		return
	}
	oldRoomID := state.roomID
	after, err := deps.Rooms.Leave(ctx, state.roomID, state.userID)
	if err != nil {
		writeLeaveRoomError(conn, env.ReqId, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, err.Error())
		return
	}
	state.roomID = ""
	if deps.Hub != nil {
		deps.Hub.Unregister(state.userID, oldRoomID)
	}
	if deps.Session != nil {
		_ = deps.Session.UnbindRoom(ctx, state.userID)
	}
	resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LeaveRoomResp{LeaveRoomResp: &clientv1.LeaveRoomResponse{}}}
	b, _ := proto.Marshal(resp)
	_ = session.WriteBinary(conn, frame.Encode(msgid.LeaveRoomResp, b))
	if after != nil {
		after()
	}
}

// handleExchangeThree / handleQueMen / handleDiscard / handlePong / handleGang / handleHu
// 形成对等结构：unmarshal → 必要字段非空校验 → 限流/幂等 → 调用服务 → 响应 + after 闭包。
func handleExchangeThree(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	msgID uint16,
	payload []byte,
) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetExchangeThreeReq()
	if req == nil || state.roomID == "" || state.userID == "" {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	after, err := deps.Rooms.ExchangeThree(ctx, state.roomID, state.userID, req.GetTiles(), req.GetDirection())
	resp, after := exchangeThreeErrEnvelope(env.ReqId, after, err)
	respondAction(conn, env.ReqId, msgid.ExchangeThreeResp, resp, after)
}

func handleQueMen(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	msgID uint16,
	payload []byte,
) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetQueMenReq()
	if req == nil || state.roomID == "" || state.userID == "" {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	after, err := deps.Rooms.QueMen(ctx, state.roomID, state.userID, req.GetSuit())
	resp, after := queMenErrEnvelope(env.ReqId, after, err)
	respondAction(conn, env.ReqId, msgid.QueMenResp, resp, after)
}

func handleDiscard(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	msgID uint16,
	payload []byte,
) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetDiscardReq()
	if req == nil || state.roomID == "" || state.userID == "" {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	after, err := deps.Rooms.Discard(ctx, state.roomID, state.userID, req.GetTile())
	resp, after := discardErrEnvelope(env.ReqId, after, err)
	respondAction(conn, env.ReqId, msgid.DiscardResp, resp, after)
}

func handlePong(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	msgID uint16,
	payload []byte,
) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	if env.GetPongReq() == nil || state.roomID == "" || state.userID == "" {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	after, err := deps.Rooms.Pong(ctx, state.roomID, state.userID)
	resp, after := pongErrEnvelope(env.ReqId, after, err)
	respondAction(conn, env.ReqId, msgid.PongResp, resp, after)
}

func handleGang(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	msgID uint16,
	payload []byte,
) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetGangReq()
	if req == nil || state.roomID == "" || state.userID == "" {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	after, err := deps.Rooms.Gang(ctx, state.roomID, state.userID, req.GetTile())
	resp, after := gangErrEnvelope(env.ReqId, after, err)
	respondAction(conn, env.ReqId, msgid.GangResp, resp, after)
}

func handleHu(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	msgID uint16,
	payload []byte,
) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	if env.GetHuReq() == nil || state.roomID == "" || state.userID == "" {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	after, err := deps.Rooms.Hu(ctx, state.roomID, state.userID)
	resp, after := huErrEnvelope(env.ReqId, after, err)
	respondAction(conn, env.ReqId, msgid.HuResp, resp, after)
}

func writeJoinRoomError(conn *websocket.Conn, reqID string, code clientv1.ErrorCode, msg string) {
	resp := &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_JoinRoomResp{JoinRoomResp: &clientv1.JoinRoomResponse{
		ErrorCode:    code,
		ErrorMessage: msg,
	}}}
	b, _ := proto.Marshal(resp)
	_ = session.WriteBinary(conn, frame.Encode(msgid.JoinRoomResp, b))
}

func writeLeaveRoomError(conn *websocket.Conn, reqID string, code clientv1.ErrorCode, msg string) {
	resp := &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_LeaveRoomResp{LeaveRoomResp: &clientv1.LeaveRoomResponse{
		ErrorCode:    code,
		ErrorMessage: msg,
	}}}
	b, _ := proto.Marshal(resp)
	_ = session.WriteBinary(conn, frame.Encode(msgid.LeaveRoomResp, b))
}
