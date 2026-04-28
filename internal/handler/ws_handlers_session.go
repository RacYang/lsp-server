package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	appmetrics "racoo.cn/lsp/internal/metrics"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/pkg/logx"
)

// handleLogin 处理 LoginReq：带 session_token 走重连路径，否则签发新身份。
func handleLogin(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	r *http.Request,
	state *wsConnState,
	payload []byte,
) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetLoginReq()
	if req == nil {
		return
	}
	if tok := req.GetSessionToken(); tok != "" && deps.Session != nil {
		handleLoginResume(ctx, deps, conn, state, &env, tok)
		return
	}
	handleLoginIssue(ctx, deps, conn, r, state, &env)
}

// handleLoginResume 处理带 session_token 的登录请求，覆盖错误、重定向与正常重连三种结果。
func handleLoginResume(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	env *clientv1.Envelope,
	tok string,
) {
	rr, err := deps.Rooms.Resume(ctx, tok)
	if err != nil {
		appmetrics.ReconnectTotal.WithLabelValues("error").Inc()
		code := clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED
		var resumeErr *ResumeError
		if errors.As(err, &resumeErr) && resumeErr != nil {
			code = resumeErr.Code
		}
		writeLoginError(conn, env.ReqId, code, err.Error())
		return
	}
	if rr.Redirect != nil {
		appmetrics.ReconnectTotal.WithLabelValues("redirect").Inc()
		writeLoginRedirect(conn, env.ReqId, rr, tok)
		return
	}

	state.userID = rr.UserID
	state.roomID = rr.RoomID
	if rr.Resumed && deps.Hub != nil && state.roomID != "" {
		deps.Hub.Register(state.userID, state.roomID, conn)
	}
	if rr.Resumed {
		appmetrics.ReconnectTotal.WithLabelValues("resumed").Inc()
		sinceCursor := rr.SnapshotSinceCursor
		if sinceCursor == "" && state.roomID != "" {
			sinceCursor = state.roomID + ":0"
		}
		if err := deps.Rooms.EnsureRoomEventSubscription(ctx, state.roomID, sinceCursor); err != nil {
			logx.Warn(ctx, "恢复后订阅房间事件流失败",
				"trace_id", "", "user_id", state.userID, "room_id", state.roomID, "err", err.Error())
		}
	}

	login := &clientv1.LoginResponse{
		UserId:       state.userID,
		SessionToken: tok,
		Resumed:      rr.Resumed,
		ResumeCursor: rr.SnapshotSinceCursor,
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED,
		ErrorMessage: "",
	}
	respEnv := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LoginResp{LoginResp: login}}
	b, _ := proto.Marshal(respEnv)
	_ = session.WriteBinary(conn, frame.Encode(msgid.LoginResp, b))

	if rr.Snapshot != nil {
		snapEnv := &clientv1.Envelope{ReqId: rr.SnapshotSinceCursor, Body: &clientv1.Envelope_Snapshot{Snapshot: rr.Snapshot}}
		sb, _ := proto.Marshal(snapEnv)
		_ = session.WriteBinary(conn, frame.Encode(msgid.SnapshotNotify, sb))
	}
	if rr.Settlement != nil {
		settleEnv := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_Settlement{Settlement: rr.Settlement}}
		sb, _ := proto.Marshal(settleEnv)
		_ = session.WriteBinary(conn, frame.Encode(msgid.Settlement, sb))
	}
}

// handleLoginIssue 在缺少 session_token 时为新连接分配 user_id 与令牌。
func handleLoginIssue(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	r *http.Request,
	state *wsConnState,
	env *clientv1.Envelope,
) {
	state.userID = roomsvc.NewUserID()
	var plainTok string
	if deps.Session != nil {
		var err error
		plainTok, err = deps.Session.Issue(ctx, state.userID, r.Host)
		if err != nil {
			logx.Warn(ctx, "签发会话令牌失败继续无令牌模式",
				"trace_id", "", "user_id", state.userID, "room_id", "", "err", err.Error())
		}
	}
	login := &clientv1.LoginResponse{UserId: state.userID, SessionToken: plainTok, Resumed: false}
	respEnv := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LoginResp{LoginResp: login}}
	b, _ := proto.Marshal(respEnv)
	_ = session.WriteBinary(conn, frame.Encode(msgid.LoginResp, b))
}

func writeLoginError(conn *websocket.Conn, reqID string, code clientv1.ErrorCode, msg string) {
	resp := &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		ErrorCode:    code,
		ErrorMessage: msg,
	}}}
	b, _ := proto.Marshal(resp)
	_ = session.WriteBinary(conn, frame.Encode(msgid.LoginResp, b))
}

func writeLoginRedirect(conn *websocket.Conn, reqID string, rr *ResumeResult, tok string) {
	resp := &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		UserId:       rr.UserID,
		SessionToken: tok,
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_ROUTE_REDIRECT,
		ErrorMessage: rr.Redirect.GetReason(),
	}}}
	b, _ := proto.Marshal(resp)
	_ = session.WriteBinary(conn, frame.Encode(msgid.LoginResp, b))

	redirectEnv := &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: rr.Redirect}}
	rb, _ := proto.Marshal(redirectEnv)
	_ = session.WriteBinary(conn, frame.Encode(msgid.RouteRedirectNotify, rb))
}

// handleHeartbeat 刷新 hub 心跳，并回 ServerTsMs；不引入幂等/限流（心跳本身高频）。
func handleHeartbeat(deps Deps, conn *websocket.Conn, state *wsConnState, payload []byte) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	if env.GetHeartbeatReq() == nil {
		return
	}
	if deps.Hub != nil && state.userID != "" {
		deps.Hub.TouchHeartbeat(state.userID)
	}
	resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_HeartbeatResp{
		HeartbeatResp: &clientv1.HeartbeatResponse{ServerTsMs: time.Now().UnixMilli()},
	}}
	b, _ := proto.Marshal(resp)
	_ = session.WriteBinary(conn, frame.Encode(msgid.HeartbeatResp, b))
}
