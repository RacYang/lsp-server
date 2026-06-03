package handler

import (
	"context"
	"strings"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/protocol"
	"racoo.cn/lsp/internal/session"
)

// handleRename 更新玩家昵称并回写应用结果。
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

// sanitizeNickname 过滤控制字符并截断超长昵称。
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
