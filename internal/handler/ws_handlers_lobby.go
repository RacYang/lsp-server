package handler

import (
	"context"
	"strings"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/pkg/logx"
)

const defaultClientRuleID = "sichuan_xzdd"

// handleListRooms 返回大厅可加入的公开房间列表；查询不改变服务端状态，因此不走幂等缓存。
func handleListRooms(ctx context.Context, deps Deps, conn *websocket.Conn, state *wsConnState, payload []byte) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetListRoomsReq()
	if req == nil {
		return
	}
	if state.userID == "" {
		writeLobbyResponse(conn, msgid.ListRoomsResp, &clientv1.Envelope{
			ReqId: env.ReqId,
			Body: &clientv1.Envelope_ListRoomsResp{ListRoomsResp: &clientv1.ListRoomsResponse{
				ErrorCode:    clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
				ErrorMessage: "尚未登录",
			}},
		})
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
	if req == nil {
		return
	}
	if state.userID == "" {
		writeLobbyResponse(conn, msgid.AutoMatchResp, &clientv1.Envelope{
			ReqId: env.ReqId,
			Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{
				ErrorCode:    clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
				ErrorMessage: "尚未登录",
			}},
		})
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	roomID, seat, err := deps.Rooms.AutoMatch(ctx, req.GetRuleId(), state.userID, req.GetPadWithBots())
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
	seats := selfSeatInfo(ctx, deps, int32(seat), state.userID) //nolint:gosec // 座位号固定为 0..3
	var afterAddBot func()
	if req.GetPadWithBots() {
		added, after, err := deps.Rooms.AddBot(ctx, roomID, state.userID, 3, "normal", "automatch:"+roomID+":"+state.userID)
		if err != nil {
			writeLobbyResponse(conn, msgid.AutoMatchResp, &clientv1.Envelope{
				ReqId: env.ReqId,
				Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{
					ErrorCode:    clientv1.ErrorCode_ERROR_CODE_INVALID_STATE,
					ErrorMessage: err.Error(),
				}},
			})
			return
		}
		seats = mergeSeatInfos(seats, added)
		afterAddBot = after
	}
	writeLobbyResponse(conn, msgid.AutoMatchResp, &clientv1.Envelope{
		ReqId: env.ReqId,
		Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{
			RoomId:      roomID,
			SeatIndex:   int32(seat), //nolint:gosec // 座位号固定为 0..3
			RuleId:      normalizeClientRuleID(req.GetRuleId()),
			DisplayName: roomID,
			Seats:       seats,
		}},
	})
	if afterAddBot != nil {
		afterAddBot()
	}
}

func handleAddBot(ctx context.Context, deps Deps, conn *websocket.Conn, state *wsConnState, msgID uint16, payload []byte) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetAddBotReq()
	if req == nil || state.userID == "" || state.roomID == "" {
		return
	}
	if shouldDropRequest(&env, msgID, state.userID) {
		return
	}
	added, after, err := deps.Rooms.AddBot(ctx, state.roomID, state.userID, req.GetCount(), req.GetDifficulty(), req.GetOpId())
	resp := &clientv1.AddBotResponse{Added: added}
	if err != nil {
		resp.ErrorCode = clientv1.ErrorCode_ERROR_CODE_INVALID_STATE
		resp.ErrorMessage = err.Error()
	}
	writeLobbyResponse(conn, msgid.AddBotResp, &clientv1.Envelope{
		ReqId: env.ReqId,
		Body:  &clientv1.Envelope_AddBotResp{AddBotResp: resp},
	})
	if err == nil {
		logx.Info(logx.WithRoomID(logx.WithUserID(ctx, state.userID), state.roomID), "玩家添加机器人", "count", len(added))
		if after != nil {
			after()
		}
	}
}

func mergeSeatInfos(base, added []*clientv1.SeatInfo) []*clientv1.SeatInfo {
	if len(added) == 0 {
		return base
	}
	out := append([]*clientv1.SeatInfo(nil), base...)
	for _, seat := range added {
		replaced := false
		for i, existing := range out {
			if existing.GetSeatIndex() == seat.GetSeatIndex() {
				out[i] = seat
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, seat)
		}
	}
	return out
}

// handleCreateRoom 创建房间后直接让创建者入座；私密房只可凭 room_id 手动加入。
func handleCreateRoom(ctx context.Context, deps Deps, conn *websocket.Conn, state *wsConnState, msgID uint16, payload []byte) {
	var env clientv1.Envelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		return
	}
	req := env.GetCreateRoomReq()
	if req == nil {
		return
	}
	if state.userID == "" {
		writeLobbyResponse(conn, msgid.CreateRoomResp, &clientv1.Envelope{
			ReqId: env.ReqId,
			Body: &clientv1.Envelope_CreateRoomResp{CreateRoomResp: &clientv1.CreateRoomResponse{
				ErrorCode:    clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
				ErrorMessage: "尚未登录",
			}},
		})
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
			RoomId:      roomID,
			SeatIndex:   int32(seat), //nolint:gosec // 座位号固定为 0..3
			RuleId:      normalizeClientRuleID(req.GetRuleId()),
			DisplayName: normalizeClientDisplayName(req.GetDisplayName(), roomID),
			Seats:       selfSeatInfo(ctx, deps, int32(seat), state.userID), //nolint:gosec // 座位号固定为 0..3
		}},
	})
}

func normalizeClientRuleID(ruleID string) string {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return defaultClientRuleID
	}
	return ruleID
}

func normalizeClientDisplayName(displayName, roomID string) string {
	displayName = strings.TrimSpace(displayName)
	if displayName != "" {
		return displayName
	}
	return roomID
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
