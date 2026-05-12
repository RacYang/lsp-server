package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/app"
	serverconfig "racoo.cn/lsp/internal/config"
)

func TestSceneRouterPlaysOneRoundWithBots(t *testing.T) {
	if testing.Short() {
		t.Skip("完整 TUI 对局验收在 short 模式下跳过")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	serverCfg, err := serverconfig.Load("../../configs/dev.yaml")
	require.NoError(t, err)
	serverCfg.ServerAddr = "127.0.0.1:0"
	serverCfg.ObsAddr = ""
	serverApp, err := app.NewAllInProcess(ctx, serverCfg)
	require.NoError(t, err)
	go func() {
		_ = serverApp.Run(ctx)
	}()

	state := NewAppState("真人验收")
	cfg := &Config{
		Nickname:       "真人验收",
		ServerURL:      "ws://" + serverApp.Addr().String() + "/ws",
		TileTheme:      tileThemeASCII,
		ClaimTimeoutMS: 300,
	}
	tokenFile := t.TempDir() + "/session.token"
	client := NewWSClient(cfg.ServerURL, cfg.Nickname, tokenFile, "", false, state)
	bus := NewEventBus(state)
	go client.Run(ctx)
	go bus.Run(ctx, client.Events())
	require.NoError(t, waitForSession(ctx, state, 5*time.Second))

	lobbyGW := NewWSLobbyGateway(client, bus, state)
	tableGW := NewWSTableGateway(client, bus)
	router := NewSceneRouter(state, lobbyGW, tableGW, cfg)
	screen := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	screen.SetSize(145, 45)

	render := func() {
		router.Tick(ctx, time.Now())
		router.Render(screen, time.Now())
	}
	press := func(key *tcell.EventKey) {
		router.HandleEvent(ctx, key, screen)
		render()
	}
	pressRune := func(r rune) {
		press(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	pressEnter := func() {
		press(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	}
	pressRight := func() {
		press(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	}

	render()
	require.Equal(t, SceneLobby, router.CurrentSceneID())
	pressEnter() // 大厅默认选中“快速开始”，AutoMatch 会请求服务端补齐机器人。
	require.Eventually(t, func() bool {
		render()
		view := state.Snapshot()
		return view.Phase == phaseTable && view.RoomID != ""
	}, 5*time.Second, 20*time.Millisecond, "快速开始后没有进入房间，screen=%q", simulationScreenText(screen))
	initialView := state.Snapshot()
	selfUserID := initialView.UserID
	selfSeat := initialView.SeatIndex
	require.NotEmpty(t, selfUserID, "登录用户必须存在")
	require.GreaterOrEqual(t, selfSeat, int32(0), "进入牌桌后必须绑定座位")

	var (
		exchangeSubmitted bool
		queSubmitted      bool
		selfDiscards      int
		lastProgress      string
	)
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		render()
		view := state.Snapshot()
		lastProgress = fmt.Sprintf("scene=%s room_state=%s waiting=%s round_phase=%s acting=%d cursor={mode:%d index:%d pending:%v since:%d marked:%v} hand=%v discards=%v notice=%q log=%s screen=%q",
			router.CurrentSceneID(), view.RoomState, view.WaitingAction, view.RoundPhase.String(), view.ActingSeat,
			router.cursor.Mode, router.cursor.Index, router.cursor.Pending, router.cursor.PendingSinceStep, router.cursor.Marked,
			selfHand(view), selfDiscardsView(view), view.UXNotice, lastLogLine(view), simulationScreenText(screen))
		require.Equal(t, selfUserID, view.UserID, "对局中登录用户不应漂移，progress=%s", lastProgress)
		require.Equal(t, selfSeat, view.SeatIndex, "对局中客户端自我座位不应被推送事件改写，progress=%s", lastProgress)
		require.Equal(t, selfUserID, view.Players[selfSeat].UserID, "自我座位必须持续绑定登录用户，progress=%s", lastProgress)
		if view.LastSettlement != nil || router.CurrentSceneID() == SceneSettle {
			require.Positive(t, selfDiscards, "至少应完成一次真人出牌，progress=%s", lastProgress)
			return
		}

		switch router.CurrentSceneID() {
		case SceneRoomPrep:
			pressEnter()
		case SceneTable:
			switch view.WaitingAction {
			case "exchange_three":
				if !exchangeSubmitted {
					require.Len(t, selfHand(view), defaultStartingHandSize, "换三张必须基于自己的 13 张权威手牌，progress=%s", lastProgress)
					pressEnter()
					pressRight()
					pressEnter()
					pressRight()
					pressEnter()
					pressEnter()
					exchangeSubmitted = true
				}
			case "que_men":
				if !queSubmitted {
					pressRune('m')
					queSubmitted = true
				}
			case "discard":
				if view.SeatIndex == view.ActingSeat {
					// 渲染和网络事件是异步的；按键前再取一次快照，避免用已经过期的
					// "轮到我"画面断言后续手牌变化。
					current := state.Snapshot()
					if current.WaitingAction != "discard" || current.SeatIndex != current.ActingSeat {
						continue
					}
					before := len(selfHand(current))
					pressEnter()
					require.Eventually(t, func() bool {
						render()
						next := state.Snapshot()
						return len(selfHand(next)) < before ||
							next.WaitingAction != "discard" ||
							next.ActingSeat != next.SeatIndex ||
							strings.Contains(next.UXNotice, "出牌失败")
					}, 2*time.Second, 20*time.Millisecond, "出牌后手牌未同步，progress=%s", lastProgress)
					after := state.Snapshot()
					require.NotContains(t, after.UXNotice, "出牌失败", "出牌请求被拒绝，progress=%s", lastProgress)
					if len(selfHand(after)) == before-1 {
						selfDiscards++
					}
				}
			case "claim_window", "tsumo_window":
				pressRune('n')
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("真实 TUI 对局未能打到结算: %s", lastProgress)
}

func selfHand(view RoomView) []string {
	if view.SeatIndex < 0 || int(view.SeatIndex) >= len(view.Players) {
		return nil
	}
	return append([]string(nil), view.Players[view.SeatIndex].Hand...)
}

func selfDiscardsView(view RoomView) []string {
	if view.SeatIndex < 0 || int(view.SeatIndex) >= len(view.Players) {
		return nil
	}
	return append([]string(nil), view.Players[view.SeatIndex].Discards...)
}

func lastLogLine(view RoomView) string {
	if len(view.Log) == 0 {
		return ""
	}
	return view.Log[len(view.Log)-1].Text
}

func simulationScreenText(screen tcell.SimulationScreen) string {
	cells, w, h := screen.GetContents()
	lines := make([]string, 0, h)
	for y := 0; y < h; y++ {
		var b strings.Builder
		skipTail := false
		for x := 0; x < w; x++ {
			if skipTail {
				skipTail = false
				continue
			}
			cell := cells[y*w+x]
			mainc := ' '
			if len(cell.Runes) > 0 && cell.Runes[0] != 0 {
				mainc = cell.Runes[0]
			}
			if mainc == 0 {
				mainc = ' '
			}
			b.WriteRune(mainc)
			if uniseg.StringWidth(string(mainc)) >= 2 {
				skipTail = true
			}
		}
		line := strings.TrimRight(b.String(), " ")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
