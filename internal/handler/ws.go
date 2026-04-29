// Package handler 将二进制帧路由到具体业务，并调用应用服务层。
package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
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
	ListRooms(ctx context.Context, pageSize int32, pageToken string) ([]*clientv1.RoomMeta, string, error)
	AutoMatch(ctx context.Context, ruleID, userID string) (string, int, error)
	CreateRoom(ctx context.Context, ruleID, displayName string, private bool, userID string) (string, int, error)
	Resume(ctx context.Context, sessionToken string) (*ResumeResult, error)
	EnsureRoomEventSubscription(ctx context.Context, roomID, sinceCursor string) error
}

// wsConnState 跨帧维护的可变身份；handler 唯一允许写者，遵守"单写者"约定。
type wsConnState struct {
	userID string
	roomID string
}

// HandleWebSocket 升级为 WebSocket 并启动单连接的帧读循环。
//
// 写出由本函数与本包内 handler 串行触达，避免 gorilla/websocket 的并发写者风险。
func HandleWebSocket(ctx context.Context, deps Deps, w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(req *http.Request) bool {
			return allowWebSocketOrigin(req, deps.AllowedOrigins)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logx.Error(ctx, "连接升级为 WebSocket 时失败", "err", err.Error())
		return
	}
	state := wsConnState{}
	defer func() {
		if deps.Hub != nil && state.userID != "" {
			deps.Hub.Unregister(state.userID, state.roomID)
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
			logCtx := logx.WithRoomID(logx.WithUserID(ctx, state.userID), state.roomID)
			logx.Warn(logCtx, "二进制帧解析失败请检查客户端版本",
				"err", err.Error())
			continue
		}
		if deps.Hub != nil {
			deps.Hub.CloseExpiredHeartbeats()
		}
		wsFramesTotal.WithLabelValues(fmt.Sprintf("%d", h.MsgID)).Inc()

		dispatchFrame(ctx, deps, conn, r, &state, h)
	}
}
