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

// handleListRooms 返回大厅可加入的公开房间列表；查询不改变服务端状态，因此不走幂等缓存。
func handleListRooms(ctx context.Context, deps Deps, conn *websocket.Conn, state *wsConnState, payload []byte) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetListRoomsReq()
	if req == nil || state.userID == "" {
		return
	}
	rooms, next, err := deps.Rooms.ListRooms(ctx, req.GetPageSize(), req.GetPageToken())
	resp := &clientv1.ListRoomsResponse{Rooms: rooms, NextPageToken: next}
	if err != nil {
		resp.ErrorCode = clientv1.ErrorCode_ERROR_CODE_INVALID_STATE
		resp.ErrorMessage = err.Error()
	}
	writeLobbyResponse(conn, msgid.ListRoomsResp, &clientv1.Envelope{
		ReqId: env.ReqId,
		Body:  &clientv1.Envelope_ListRoomsResp{ListRoomsResp: resp},
	})
}

// handleAutoMatch 在大厅侧选择或创建房间，并复用进房后的 session/hub 绑定流程。
func handleAutoMatch(ctx context.Context, deps Deps, conn *websocket.Conn, state *wsConnState, msgID uint16, payload []byte) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetAutoMatchReq()
	if req == nil || state.userID == "" {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	roomID, seat, err := deps.Rooms.AutoMatch(ctx, req.GetRuleId(), state.userID)
	if err != nil {
		writeLobbyResponse(conn, msgid.AutoMatchResp, &clientv1.Envelope{
			ReqId: env.ReqId,
			Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{
				ErrorCode:    joinRoomErrorCode(err),
				ErrorMessage: err.Error(),
			}},
		})
		return
	}
	if err := bindJoinedRoom(ctx, deps, conn, state, roomID); err != nil {
		writeLobbyResponse(conn, msgid.AutoMatchResp, &clientv1.Envelope{
			ReqId: env.ReqId,
			Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{
				ErrorCode:    clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
				ErrorMessage: err.Error(),
			}},
		})
		return
	}
	writeLobbyResponse(conn, msgid.AutoMatchResp, &clientv1.Envelope{
		ReqId: env.ReqId,
		Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{
			RoomId:    roomID,
			SeatIndex: int32(seat), //nolint:gosec // 座位号固定为 0..3
		}},
	})
}

// handleCreateRoom 创建房间后直接让创建者入座；私密房只可凭 room_id 手动加入。
func handleCreateRoom(ctx context.Context, deps Deps, conn *websocket.Conn, state *wsConnState, msgID uint16, payload []byte) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetCreateRoomReq()
	if req == nil || state.userID == "" {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	roomID, seat, err := deps.Rooms.CreateRoom(ctx, req.GetRuleId(), req.GetDisplayName(), req.GetPrivate(), state.userID)
	if err != nil {
		writeLobbyResponse(conn, msgid.CreateRoomResp, &clientv1.Envelope{
			ReqId: env.ReqId,
			Body: &clientv1.Envelope_CreateRoomResp{CreateRoomResp: &clientv1.CreateRoomResponse{
				ErrorCode:    joinRoomErrorCode(err),
				ErrorMessage: err.Error(),
			}},
		})
		return
	}
	if err := bindJoinedRoom(ctx, deps, conn, state, roomID); err != nil {
		writeLobbyResponse(conn, msgid.CreateRoomResp, &clientv1.Envelope{
			ReqId: env.ReqId,
			Body: &clientv1.Envelope_CreateRoomResp{CreateRoomResp: &clientv1.CreateRoomResponse{
				ErrorCode:    clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
				ErrorMessage: err.Error(),
			}},
		})
		return
	}
	writeLobbyResponse(conn, msgid.CreateRoomResp, &clientv1.Envelope{
		ReqId: env.ReqId,
		Body: &clientv1.Envelope_CreateRoomResp{CreateRoomResp: &clientv1.CreateRoomResponse{
			RoomId:    roomID,
			SeatIndex: int32(seat), //nolint:gosec // 座位号固定为 0..3
		}},
	})
}

func bindJoinedRoom(ctx context.Context, deps Deps, conn *websocket.Conn, state *wsConnState, roomID string) error {
	state.roomID = roomID
	if deps.Session != nil {
		if err := deps.Session.BindRoom(ctx, state.userID, state.roomID); err != nil {
			return err
		}
	}
	if deps.Hub != nil {
		deps.Hub.Register(state.userID, state.roomID, conn)
	}
	return nil
}

func writeLobbyResponse(conn *websocket.Conn, outMsgID uint16, env *clientv1.Envelope) {
	b, _ := proto.Marshal(env)
	_ = session.WriteBinary(conn, frame.Encode(outMsgID, b))
}
