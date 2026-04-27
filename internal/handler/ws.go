// Package handler 将二进制帧路由到具体业务，并调用应用服务层。
package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

// Deps 为处理器依赖。
type Deps struct {
	Rooms   RoomGateway
	Hub     *session.Hub
	Session *session.Manager
	// AllowedOrigins 非空时表示允许跨站 WebSocket 的白名单；为空时退回同源校验。
	AllowedOrigins []string
}

// RoomGateway 抽象本地房间服务或远程 room/lobby gRPC 协调器。
type RoomGateway interface {
	Join(ctx context.Context, roomID, userID string) (int, error)
	Ready(ctx context.Context, roomID, userID string) (func(), error)
	Leave(ctx context.Context, roomID, userID string) (func(), error)
	ExchangeThree(ctx context.Context, roomID, userID string, tiles []string, direction int32) (func(), error)
	QueMen(ctx context.Context, roomID, userID string, suit int32) (func(), error)
	Discard(ctx context.Context, roomID, userID, tile string) (func(), error)
	Pong(ctx context.Context, roomID, userID string) (func(), error)
	Gang(ctx context.Context, roomID, userID, tile string) (func(), error)
	Hu(ctx context.Context, roomID, userID string) (func(), error)
	Resume(ctx context.Context, sessionToken string) (*ResumeResult, error)
	EnsureRoomEventSubscription(ctx context.Context, roomID, sinceCursor string) error
}

// HandleWebSocket 升级为 WebSocket 并处理帧循环。
func HandleWebSocket(ctx context.Context, deps Deps, w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(req *http.Request) bool {
			return allowWebSocketOrigin(req, deps.AllowedOrigins)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logx.Error(ctx, "连接升级为 WebSocket 时失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return
	}
	var userID string
	var roomID string
	defer func() {
		if deps.Hub != nil && userID != "" {
			deps.Hub.Unregister(userID, roomID)
		}
		_ = session.CloseConn(conn)
	}()
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
		if deps.Hub != nil {
			deps.Hub.CloseExpiredHeartbeats()
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
					appmetrics.ReconnectTotal.WithLabelValues("error").Inc()
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
					appmetrics.ReconnectTotal.WithLabelValues("redirect").Inc()
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
					appmetrics.ReconnectTotal.WithLabelValues("resumed").Inc()
					sinceCursor := rr.SnapshotSinceCursor
					if sinceCursor == "" && roomID != "" {
						sinceCursor = roomID + ":0"
					}
					if err := deps.Rooms.EnsureRoomEventSubscription(ctx, roomID, sinceCursor); err != nil {
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
			if shouldDropRequest(&env, h.MsgID, userID) {
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
		case msgid.HeartbeatReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			if env.GetHeartbeatReq() == nil {
				continue
			}
			if deps.Hub != nil && userID != "" {
				deps.Hub.TouchHeartbeat(userID)
			}
			resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_HeartbeatResp{
				HeartbeatResp: &clientv1.HeartbeatResponse{ServerTsMs: time.Now().UnixMilli()},
			}}
			b, _ := proto.Marshal(resp)
			_ = session.WriteBinary(conn, frame.Encode(msgid.HeartbeatResp, b))
		case msgid.ExchangeThreeReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			req := env.GetExchangeThreeReq()
			if req == nil || roomID == "" || userID == "" {
				continue
			}
			if shouldDropRequest(&env, h.MsgID, userID) {
				continue
			}
			after, err := deps.Rooms.ExchangeThree(ctx, roomID, userID, req.GetTiles(), req.GetDirection())
			resp, after := exchangeThreeErrEnvelope(env.ReqId, after, err)
			respondAction(conn, env.ReqId, msgid.ExchangeThreeResp, resp, after)
		case msgid.QueMenReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			req := env.GetQueMenReq()
			if req == nil || roomID == "" || userID == "" {
				continue
			}
			if shouldDropRequest(&env, h.MsgID, userID) {
				continue
			}
			after, err := deps.Rooms.QueMen(ctx, roomID, userID, req.GetSuit())
			resp, after := queMenErrEnvelope(env.ReqId, after, err)
			respondAction(conn, env.ReqId, msgid.QueMenResp, resp, after)
		case msgid.LeaveRoomReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			if env.GetLeaveRoomReq() == nil {
				continue
			}
			if shouldDropRequest(&env, h.MsgID, userID) {
				continue
			}
			if roomID == "" || userID == "" {
				resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LeaveRoomResp{LeaveRoomResp: &clientv1.LeaveRoomResponse{
					ErrorCode:    clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
					ErrorMessage: "尚未进入房间",
				}}}
				b, _ := proto.Marshal(resp)
				_ = session.WriteBinary(conn, frame.Encode(msgid.LeaveRoomResp, b))
				continue
			}
			oldRoomID := roomID
			after, err := deps.Rooms.Leave(ctx, roomID, userID)
			if err != nil {
				resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LeaveRoomResp{LeaveRoomResp: &clientv1.LeaveRoomResponse{
					ErrorCode:    clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
					ErrorMessage: err.Error(),
				}}}
				b, _ := proto.Marshal(resp)
				_ = session.WriteBinary(conn, frame.Encode(msgid.LeaveRoomResp, b))
				continue
			}
			roomID = ""
			if deps.Hub != nil {
				deps.Hub.Unregister(userID, oldRoomID)
			}
			if deps.Session != nil {
				_ = deps.Session.UnbindRoom(ctx, userID)
			}
			resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LeaveRoomResp{LeaveRoomResp: &clientv1.LeaveRoomResponse{}}}
			b, _ := proto.Marshal(resp)
			_ = session.WriteBinary(conn, frame.Encode(msgid.LeaveRoomResp, b))
			if after != nil {
				after()
			}
		case msgid.DiscardReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			req := env.GetDiscardReq()
			if req == nil || roomID == "" || userID == "" {
				continue
			}
			if shouldDropRequest(&env, h.MsgID, userID) {
				continue
			}
			after, err := deps.Rooms.Discard(ctx, roomID, userID, req.GetTile())
			resp, after := discardErrEnvelope(env.ReqId, after, err)
			respondAction(conn, env.ReqId, msgid.DiscardResp, resp, after)
		case msgid.PongReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			if env.GetPongReq() == nil || roomID == "" || userID == "" {
				continue
			}
			if shouldDropRequest(&env, h.MsgID, userID) {
				continue
			}
			after, err := deps.Rooms.Pong(ctx, roomID, userID)
			resp, after := pongErrEnvelope(env.ReqId, after, err)
			respondAction(conn, env.ReqId, msgid.PongResp, resp, after)
		case msgid.GangReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			req := env.GetGangReq()
			if req == nil || roomID == "" || userID == "" {
				continue
			}
			if shouldDropRequest(&env, h.MsgID, userID) {
				continue
			}
			after, err := deps.Rooms.Gang(ctx, roomID, userID, req.GetTile())
			resp, after := gangErrEnvelope(env.ReqId, after, err)
			respondAction(conn, env.ReqId, msgid.GangResp, resp, after)
		case msgid.HuReq:
			var env clientv1.Envelope
			if err := proto.Unmarshal(h.Payload, &env); err != nil {
				continue
			}
			if env.GetHuReq() == nil || roomID == "" || userID == "" {
				continue
			}
			if shouldDropRequest(&env, h.MsgID, userID) {
				continue
			}
			after, err := deps.Rooms.Hu(ctx, roomID, userID)
			resp, after := huErrEnvelope(env.ReqId, after, err)
			respondAction(conn, env.ReqId, msgid.HuResp, resp, after)
		default:
			unknownMsgTotal.Inc()
			logx.Info(ctx, "收到尚未实现的消息编号已跳过", "trace_id", "", "user_id", userID, "room_id", roomID, "msg_id", fmt.Sprintf("%d", h.MsgID))
		}
	}
}

func allowWebSocketOrigin(r *http.Request, allowedOrigins []string) bool {
	if r == nil {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if len(allowedOrigins) > 0 {
		for _, allowed := range allowedOrigins {
			if strings.EqualFold(strings.TrimSpace(allowed), origin) {
				return true
			}
		}
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func shouldDropRequest(env *clientv1.Envelope, msgID uint16, userID string) bool {
	if env == nil {
		return false
	}
	if !defaultWSRateLimiter.Allow(userID) {
		rateLimitedTotal.WithLabelValues("gate").Inc()
		return true
	}
	key := strings.TrimSpace(env.GetIdempotencyKey())
	if key == "" {
		return false
	}
	if defaultWSIdemCache.SeenOrStore("ws", msgID, userID, key) {
		idempotentReplayTotal.Inc()
		return true
	}
	return false
}

func respondAction(conn *websocket.Conn, reqID string, responseMsgID uint16, env *clientv1.Envelope, after func()) {
	b, _ := proto.Marshal(env)
	_ = session.WriteBinary(conn, frame.Encode(responseMsgID, b))
	if after != nil {
		after()
	}
}

func discardErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_DiscardResp{DiscardResp: &clientv1.DiscardResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_DiscardResp{DiscardResp: &clientv1.DiscardResponse{}}}, after
}

func pongErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_PongResp{PongResp: &clientv1.PongResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_PongResp{PongResp: &clientv1.PongResponse{}}}, after
}

func gangErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_GangResp{GangResp: &clientv1.GangResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_GangResp{GangResp: &clientv1.GangResponse{}}}, after
}

func huErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_HuResp{HuResp: &clientv1.HuResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_HuResp{HuResp: &clientv1.HuResponse{}}}, after
}

func exchangeThreeErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_ExchangeThreeResp{ExchangeThreeResp: &clientv1.ExchangeThreeResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_ExchangeThreeResp{ExchangeThreeResp: &clientv1.ExchangeThreeResponse{}}}, after
}

func queMenErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_QueMenResp{QueMenResp: &clientv1.QueMenResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_QueMenResp{QueMenResp: &clientv1.QueMenResponse{}}}, after
}

func actionErrorCode(err error) clientv1.ErrorCode {
	if errors.Is(err, roomsvc.ErrRateLimited) {
		rateLimitedTotal.WithLabelValues("room").Inc()
		return clientv1.ErrorCode_ERROR_CODE_RATE_LIMITED
	}
	return clientv1.ErrorCode_ERROR_CODE_INVALID_STATE
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
