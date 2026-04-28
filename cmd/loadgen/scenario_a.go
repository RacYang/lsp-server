//nolint:staticcheck // ADR-0025 指定 nhooyr.io/websocket 作为压测客户端依赖。
package main

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"nhooyr.io/websocket"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
)

type benchClient struct {
	userID       string
	sessionToken string
	conn         *websocket.Conn
}

// runScenarioA 执行单房间稳态剧本：四名玩家登录、进房、准备，并用心跳维持连接流量。
func runScenarioA(ctx context.Context, cfg scenarioConfig) (scenarioSummary, error) {
	summary := scenarioSummary{
		Scenario:       "a",
		Version:        cfg.Version,
		Rooms:          cfg.Rooms,
		PlayersPerRoom: cfg.PlayersPerRoom,
		RoundCount:     cfg.RoundCount,
		RuntimeAdvice: []string{
			"若心跳与 ready 延迟稳定，可保持 runtime.gate.ws_rate_limit_* 默认值",
			"Scenario B/C 取得 mailbox 与重连数据后再调整 runtime.room.mailbox_capacity",
		},
	}
	for roomIndex := 0; roomIndex < cfg.Rooms; roomIndex++ {
		roomID := fmt.Sprintf("%s-%d", cfg.RoomIDPrefix, roomIndex)
		clients, requests, err := prepareRoom(ctx, cfg, roomID)
		summary.Requests += requests
		if err != nil {
			summary.Errors++
			summary.Notes = append(summary.Notes, fmt.Sprintf("房间 %s 准备失败: %v", roomID, err))
			continue
		}
		for _, c := range clients {
			defer func(c *benchClient) {
				_ = c.conn.Close(websocket.StatusNormalClosure, "loadgen done")
			}(c)
		}
		for round := 0; round < cfg.RoundCount; round++ {
			for _, c := range clients {
				if err := c.send(ctx, msgid.HeartbeatReq, &clientv1.Envelope{
					ReqId: fmt.Sprintf("%s-heartbeat-%d", c.userID, round),
					Body:  &clientv1.Envelope_HeartbeatReq{HeartbeatReq: &clientv1.HeartbeatRequest{ClientTsMs: time.Now().UnixMilli()}},
				}); err != nil {
					summary.Errors++
					summary.Notes = append(summary.Notes, fmt.Sprintf("心跳发送失败: %v", err))
					continue
				}
				summary.Requests++
			}
		}
	}
	summary.Passed = summary.Errors == 0 && summary.Requests > 0
	if summary.Passed {
		summary.Notes = append(summary.Notes, "Scenario A 完成登录、进房、准备与稳态心跳")
	}
	return summary, nil
}

func prepareRoom(ctx context.Context, cfg scenarioConfig, roomID string) ([]*benchClient, int, error) {
	clients := make([]*benchClient, 0, cfg.PlayersPerRoom)
	requests := 0
	for seat := 0; seat < cfg.PlayersPerRoom; seat++ {
		conn, _, err := websocket.Dial(ctx, cfg.WSURL, nil)
		if err != nil {
			return nil, requests, err
		}
		client := &benchClient{conn: conn}
		if err := client.send(ctx, msgid.LoginReq, &clientv1.Envelope{
			ReqId: fmt.Sprintf("%s-login-%d", roomID, seat),
			Body:  &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{Nickname: fmt.Sprintf("压测玩家%d", seat)}},
		}); err != nil {
			return nil, requests, err
		}
		requests++
		loginResp, err := client.readUntil(ctx, msgid.LoginResp)
		if err != nil {
			return nil, requests, err
		}
		client.userID = loginResp.GetLoginResp().GetUserId()
		client.sessionToken = loginResp.GetLoginResp().GetSessionToken()
		if client.userID == "" {
			return nil, requests, fmt.Errorf("登录响应缺少 user_id")
		}
		if err := client.send(ctx, msgid.JoinRoomReq, &clientv1.Envelope{
			ReqId: fmt.Sprintf("%s-join-%d", roomID, seat),
			Body:  &clientv1.Envelope_JoinRoomReq{JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: roomID}},
		}); err != nil {
			return nil, requests, err
		}
		requests++
		if _, err := client.readUntil(ctx, msgid.JoinRoomResp); err != nil {
			return nil, requests, err
		}
		clients = append(clients, client)
	}
	for seat, client := range clients {
		if err := client.send(ctx, msgid.ReadyReq, &clientv1.Envelope{
			ReqId: fmt.Sprintf("%s-ready-%d", roomID, seat),
			Body:  &clientv1.Envelope_ReadyReq{ReadyReq: &clientv1.ReadyRequest{}},
		}); err != nil {
			return nil, requests, err
		}
		requests++
		if _, err := client.readUntil(ctx, msgid.ReadyResp); err != nil {
			return nil, requests, err
		}
	}
	return clients, requests, nil
}

func (c *benchClient) send(ctx context.Context, msgID uint16, env *clientv1.Envelope) error {
	payload, err := proto.Marshal(env)
	if err != nil {
		return err
	}
	return c.conn.Write(ctx, websocket.MessageBinary, frame.Encode(msgID, payload))
}

func (c *benchClient) readUntil(ctx context.Context, wantMsgID uint16) (*clientv1.Envelope, error) {
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		msgType, data, err := c.conn.Read(deadline)
		if err != nil {
			return nil, err
		}
		if msgType != websocket.MessageBinary {
			continue
		}
		h, err := frame.ReadFrame(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if h.MsgID != wantMsgID {
			continue
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(h.Payload, &env); err != nil {
			return nil, err
		}
		return &env, nil
	}
}
