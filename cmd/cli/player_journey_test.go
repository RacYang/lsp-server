package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/app"
	serverconfig "racoo.cn/lsp/internal/config"
)

// TestPlayerJourneyAgainstRealBackend 是 docs/spec/player-journey.md v0.5 的端到端驱动器：
// 起一个进程内 cmd/all（与子进程方式等价：同一个 app.NewAllInProcess 工厂），让 cli 通过
// SimulationScreen 走完整旅程，断言全部以 spec 条款编号 [XX.X] 命名，便于回归失败时
// 直接锚回 spec 与 docs/spec/architecture-gaps.md 中的 AID。
//
// 帧 dump 落到 t.TempDir()/frames.jsonl，由 FrameLogger 写入，便于 drill 复盘。
//
// 这是 drill 工具而非 CI 闸门：默认 skip，仅在 LSP_JOURNEY_DRIVE=1 时启用，避免
// 把"架构缺陷待修复"的合法失败拖进每次 verify-fast。每个 AID 修复后，会产出
// 形如 TestPlayerJourney_<条款>_<场景> 的小颗粒回归（见 docs/spec/architecture-gaps.md），
// 那些才进 CI。
func TestPlayerJourneyAgainstRealBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("玩家旅程驱动器在 short 模式下跳过")
	}
	if os.Getenv("LSP_JOURNEY_DRIVE") != "1" {
		t.Skip("LSP_JOURNEY_DRIVE!=1，玩家旅程驱动器默认 skip，按需开启 (LSP_JOURNEY_DRIVE=1 go test -run TestPlayerJourneyAgainstRealBackend ./cmd/cli/...)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	serverCfg, err := serverconfig.Load("../../configs/dev.yaml")
	require.NoError(t, err)
	serverCfg.ServerAddr = "127.0.0.1:0"
	serverCfg.ObsAddr = ""
	serverApp, err := app.NewAllInProcess(ctx, serverCfg)
	require.NoError(t, err)
	go func() { _ = serverApp.Run(ctx) }()

	state := NewAppState("旅程驱动")
	cfg := &Config{
		Nickname:       "旅程驱动",
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

	framePath := filepath.Join(t.TempDir(), "frames.jsonl")
	t.Setenv("LSP_FRAME_LOG", framePath)
	frameLog := NewFrameLoggerFromEnv()
	require.NotNil(t, frameLog, "FrameLogger 应当能按 LSP_FRAME_LOG 打开 %s", framePath)
	router.SetFrameLog(frameLog)
	t.Cleanup(func() {
		_ = frameLog.Close()
		t.Logf("frame dump: %s", framePath)
	})

	screen := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, screen.Init())
	defer screen.Fini()
	screen.SetSize(145, 45)

	progress := &journeyProgress{}

	render := func() {
		router.Tick(ctx, time.Now())
		router.Render(screen, time.Now())
		progress.observe(t, router, state, screen)
	}
	press := func(key *tcell.EventKey) {
		router.HandleEvent(ctx, key, screen)
		render()
	}
	pressRune := func(r rune) { press(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)) }
	pressEnter := func() { press(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)) }
	pressRight := func() { press(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)) }

	render()
	require.Equal(t, SceneLobby, router.CurrentSceneID(), "[L2.1] 启动后应停在大厅主屏")

	pressEnter() // [L3.2] AutoMatch
	require.Eventually(t, func() bool {
		render()
		view := state.Snapshot()
		return view.Phase == phaseTable && view.RoomID != ""
	}, 5*time.Second, 20*time.Millisecond, "[L3.3] 快速开始未能在预算内进入房间，screen=%q", simulationScreenText(screen))

	initial := state.Snapshot()
	require.NotEmpty(t, initial.UserID, "[L1.3] 登录用户必须存在")
	require.GreaterOrEqual(t, initial.SeatIndex, int32(0), "[P1.1] 进房必须绑定座位")
	require.NotContains(t,
		[]string{"settling", "closed"}, initial.RoomState,
		"[L3.1] AutoMatch 不得把玩家匹入 settling/closed 的房，实际 RoomState=%q", initial.RoomState,
	)
	progress.firstTableState = initial.RoomState

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
		lastProgress = describeJourney(router, view, screen)
		require.Equal(t, initial.UserID, view.UserID, "[L1.3] 对局中登录用户不应漂移，progress=%s", lastProgress)
		require.Equal(t, initial.SeatIndex, view.SeatIndex, "[P1.1] 自我座位不得被推送事件改写，progress=%s", lastProgress)
		require.Equal(t, initial.UserID, view.Players[view.SeatIndex].UserID, "[P1.1] 自我座位必须持续绑定登录用户，progress=%s", lastProgress)

		if view.LastSettlement != nil || router.CurrentSceneID() == SceneSettle {
			require.Positive(t, selfDiscards, "[D2.3] 至少应完成一次真人出牌，progress=%s", lastProgress)
			progress.assertSpec(t, view, lastProgress)
			progress.assertFrameLog(t, framePath, lastProgress)
			return
		}

		switch router.CurrentSceneID() {
		case SceneRoomPrep:
			pressEnter()
		case SceneTable:
			switch view.WaitingAction {
			case "exchange_three":
				if !exchangeSubmitted {
					require.Len(t, selfHand(view), defaultStartingHandSize,
						"[E1.1] 换三张时必须看到 13 张权威手牌，progress=%s", lastProgress)
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
					}, 2*time.Second, 20*time.Millisecond, "[D2.3] 出牌后手牌未在 2s 内同步，progress=%s", lastProgress)
					after := state.Snapshot()
					require.NotContains(t, after.UXNotice, "出牌失败",
						"[D2.2/D2.3] 出牌请求被拒绝，progress=%s", lastProgress)
					if len(selfHand(after)) == before-1 {
						selfDiscards++
					}
				}
			case "claim_window", "tsumo_window":
				// 真人不抢答，显式 pass；服务端不得代选。
				pressRune('n')
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("[journey] 真实 TUI 对局未能在预算内打到结算: %s", lastProgress)
}

// journeyProgress 记录旅程沿途观察到的事实，便于结束后做 spec 级断言。
type journeyProgress struct {
	firstTableState       string
	sawExchangeThree      bool
	sawQueMen             bool
	sawDiscardWaiting     bool
	phaseDrawHasWaiting   bool // [D1.1] 违例：PHASE_DRAW 时 WaitingAction=="draw"
	managedFrameSeen      string
	managedScreenSampleAt string
}

func (p *journeyProgress) observe(t *testing.T, router *SceneRouter, state *AppState, screen tcell.SimulationScreen) {
	t.Helper()
	view := state.Snapshot()
	switch view.WaitingAction {
	case "exchange_three":
		p.sawExchangeThree = true
	case "que_men":
		p.sawQueMen = true
	case "discard":
		p.sawDiscardWaiting = true
	}
	if view.RoundPhase.String() == "PHASE_DRAW" && view.WaitingAction == "draw" {
		p.phaseDrawHasWaiting = true
	}
	text := simulationScreenText(screen)
	if p.managedFrameSeen == "" {
		// [G12] 座位状态值域仅限：● ○ ▲ ✓ ▣ □；任何 "托管" 字样或 ◐ 图标都视为违例。
		// 不在 observe() 里直接 fail：保留到 assertSpec() 统一裁决，避免短暂网络抖动期间
		// 假命中 ◐（cli 渲染层目前可能在某些过渡帧短暂出现）。
		offenders := []string{}
		if strings.Contains(text, "托管") {
			offenders = append(offenders, `"托管"`)
		}
		if strings.Contains(text, "◐") {
			offenders = append(offenders, "◐")
		}
		if len(offenders) > 0 {
			p.managedFrameSeen = text
			p.managedScreenSampleAt = fmt.Sprintf("offenders=%v scene=%s room_state=%s waiting=%s",
				offenders, router.CurrentSceneID(), view.RoomState, view.WaitingAction)
		}
	}
}

func (p *journeyProgress) assertSpec(t *testing.T, view RoomView, lastProgress string) {
	t.Helper()
	require.True(t, p.sawExchangeThree,
		"[E1.1] 必须曾经观察到 waiting_action=exchange_three，progress=%s", lastProgress)
	require.True(t, p.sawQueMen,
		"[Q1.1] 必须曾经观察到 waiting_action=que_men，progress=%s", lastProgress)
	require.True(t, p.sawDiscardWaiting,
		"[D2.1] 必须曾经观察到 waiting_action=discard，progress=%s", lastProgress)
	require.False(t, p.phaseDrawHasWaiting,
		"[D1.1] PHASE_DRAW 时 WaitingAction 不得被写入 \"draw\"，progress=%s", lastProgress)
	require.Empty(t, p.managedFrameSeen,
		"[G12] cli 帧文本不得出现「托管」字样或 ◐ 图标，sample=%s", p.managedScreenSampleAt)

	if settlement := view.LastSettlement; settlement != nil {
		var fanSum int32
		for _, s := range settlement.GetSeatScores() {
			fanSum += s.GetTotalFan()
		}
		require.Zero(t, fanSum,
			"[G14/S7.1] SettlementNotify.seat_scores 总和必须为 0，实际=%d", fanSum)
		for _, pn := range settlement.GetPenalties() {
			// 单条 PenaltyItem 自身就是 from-seat -> to-seat 的同额转移，
			// from+to 的算术和恒为 0；这里仅做存在性校验，避免误把缺字段当违例。
			require.NotEmpty(t, pn.GetReason(),
				"[S4.1] 罚分项必须显式带 reason 文案")
		}
	}
}

func (p *journeyProgress) assertFrameLog(t *testing.T, framePath, lastProgress string) {
	t.Helper()
	f, err := os.Open(framePath) //nolint:gosec // 路径来自 t.TempDir()
	if err != nil {
		t.Logf("[frame_log] 打开失败（驱动器允许 LSP_FRAME_LOG 未写盘）: %v", err)
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		totalFrames      int
		sceneCounts      = map[string]int{}
		waitingCounts    = map[string]int{}
		autoPlayOffender string
	)
	for scanner.Scan() {
		var rec frameRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		totalFrames++
		sceneCounts[rec.Scene]++
		if rec.WaitingAction != "" {
			waitingCounts[rec.WaitingAction]++
		}
		for _, seat := range rec.Seats {
			// 仅对非机器人座位断言 auto_play=false：bot 在服务端可能被打上 auto_play=true
			// （是机器人接管而非托管），cli 渲染层已通过 IsBot 分支吃掉，不会出现 ◐。
			// 但凡非 bot 的座位带 auto_play=true，都是 A1 / [G12] 的直接证据。
			if seat.AutoPlay && !seat.IsBot && autoPlayOffender == "" {
				autoPlayOffender = fmt.Sprintf("seat=%d user=%s nick=%s scene=%s", seat.Seat, seat.UserID, seat.Nickname, rec.Scene)
			}
		}
	}
	require.NoError(t, scanner.Err(), "[frame_log] 扫描失败，progress=%s", lastProgress)
	require.Positive(t, totalFrames, "[frame_log] FrameLogger 至少应写出一帧")
	require.Positive(t, sceneCounts[string(SceneLobby)], "[frame_log] 应至少观察到 lobby 帧")
	require.Positive(t, sceneCounts[string(SceneTable)], "[frame_log] 应至少观察到 table 帧")
	require.Positive(t, waitingCounts["discard"], "[D2.1] 帧 dump 中应包含 waiting_action=discard")
	require.Empty(t, autoPlayOffender,
		"[G12] frame dump 中非机器人座位不得带 auto_play=true offender=%s", autoPlayOffender)
}

func describeJourney(router *SceneRouter, view RoomView, screen tcell.SimulationScreen) string {
	return fmt.Sprintf("scene=%s room_state=%s waiting=%s round_phase=%s acting=%d hand=%v discards=%v notice=%q log=%s screen=%q",
		router.CurrentSceneID(), view.RoomState, view.WaitingAction, view.RoundPhase.String(), view.ActingSeat,
		selfHand(view), selfDiscardsView(view), view.UXNotice, lastLogLine(view), simulationScreenText(screen))
}
