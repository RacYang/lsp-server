// Package handler 将二进制帧路由到具体业务，并调用应用服务层。
package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

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
	Rooms *roomsvc.Service
	Hub   *session.Hub
}

// HandleWebSocket 升级为 WebSocket 并处理帧循环。
func HandleWebSocket(ctx context.Context, deps Deps, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logx.Error(ctx, "连接升级为 WebSocket 时失败", "trace_id", "", "user_id", "", "room_id", "", "err", err.Error())
		return
	}
	defer func() { _ = conn.Close() }()
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
			userID = roomsvc.NewUserID()
			resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_LoginResp{
				LoginResp: &clientv1.LoginResponse{UserId: userID},
			}}
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
					ErrorCode: clientv1.ErrorCode_ERROR_CODE_ROOM_FULL,
				}}}
				b, _ := proto.Marshal(resp)
				_ = session.WriteBinary(conn, frame.Encode(msgid.JoinRoomResp, b))
				continue
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
			settlementPayload, err := deps.Rooms.Ready(ctx, roomID, userID)
			if err != nil {
				resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_ReadyResp{ReadyResp: &clientv1.ReadyResponse{
					ErrorCode: clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
				}}}
				b, _ := proto.Marshal(resp)
				_ = session.WriteBinary(conn, frame.Encode(msgid.ReadyResp, b))
				continue
			}
			resp := &clientv1.Envelope{ReqId: env.ReqId, Body: &clientv1.Envelope_ReadyResp{ReadyResp: &clientv1.ReadyResponse{}}}
			b, _ := proto.Marshal(resp)
			_ = session.WriteBinary(conn, frame.Encode(msgid.ReadyResp, b))
			if len(settlementPayload) > 0 {
				deps.Hub.Broadcast(roomID, msgid.Settlement, settlementPayload)
			}
		default:
			logx.Info(ctx, "收到尚未实现的消息编号已跳过", "trace_id", "", "user_id", userID, "room_id", roomID, "msg_id", fmt.Sprintf("%d", h.MsgID))
		}
	}
}
