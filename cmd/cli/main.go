package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
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
		wsURL              = flag.String("ws", "wss://racoo.cn/ws", "gate WebSocket 地址")
		name               = flag.String("name", "终端玩家", "登录昵称")
		roomID             = flag.String("room", "", "启动后自动加入的房间")
		autoReady          = flag.Bool("auto-ready", false, "进房后自动发送 ready")
		tokenFile          = flag.String("token-file", filepath.Join(os.Getenv("HOME"), ".lsp", "session.token"), "会话令牌文件")
		origin             = flag.String("origin", "", "WebSocket Origin 头")
		insecureSkipVerify = flag.Bool("insecure-skip-verify", false, "wss 调试时跳过证书校验")
		cjkTiles           = flag.Bool("cjk-tiles", false, "使用中文花色牌面（需要等宽 CJK 字体）")
		noColor            = flag.Bool("no-color", false, "关闭牌张颜色")
		smokeDuration      = flag.Duration("smoke-duration", 0, "非交互冒烟时长，例如 5s；为 0 时启动 TUI")
		showVersion        = flag.Bool("version", false, "打印版本信息后退出")
	)
	flag.Parse()
	if *showVersion {
		fmt.Printf("lsp-cli %s commit=%s date=%s\n", version, commit, buildDate)
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	state := NewAppState(*name)
	state.Mutate(func(v *RoomView) { v.ServerURL = *wsURL })
	client := NewWSClient(*wsURL, *name, *tokenFile, *origin, *insecureSkipVerify, state)
	handler := NewCommandHandler(client, state)
	if *smokeDuration > 0 {
		if err := runSmoke(ctx, client, handler, state, *roomID, *autoReady, *smokeDuration); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "冒烟失败:", err)
			return 1
		}
		return 0
	}
	ui := NewUI(state, client, handler, RenderOptions{Width: 120, Height: 36, CJKTiles: *cjkTiles, NoColor: *noColor})

	if *roomID != "" {
		go client.Run(ctx)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case env := <-client.Events():
				state.Apply(env)
				if env.GetLoginResp() != nil && *roomID != "" {
					_ = handler.Handle(ctx, "join "+*roomID)
				} else if env.GetLoginResp() != nil {
					_ = handler.Handle(ctx, "list")
				}
				if (env.GetJoinRoomResp() != nil || env.GetAutoMatchResp() != nil || env.GetCreateRoomResp() != nil) && *autoReady {
					_ = handler.Handle(ctx, "ready")
				}
			}
		}
	}()

	if err := ui.Run(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "启动终端界面失败:", err)
		return 1
	}
	return 0
}

func runSmoke(ctx context.Context, client *WSClient, handler *CommandHandler, state *AppState, roomID string, autoReady bool, duration time.Duration) error {
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
			if roomID != "" && view.RoomID == "" {
				return fmt.Errorf("未完成进房")
			}
			fmt.Printf("smoke ok: user=%s room=%s seat=%d\n", view.UserID, view.RoomID, view.SeatIndex)
			return nil
		case env := <-client.Events():
			state.Apply(env)
			if env.GetLoginResp() != nil && roomID != "" {
				_ = handler.Handle(smokeCtx, "join "+roomID)
			}
			if env.GetJoinRoomResp() != nil && autoReady {
				_ = handler.Handle(smokeCtx, "ready")
			}
		}
	}
}
