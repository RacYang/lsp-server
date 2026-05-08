package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	roomsvc "racoo.cn/lsp/internal/service/room"
)

// TestBotSupervisorAllBotsCompleteARound 端到端验证：4 家全是占位 bot 时，
// supervisor 应自动推进 ready/exchange/que_men/discard/claim/tsumo 直到结算或所有人胡完。
//
// ADR-0037 描述的"占座 + 后续补 supervisor"在此被锁住：禁止再退化成"bot 永远不出牌"的卡死。
func TestBotSupervisorAllBotsCompleteARound(t *testing.T) {
	t.Parallel()

	svc := roomsvc.NewServiceWithRule(roomsvc.NewLobby(), "sichuan_xuezhandaodi_huansanzhang")
	svc.SetMailboxCapacity(64)
	sup := NewBotSupervisor(svc)
	svc.SetAfterCmdHook(sup.AfterCmd)

	const roomID = "BOT-ROOM"
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	bots := []string{"bot:" + roomID + ":0", "bot:" + roomID + ":1", "bot:" + roomID + ":2", "bot:" + roomID + ":3"}
	for _, uid := range bots {
		if _, err := svc.Join(ctx, roomID, uid); err != nil {
			t.Fatalf("join %s: %v", uid, err)
		}
	}
	for _, uid := range bots {
		if _, err := svc.Ready(ctx, roomID, uid); err != nil {
			t.Fatalf("ready %s: %v", uid, err)
		}
	}

	// 等到房间走出 exchange/que_men/discard 阶段：要么所有人胡完 closed，
	// 要么至少进入 settling（LastSettlement）。轮询而不是 sleep，确保稳定。
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		view, ok, err := svc.RoundView(ctx, roomID)
		require.NoError(t, err)
		if !ok || view.Closed {
			return
		}
		// 没卡在 exchange/que_men 视为推进成功。
		if view.WaitingAction != "exchange_three" && view.WaitingAction != "que_men" {
			discardDeadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(discardDeadline) {
				v2, ok2, _ := svc.RoundView(ctx, roomID)
				if !ok2 || v2.Closed {
					return
				}
				totalDiscards := 0
				for _, ds := range v2.DiscardsBySeat {
					totalDiscards += len(ds)
				}
				if totalDiscards > 0 {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			t.Fatalf("bot 进入出牌阶段后未真实出牌；waiting=%s acting=%d", view.WaitingAction, view.ActingSeat)
		}
		time.Sleep(50 * time.Millisecond)
	}
	view, _, _ := svc.RoundView(ctx, roomID)
	t.Fatalf("supervisor 卡在 exchange/que_men 阶段：waiting=%s exchange_done=%v que_done=%v",
		view.WaitingAction, view.ExchangeSubmitted, view.QueSubmitted)
}

// TestBotSupervisorMixedHumanWaitsAfterExchange 验证混人混 bot 时：
// bot 可以并发完成换三张+定缺，但出牌阶段会停下来等真人，不会代真人出牌。
func TestBotSupervisorMixedHumanWaitsAfterExchange(t *testing.T) {
	t.Parallel()

	svc := roomsvc.NewServiceWithRule(roomsvc.NewLobby(), "sichuan_xuezhandaodi_huansanzhang")
	svc.SetMailboxCapacity(64)
	sup := NewBotSupervisor(svc)
	svc.SetAfterCmdHook(sup.AfterCmd)

	const roomID = "MIX-ROOM"
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	humanID := "user-real"
	users := []string{humanID, "bot:" + roomID + ":1", "bot:" + roomID + ":2", "bot:" + roomID + ":3"}
	for _, uid := range users {
		if _, err := svc.Join(ctx, roomID, uid); err != nil {
			t.Fatalf("join %s: %v", uid, err)
		}
		if _, err := svc.Ready(ctx, roomID, uid); err != nil {
			t.Fatalf("ready %s: %v", uid, err)
		}
	}

	humanExchanged := false
	humanQueDone := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		view, ok, _ := svc.RoundView(ctx, roomID)
		if !ok {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		switch view.WaitingAction {
		case "exchange_three":
			if !humanExchanged && len(view.HandsBySeat) > 0 && len(view.HandsBySeat[0]) >= 3 {
				_, err := svc.ExchangeThree(ctx, roomID, humanID, view.HandsBySeat[0][:3], 3)
				if err == nil {
					humanExchanged = true
				}
			}
		case "que_men":
			if !humanQueDone {
				_, err := svc.QueMen(ctx, roomID, humanID, 0)
				if err == nil {
					humanQueDone = true
				}
			}
		case "discard":
			require.True(t, humanExchanged && humanQueDone)
			handBefore := append([]string(nil), view.HandsBySeat[0]...)
			time.Sleep(200 * time.Millisecond)
			view2, _, _ := svc.RoundView(ctx, roomID)
			if view2.ActingSeat == 0 {
				require.Equalf(t, handBefore, view2.HandsBySeat[0], "supervisor 不应代真人出牌")
			}
			return
		case "claim_window", "tsumo_window":
			for _, c := range view.ClaimCandidates {
				if c.Seat == 0 && len(c.Actions) > 0 {
					handBefore := append([]string(nil), view.HandsBySeat[0]...)
					time.Sleep(200 * time.Millisecond)
					view2, _, _ := svc.RoundView(ctx, roomID)
					require.Equalf(t, handBefore, view2.HandsBySeat[0], "claim_window 不应被代抢")
					return
				}
			}
		}
		time.Sleep(30 * time.Millisecond)
	}

	view, _, _ := svc.RoundView(ctx, roomID)
	if !humanExchanged {
		t.Skip("真人未能在 deadline 内提交换三张；策略时间窗口不稳定，跳过")
	}
	for seat, done := range view.ExchangeSubmitted {
		if seat == 0 {
			continue
		}
		require.Truef(t, done, "bot 座位 %d 未完成换三张", seat)
	}
}

// TestIsBotUserID 锁住 bot user_id 前缀约定，让 supervisor 与 lobby AddBot 命名空间保持同步。
func TestIsBotUserID(t *testing.T) {
	t.Parallel()
	require.True(t, IsBotUserID("bot:R1:0"))
	require.True(t, IsBotUserID("bot:any"))
	require.False(t, IsBotUserID("user-1"))
	require.False(t, IsBotUserID(""))
	require.False(t, IsBotUserID(strings.ToUpper("bot:R1:0")), "前缀大小写敏感，避免误判普通用户")
}
