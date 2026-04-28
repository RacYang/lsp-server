//nolint:staticcheck // 压测客户端当前按 ADR-0025 使用 nhooyr.io/websocket 连接，后续若治理允许再替换。
package main

import (
	"context"
	"fmt"
	"sync"
)

// runScenarioB 执行大会话压力剧本，重点观察房间事件循环队列水位与 PostgreSQL 写入尾延迟。
// 场景不负责修改运行时参数，只把容量建议写入摘要，供后续人工回写配置。
func runScenarioB(ctx context.Context, cfg scenarioConfig) (scenarioSummary, error) {
	summary := scenarioSummary{
		Scenario:       "b",
		Version:        cfg.Version,
		Rooms:          cfg.Rooms,
		PlayersPerRoom: cfg.PlayersPerRoom,
		RoundCount:     cfg.RoundCount,
		RuntimeAdvice: []string{
			"若 lsp_actor_queue_depth p95 超过 80%，优先评估 runtime.room.mailbox_capacity",
			"若 append_event p99 超过 SLO，优先检查 PostgreSQL 连接池与存储退避",
		},
	}
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for roomIndex := 0; roomIndex < cfg.Rooms; roomIndex++ {
		wg.Add(1)
		go func(roomIndex int) {
			defer wg.Done()
			roomID := fmt.Sprintf("%s-%d", cfg.RoomIDPrefix, roomIndex)
			clients, requests, err := prepareRoom(ctx, cfg, roomID)
			mu.Lock()
			summary.Requests += requests
			if err != nil {
				summary.Errors++
				summary.Notes = append(summary.Notes, fmt.Sprintf("房间 %s 准备失败: %v", roomID, err))
				mu.Unlock()
				return
			}
			summary.Notes = append(summary.Notes, fmt.Sprintf("房间 %s 已完成大会话准备", roomID))
			mu.Unlock()
			for _, c := range clients {
				_ = c.conn.Close(1000, "scenario b done")
			}
		}(roomIndex)
	}
	wg.Wait()
	summary.Passed = summary.Errors == 0 && summary.Requests > 0
	return summary, nil
}
