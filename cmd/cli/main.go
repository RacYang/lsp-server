package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/pkg/logx"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		configPath         = flag.String("config", DefaultConfigPath(), "本地配置文件路径 (TOML)")
		nameFlag           = flag.String("name", "", "覆盖配置中的昵称")
		wsURLFlag          = flag.String("ws", "", "覆盖配置中的服务器 WebSocket 地址")
		origin             = flag.String("origin", "", "WebSocket Origin 头")
		insecureSkipVerify = flag.Bool("insecure-skip-verify", false, "wss 调试时跳过证书校验")
		smokeDuration      = flag.Duration("smoke-duration", 0, "非交互冒烟时长，例如 5s；为 0 时启动正常 UI")
		smokeRoom          = flag.String("room", "", "冒烟模式下自动加入的房间")
		showVersion        = flag.Bool("version", false, "打印版本信息后退出")
	)
	flag.Parse()
	if *showVersion {
		fmt.Printf("lsp-cli %s commit=%s date=%s\n", version, commit, buildDate)
		return 0
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取配置失败:", err)
	}
	applyFlagsToConfig(&cfg, *nameFlag, *wsURLFlag)

	if cfg.Nickname == "" && *smokeDuration == 0 {
		nickname, err := promptNickname(os.Stdin, os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取昵称失败:", err)
			return 1
		}
		if nickname == "" {
			fmt.Fprintln(os.Stderr, "昵称不能为空,退出")
			return 1
		}
		cfg.Nickname = nickname
	}
	if cfg.Nickname == "" {
		cfg.Nickname = "终端玩家"
	}

	tokenPath := filepath.Join(filepath.Dir(*configPath), "session.token")
	syncTokenFile(tokenPath, cfg.SessionToken)

	state := NewAppState(cfg.Nickname)
	state.Mutate(func(v *RoomView) { v.ServerURL = cfg.ServerURL })

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx = logx.WithTraceID(ctx, "cli")
	ctx = logx.WithUserID(ctx, cfg.Nickname)
	ctx = logx.WithRoomID(ctx, "")

	client := NewWSClient(cfg.ServerURL, cfg.Nickname, tokenPath, *origin, *insecureSkipVerify, state)
	var cfgMu sync.Mutex
	client.SetRedirectHandler(func(url string) error {
		cfgMu.Lock()
		defer cfgMu.Unlock()
		cfg.ServerURL = url
		client.SetConfig(url, "")
		state.Mutate(func(v *RoomView) {
			v.ServerURL = url
			v.Connected = false
			v.Reconnecting = true
			v.LastError = "服务端要求切换网关"
		})
		if *configPath != "" {
			return SaveConfig(*configPath, cfg)
		}
		return nil
	})

	if *smokeDuration > 0 {
		handler := NewCommandHandler(client, state)
		if err := runSmoke(ctx, client, handler, state, *smokeRoom, *smokeDuration); err != nil {
			fmt.Fprintln(os.Stderr, "冒烟失败:", err)
			return 1
		}
		return 0
	}

	if err := SilentLogin(ctx, client, &cfg, *configPath); err != nil {
		fmt.Fprintln(os.Stderr, "登录失败:", err)
		return 1
	}
	syncTokenFile(tokenPath, cfg.SessionToken)

	bus := NewEventBus(state)
	go client.Run(ctx)
	go bus.Run(ctx, client.Events())
	go runEmergencyAlerts(ctx, bus, os.Stderr)
	if err := waitForSession(ctx, state, 10*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "正式连接失败:", err)
		return 1
	}

	prompter := NewIOPrompter(os.Stdin, os.Stdout)
	switcher := NewTerminalSwitch()
	lobbyGW := NewWSLobbyGateway(client, bus, state)
	tableGW := NewWSTableGateway(client, bus, state.PhaseToken)

	_ = prompter // 旧行式 lobby 已被全屏 SceneRouter 替代，保留构造避免配置读写路径漂移。
	if err := RunSceneApp(ctx, switcher, state, lobbyGW, tableGW, &cfg); err != nil {
		if !errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "TUI 错误:", err)
		}
	}
	_ = SaveConfig(*configPath, cfg)
	return 0
}

func restartAfterSettlement(ctx context.Context, state *AppState, gw LobbyGateway) error {
	if state != nil {
		state.Mutate(func(v *RoomView) {
			resetRoomToLobby(v, true)
			v.PendingLeaveRoomID = ""
		})
	}
	leaveCtx, cancelLeave := context.WithTimeout(ctx, 3*time.Second)
	_ = gw.LeaveRoom(leaveCtx)
	cancelLeave()

	res, err := gw.AutoMatch(ctx, "")
	if err != nil {
		return err
	}
	applyJoinResultToState(state, res)
	return nil
}

func waitForSession(ctx context.Context, state *AppState, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("等待 LoginResp 超时")
		case <-ticker.C:
			view := state.Snapshot()
			if view.Connected && view.UserID != "" {
				return nil
			}
		}
	}
}

// applyFlagsToConfig 把命令行覆盖项落到配置上；空字符串保持原值。
func applyFlagsToConfig(cfg *Config, name, ws string) {
	if name != "" {
		cfg.Nickname = name
	}
	if ws != "" {
		cfg.ServerURL = ws
	}
}

// promptNickname 在 stdin 上问昵称，回车结束。
func promptNickname(in io.Reader, out io.Writer) (string, error) {
	_, _ = fmt.Fprint(out, "请输入昵称 > ")
	r := bufio.NewReader(in)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// syncTokenFile 把 cfg 中的 SessionToken 写到独立文件，
// 让 WSClient.login 沿用现有 tokenFile 读路径，无须改 conn.go。
func syncTokenFile(path, token string) {
	if path == "" {
		return
	}
	if token == "" {
		_ = os.Remove(path)
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte(token), 0o600)
}

// runEmergencyAlerts 后台 goroutine：把高优先级通知（被踢出/路由重定向/断线降级）
// 直接打到 stderr，让 lobby 阶段阻塞中的 stdin 也能立刻看到红字提示。
func runEmergencyAlerts(ctx context.Context, bus *EventBus, w io.Writer) {
	if bus == nil {
		return
	}
	id, ch := bus.Subscribe(func(env *clientv1.Envelope) bool {
		return env.GetRouteRedirect() != nil
	}, 8)
	defer bus.Unsubscribe(id)
	for {
		select {
		case <-ctx.Done():
			return
		case env, ok := <-ch:
			if !ok {
				return
			}
			if route := env.GetRouteRedirect(); route != nil {
				_, _ = fmt.Fprintf(w, "\n[!] 服务端要求重连到 %s (%s)\n", route.GetWsUrl(), route.GetReason())
			}
		}
	}
}

// snapshotSettlementSummary 把 state 中最新结算转成可读 SettlementSummary；
// 没有结算时返回 nil（lobby 不打印摘要）。血战规则下可能多个胜者，
// 这里把"自己是否在 winner 列表"作为 Win 判定依据。
func snapshotSettlementSummary(view RoomView) *SettlementSummary {
	if view.LastSettlement == nil {
		return nil
	}
	notify := view.LastSettlement
	sum := &SettlementSummary{
		RoomID:   view.RoomID,
		RuleID:   view.RuleID,
		TotalFan: int(notify.GetTotalFan()),
	}
	winners := notify.GetWinnerUserIds()
	switch {
	case len(winners) == 0:
		sum.Outcome = SettlementOutcomeDraw
	case containsString(winners, view.UserID):
		sum.Outcome = SettlementOutcomeWin
	default:
		sum.Outcome = SettlementOutcomeLose
	}
	for _, score := range notify.GetSeatScores() {
		entry := SettlementScore{
			Nickname: nicknameForSeat(view, score.GetSeatIndex()),
			Delta:    int(score.GetTotalFan()),
			IsSelf:   score.GetSeatIndex() == view.SeatIndex,
		}
		sum.Scores = append(sum.Scores, entry)
	}
	for _, breakdown := range notify.GetPerWinnerBreakdown() {
		if sum.WinnerID == "" {
			sum.WinnerID = breakdown.GetUserId()
			sum.WinnerNick = nicknameForSeat(view, breakdown.GetSeatIndex())
		}
		for _, name := range breakdown.GetFanNames() {
			sum.Fans = append(sum.Fans, SettlementFan{Name: name, Multiplier: 1})
		}
		if int(breakdown.GetFan()) > sum.TotalFan {
			sum.TotalFan = int(breakdown.GetFan())
		}
		// [S3.1] 多家胡：必须把每位胡家独立列出，而不是只保留第一个 winner。
		sum.Winners = append(sum.Winners, SettlementWinner{
			Nickname: nicknameForSeat(view, breakdown.GetSeatIndex()),
			IsSelf:   breakdown.GetSeatIndex() == view.SeatIndex,
			Fan:      int(breakdown.GetFan()),
			FanNames: append([]string(nil), breakdown.GetFanNames()...),
		})
	}
	// [S4.1] 流局或查叫罚分必须独立显示 reason / from / to / amount。
	for _, p := range notify.GetPenalties() {
		sum.Penalties = append(sum.Penalties, SettlementPenalty{
			Reason:   p.GetReason(),
			FromNick: nicknameForSeat(view, p.GetFromSeat()),
			ToNick:   nicknameForSeat(view, p.GetToSeat()),
			Amount:   int(p.GetAmount()),
		})
	}
	return sum
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func nicknameForSeat(view RoomView, seat int32) string {
	if seat < 0 || int(seat) >= len(view.Players) {
		return fmt.Sprintf("%d 号位", seat+1)
	}
	if name := view.Players[seat].Nickname; name != "" {
		return name
	}
	return fmt.Sprintf("%d 号位", seat+1)
}

// runSmoke 保留旧的非交互冒烟入口，CI 用它做最低限度的连接 + 登录验证。
func runSmoke(ctx context.Context, client *WSClient, handler *CommandHandler, state *AppState, roomID string, duration time.Duration) error {
	smokeCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	go client.Run(smokeCtx)
	for {
		select {
		case <-smokeCtx.Done():
			view := state.Snapshot()
			if view.UserID == "" {
				return fmt.Errorf("未完成登录")
			}
			fmt.Printf("smoke ok: user=%s room=%s seat=%d\n", view.UserID, view.RoomID, view.SeatIndex)
			return nil
		case env := <-client.Events():
			state.Apply(env)
			if env.GetLoginResp() != nil && roomID != "" {
				_ = handler.Handle(smokeCtx, "join "+roomID)
			}
		}
	}
}
