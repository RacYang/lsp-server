package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	appmetrics "racoo.cn/lsp/internal/metrics"

	"racoo.cn/lsp/internal/protocol"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/pkg/logx"
)

// handleLogin 处理 LoginReq：带 session_token 走重连路径，否则签发新身份。
func handleLogin(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	_ *http.Request,
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
	handleLoginIssue(ctx, deps, conn, state, &env)
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
			logCtx := logx.WithRoomID(logx.WithUserID(ctx, state.userID), state.roomID)
			logx.Warn(logCtx, "恢复后订阅房间事件流失败", "err", err.Error())
		}
		// 玩家重连时取消 actor 内待触发的投降倒计时；fire-and-forget，失败不中断重连流程。
		if state.roomID != "" {
			if err := deps.Rooms.CancelOfflineSurrender(ctx, state.roomID, state.userID); err != nil {
				logCtx := logx.WithRoomID(logx.WithUserID(ctx, state.userID), state.roomID)
				logx.Warn(logCtx, "取消离线投降倒计时失败", "err", err.Error())
			}
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
	writeBinaryFrame(conn, protocol.LoginResp, b)

	if rr.Snapshot != nil {
		snapEnv := &clientv1.Envelope{ReqId: rr.SnapshotSinceCursor, Body: &clientv1.Envelope_Snapshot{Snapshot: rr.Snapshot}}
		sb, _ := proto.Marshal(snapEnv)
		writeBinaryFrame(conn, protocol.SnapshotNotify, sb)
	}
	if len(rr.SettlementPayload) > 0 {
		// SettlementPayload 已是序列化的 Envelope proto 字节，直接推送无须重新序列化。
		writeBinaryFrame(conn, protocol.Settlement, rr.SettlementPayload)
	}
}

// handleLoginIssue 在缺少 session_token 时为新连接分配 user_id 与令牌。
func handleLoginIssue(
	ctx context.Context,
	deps Deps,
	conn *websocket.Conn,
	state *wsConnState,
	env *clientv1.Envelope,
) {
	state.userID = uuid.NewString()
	nickname := sanitizeNickname(env.GetLoginReq().GetNickname())
	if deps.Users != nil {
		if err := deps.Users.Set(ctx, state.userID, session.UserProfile{Nickname: nickname}); err != nil {
			logx.Warn(logx.WithUserID(ctx, state.userID), "写入用户 profile 失败", "err", err.Error())
		}
	}
	var plainTok string
	if deps.Session != nil {
		var err error
		plainTok, err = deps.Session.Issue(ctx, state.userID)
		if err != nil {
			logCtx := logx.WithUserID(ctx, state.userID)
			logx.Warn(logCtx, "签发会话令牌失败继续无令牌模式", "err", err.Error())
		}
	}
	login := &clientv1.LoginResponse{UserId: state.userID, SessionToken: plainTok, Resumed: false}
	respEnv := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LoginResp{LoginResp: login}}
	b, _ := proto.Marshal(respEnv)
	writeBinaryFrame(conn, protocol.LoginResp, b)
	logx.Info(logx.WithUserID(ctx, state.userID), "玩家登录成功")
}

func handleRename(ctx context.Context, deps Deps, conn *websocket.Conn, state *wsConnState, payload []byte) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetRenameReq()
	if req == nil {
		return
	}
	if state.userID == "" {
		writeRenameResponse(conn, env.ReqId, clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED, "尚未登录", "")
		return
	}
	nickname := sanitizeNickname(req.GetNickname())
	if nickname == "" {
		writeRenameResponse(conn, env.ReqId, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, "昵称不能为空", "")
		return
	}
	if deps.Users != nil {
		if err := deps.Users.Set(ctx, state.userID, session.UserProfile{Nickname: nickname}); err != nil {
			writeRenameResponse(conn, env.ReqId, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, err.Error(), "")
			return
		}
	}
	writeRenameResponse(conn, env.ReqId, clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "", nickname)
}

func sanitizeNickname(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

func writeRenameResponse(conn *websocket.Conn, reqID string, code clientv1.ErrorCode, msg, applied string) {
	resp := &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_RenameResp{RenameResp: &clientv1.RenameResponse{
		ErrorCode:       code,
		ErrorMessage:    msg,
		AppliedNickname: applied,
	}}}
	b, _ := proto.Marshal(resp)
	writeBinaryFrame(conn, protocol.RenameResp, b)
}

func writeLoginError(conn *websocket.Conn, reqID string, code clientv1.ErrorCode, msg string) {
	resp := &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		ErrorCode:    code,
		ErrorMessage: msg,
	}}}
	b, _ := proto.Marshal(resp)
	writeBinaryFrame(conn, protocol.LoginResp, b)
}

func writeLoginRedirect(conn *websocket.Conn, reqID string, rr *ResumeResult, tok string) {
	resp := &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		UserId:       rr.UserID,
		SessionToken: tok,
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_ROUTE_REDIRECT,
		ErrorMessage: rr.Redirect.GetReason(),
	}}}
	b, _ := proto.Marshal(resp)
	writeBinaryFrame(conn, protocol.LoginResp, b)

	redirectEnv := &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: rr.Redirect}}
	rb, _ := proto.Marshal(redirectEnv)
	writeBinaryFrame(conn, protocol.RouteRedirectNotify, rb)
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
	writeBinaryFrame(conn, protocol.HeartbeatResp, b)
}
