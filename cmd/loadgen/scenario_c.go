//nolint:staticcheck // ADR-0025 指定 nhooyr.io/websocket 作为压测客户端依赖。
package main

import (
	"context"
	"fmt"
	"time"

	"nhooyr.io/websocket"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/msgid"
)

// runScenarioC 执行重连冲击剧本：稳态准备后断开 50% 连接，并统计 30 秒恢复结果。
func runScenarioC(ctx context.Context, cfg scenarioConfig) (scenarioSummary, error) {
	summary := scenarioSummary{
		Scenario:       "c",
		Version:        cfg.Version,
		Rooms:          cfg.Rooms,
		PlayersPerRoom: cfg.PlayersPerRoom,
		RoundCount:     cfg.RoundCount,
		RuntimeAdvice: []string{
			"若 30 秒内恢复率低于 95%，优先检查 Redis 会话 TTL 与 gate 重连路径",
			"若恢复期间限流升高，需结合 runtime.gate.ws_rate_limit_* 调整",
		},
	}
	recovered := 0
	totalReconnects := 0
	deadline := time.Now().Add(30 * time.Second)
	for roomIndex := 0; roomIndex < cfg.Rooms; roomIndex++ {
		roomID := fmt.Sprintf("%s-%d", cfg.RoomIDPrefix, roomIndex)
		clients, requests, err := prepareRoom(ctx, cfg, roomID)
		summary.Requests += requests
		if err != nil {
			summary.Errors++
			summary.Notes = append(summary.Notes, fmt.Sprintf("房间 %s 准备失败: %v", roomID, err))
			continue
		}
		cutoff := len(clients) / 2
		if cutoff == 0 && len(clients) > 0 {
			cutoff = 1
		}
		for i, client := range clients {
			if i >= cutoff {
				defer func(client *benchClient) {
					_ = client.conn.Close(websocket.StatusNormalClosure, "scenario c done")
				}(client)
				continue
			}
			totalReconnects++
			_ = client.conn.Close(websocket.StatusNormalClosure, "scenario c reconnect")
			if reconnectClient(ctx, cfg.WSURL, client.sessionToken) {
				recovered++
			} else {
				summary.Errors++
			}
			summary.Requests++
		}
	}
	summary.Notes = append(summary.Notes, fmt.Sprintf("30 秒恢复窗口截止于 %s，恢复 %d/%d", deadline.Format(time.RFC3339), recovered, totalReconnects))
	summary.Passed = summary.Errors == 0 && totalReconnects > 0 && recovered == totalReconnects
	return summary, nil
}

func reconnectClient(ctx context.Context, wsURL string, sessionToken string) bool {
	if sessionToken == "" {
		return false
	}
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return false
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "reconnected")
	}()
	client := &benchClient{conn: conn, sessionToken: sessionToken}
	if err := client.send(ctx, msgid.LoginReq, &clientv1.Envelope{
		ReqId: "reconnect-login",
		Body: &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{
			Nickname:     "压测重连玩家",
			SessionToken: sessionToken,
		}},
	}); err != nil {
		return false
	}
	env, err := client.readUntil(ctx, msgid.LoginResp)
	if err != nil {
		return false
	}
	return env.GetLoginResp().GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED
}
