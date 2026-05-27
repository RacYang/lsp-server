// Package handler 将二进制帧路由到具体业务，并调用应用服务层。
package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/protocol"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/pkg/logx"
)

const (
	// wsPingInterval 每隔此时长向客户端发送 ping 控制帧，确认链路存活。
	wsPingInterval = 30 * time.Second
	// wsReadTimeout 两次有效消息（含 pong）之间的最大静默时间；超时后 ReadMessage 返回错误触发断开。
	wsReadTimeout = 90 * time.Second
	// wsPingWriteTimeout ping 控制帧写入的超时上限。
	wsPingWriteTimeout = 5 * time.Second
)

// Deps 为处理器依赖。
type Deps struct {
	Rooms   RoomGateway
	Hub     *session.Hub
	Session *session.Manager
	Users   *session.UserDirectory
	// AllowedOrigins 非空时表示允许跨站 WebSocket 的白名单；为空时退回同源校验。
	AllowedOrigins []string
}

// RoomGateway 抽象本地房间服务或远程 room/lobby gRPC 协调器。
type RoomGateway interface {
	Join(ctx context.Context, roomID, userID string) (int, error)
	Ready(ctx context.Context, roomID, userID string) (func(), error)
	Leave(ctx context.Context, roomID, userID string) (func(), error)
	MarkSeatOffline(ctx context.Context, roomID, userID string) error
	CancelOfflineSurrender(ctx context.Context, roomID, userID string) error
	OpeningAction(ctx context.Context, roomID, userID, action string, tiles []string, direction, suit int32, params map[string]string, tok *clientv1.PhaseToken) (func(), error)
	Discard(ctx context.Context, roomID, userID, tile string, tok *clientv1.PhaseToken) (func(), error)
	Pong(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error)
	Chi(ctx context.Context, roomID, userID string, tiles []string, tok *clientv1.PhaseToken) (func(), error)
	Gang(ctx context.Context, roomID, userID, tile string, tok *clientv1.PhaseToken) (func(), error)
	Hu(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error)
	Pass(ctx context.Context, roomID, userID string, tok *clientv1.PhaseToken) (func(), error)
	ListRooms(ctx context.Context, pageSize int32, pageToken string) ([]*clientv1.RoomMeta, string, error)
	ListRules(ctx context.Context) ([]*clientv1.RuleMeta, error)
	AutoMatch(ctx context.Context, ruleID, userID string, padWithBots bool) (string, int, error)
	CreateRoom(ctx context.Context, ruleID, displayName string, private bool, userID string) (string, int, error)
	AddBot(ctx context.Context, roomID, userID string, count int32, difficulty, opID string) ([]*clientv1.SeatInfo, func(), error)
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
// 写端由 session.WriteBinary 的每连接写协程序列化，ping 控制帧通过 gorilla 并发安全的
// WriteControl 发出，与业务帧写入互不干扰。
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

	// 初始读超时；pong handler 与每次成功读取都会续期，确保僵尸连接被及时清理。
	_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		return nil
	})

	// ping goroutine 与读循环并行运行，done 关闭时退出。
	// WriteControl 是 gorilla/websocket 文档明确并发安全的方法，可与写协程并存。
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(wsPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsPingWriteTimeout)); err != nil {
					return
				}
			}
		}
	}()

	state := wsConnState{}
	defer func() {
		close(done)
		if deps.Rooms != nil && state.userID != "" && state.roomID != "" {
			_ = deps.Rooms.MarkSeatOffline(context.Background(), state.roomID, state.userID)
		}
		if deps.Hub != nil && state.userID != "" {
			deps.Hub.UnregisterConn(state.userID, state.roomID, conn)
		}
		_ = session.CloseConn(conn)
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// 任意数据帧到达即续期，避免高负载时大量业务帧被误判超时。
		_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		h, err := protocol.ReadFrame(bytes.NewReader(data))
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

// writeBinaryFrame 编码消息帧并写入 WebSocket 连接；
// 仅在载荷超过 MaxPayload（4MiB）时静默丢弃（实践中 proto 消息不会达到此量级）。
func writeBinaryFrame(conn *websocket.Conn, msgID uint16, payload []byte) {
	enc, err := protocol.Encode(msgID, payload)
	if err != nil {
		return
	}
	_ = session.WriteBinary(conn, enc)
}
