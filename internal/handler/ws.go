// Package handler 将二进制帧路由到具体业务，并调用应用服务层。
package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/pkg/logx"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Deps 为处理器依赖。
type Deps struct {
	Rooms   RoomGateway
	Hub     *session.Hub
	Session *session.Manager
}

// RoomGateway 抽象本地房间服务或远程 room/lobby gRPC 协调器。
type RoomGateway interface {
	Join(ctx context.Context, roomID, userID string) (int, error)
	Ready(ctx context.Context, roomID, userID string) (func(), error)
	Resume(ctx context.Context, sessionToken string) (*ResumeResult, error)
	EnsureRoomEventSubscription(ctx context.Context, roomID, sinceCursor string) error
}

// HandleWebSocket 升级为 WebSocket 并处理帧循环。
func HandleWebSocket(ctx context.Context, deps Deps, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logx.Error(ctx, "连接升级为 WebSocket 时失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return
	}
	defer func() { _ = session.CloseConn(conn) }()
	var userID string
	var roomID string
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		h, err := frame.ReadFrame(bytes.NewReader(data))
		if err != nil {
			logx.Warn(ctx, "二进制帧解析失败请检查客户端版本", "trace_id", "", "user_id", userID, "room_id", roomID, "err", err.Error())
			continue
		}
		wsFramesTotal.WithLabelValues(fmt.Sprintf("%d", h.MsgID)).Inc()
		switch h.MsgID {
		case msgid.LoginReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			req := env.GetLoginReq()
			if req == nil {
				continue
			}
			if tok := req.GetSessionToken(); tok != "" && deps.Session != nil {
				rr, err := deps.Rooms.Resume(ctx, tok)
				if err != nil {
					code := clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED
					var resumeErr *ResumeError
					if errors.As(err, &resumeErr) && resumeErr != nil {
						code = resumeErr.Code
					}
					resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
						ErrorCode:    code,
						ErrorMessage: err.Error(),
					}}}
					b, _ := proto.Marshal(resp)
					_ = session.WriteBinary(conn, frame.Encode(msgid.LoginResp, b))
					continue
				}
				if rr.Redirect != nil {
					resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
						UserId:       rr.UserID,
						SessionToken: tok,
						ErrorCode:    clientv1.ErrorCode_ERROR_CODE_ROUTE_REDIRECT,
						ErrorMessage: rr.Redirect.GetReason(),
					}}}
					b, _ := proto.Marshal(resp)
					_ = session.WriteBinary(conn, frame.Encode(msgid.LoginResp, b))
					redirectEnv := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: rr.Redirect}}
					rb, _ := proto.Marshal(redirectEnv)
					_ = session.WriteBinary(conn, frame.Encode(msgid.RouteRedirectNotify, rb))
					continue
				}
				userID = rr.UserID
				roomID = rr.RoomID
				if rr.Resumed && deps.Hub != nil && roomID != "" {
					deps.Hub.Register(userID, roomID, conn)
				}
				if rr.Resumed {
					if err := deps.Rooms.EnsureRoomEventSubscription(ctx, roomID, rr.SnapshotSinceCursor); err != nil {
						logx.Warn(ctx, "恢复后订阅房间事件流失败", "trace_id", "", "user_id", userID, "room_id", roomID, "err", err.Error())
					}
				}
				login := &clientv1.LoginResponse{
					UserId:       userID,
					SessionToken: tok,
					Resumed:      rr.Resumed,
					ResumeCursor: rr.SnapshotSinceCursor,
					ErrorCode:    clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED,
					ErrorMessage: "",
				}
				resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LoginResp{LoginResp: login}}
				b, _ := proto.Marshal(resp)
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
				continue
			}
			userID = roomsvc.NewUserID()
			var plainTok string
			if deps.Session != nil {
				var err error
				plainTok, err = deps.Session.Issue(ctx, userID, r.Host)
				if err != nil {
					logx.Warn(ctx, "签发会话令牌失败继续无令牌模式", "trace_id", "", "user_id", userID, "room_id", "", "err", err.Error())
				}
			}
			login := &clientv1.LoginResponse{UserId: userID, SessionToken: plainTok, Resumed: false}
			resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LoginResp{LoginResp: login}}
			b, _ := proto.Marshal(resp)
			_ = session.WriteBinary(conn, frame.Encode(msgid.LoginResp, b))
		case msgid.JoinRoomReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			jr := env.GetJoinRoomReq()
			if jr == nil || userID == "" {
				continue
			}
			roomID = jr.RoomId
			seat, err := deps.Rooms.Join(ctx, roomID, userID)
			if err != nil {
				resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_JoinRoomResp{JoinRoomResp: &clientv1.JoinRoomResponse{
					ErrorCode:    joinRoomErrorCode(err),
					ErrorMessage: err.Error(),
				}}}
				b, _ := proto.Marshal(resp)
				_ = session.WriteBinary(conn, frame.Encode(msgid.JoinRoomResp, b))
				continue
			}
			if deps.Session != nil {
				if err := deps.Session.BindRoom(ctx, userID, roomID); err != nil {
					resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_JoinRoomResp{JoinRoomResp: &clientv1.JoinRoomResponse{
						ErrorCode:    clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
						ErrorMessage: err.Error(),
					}}}
					b, _ := proto.Marshal(resp)
					_ = session.WriteBinary(conn, frame.Encode(msgid.JoinRoomResp, b))
					continue
				}
			}
			deps.Hub.Register(userID, roomID, conn)
			resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_JoinRoomResp{JoinRoomResp: &clientv1.JoinRoomResponse{
				SeatIndex: int32(seat), //nolint:gosec // G115：座位号 0..3
			}}}
			b, _ := proto.Marshal(resp)
			_ = session.WriteBinary(conn, frame.Encode(msgid.JoinRoomResp, b))
		case msgid.ReadyReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			if roomID == "" || userID == "" {
				continue
			}
			afterReady, err := deps.Rooms.Ready(ctx, roomID, userID)
			if err != nil {
				resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_ReadyResp{ReadyResp: &clientv1.ReadyResponse{
					ErrorCode:    clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
					ErrorMessage: err.Error(),
				}}}
				b, _ := proto.Marshal(resp)
				_ = session.WriteBinary(conn, frame.Encode(msgid.ReadyResp, b))
				continue
			}
			resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_ReadyResp{ReadyResp: &clientv1.ReadyResponse{}}}
			b, _ := proto.Marshal(resp)
			_ = session.WriteBinary(conn, frame.Encode(msgid.ReadyResp, b))
			if afterReady != nil {
				afterReady()
			}
		default:
			logx.Info(ctx, "收到尚未实现的消息编号已跳过", "trace_id", "", "user_id", userID, "room_id", roomID, "msg_id", fmt.Sprintf("%d", h.MsgID))
		}
	}
}

// joinRoomErrorCode 将进房失败映射为客户端 ErrorCode；未知错误使用 UNSPECIFIED，避免误报「房间已满」。
func joinRoomErrorCode(err error) clientv1.ErrorCode {
	if err == nil {
		return clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "room full"):
		return clientv1.ErrorCode_ERROR_CODE_ROOM_FULL
	case strings.Contains(msg, "room not found"):
		return clientv1.ErrorCode_ERROR_CODE_ROOM_NOT_FOUND
	case strings.Contains(msg, "invalid argument"):
		return clientv1.ErrorCode_ERROR_CODE_INVALID_STATE
	default:
		return clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
}

func outboundMsgID(kind roomsvc.Kind) (uint16, bool) {
	switch kind {
	case roomsvc.KindExchangeThreeDone:
		return msgid.ExchangeThreeDone, true
	case roomsvc.KindQueMenDone:
		return msgid.QueMenDone, true
	case roomsvc.KindStartGame:
		return msgid.StartGame, true
	case roomsvc.KindDrawTile:
		return msgid.DrawTile, true
	case roomsvc.KindAction:
		return msgid.ActionNotify, true
	case roomsvc.KindSettlement:
		return msgid.Settlement, true
	default:
		return 0, false
	}
}
