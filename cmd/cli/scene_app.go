package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
)

// RunSceneApp 启动新版全屏 TUI。它替代旧的 lobby 行式循环，让大厅、牌桌和结算
// 共用同一个 tcell 生命周期。
func RunSceneApp(ctx context.Context, switcher *TerminalSwitch, state *AppState, lobbyGW LobbyGateway, tableGW TableGateway, cfg *Config) error {
	scr, err := switcher.EnterFullscreen()
	if err != nil {
		return fmt.Errorf("打开终端失败: %w", err)
	}
	defer switcher.LeaveFullscreen()

	router := NewSceneRouter(state, lobbyGW, tableGW, cfg)
	eventCh := make(chan tcell.Event, 16)
	tcellCtx, cancelTcell := context.WithCancel(ctx)
	defer cancelTcell()
	go func() {
		for {
			if tcellCtx.Err() != nil {
				return
			}
			ev := scr.PollEvent()
			if ev == nil {
				return
			}
			select {
			case eventCh <- ev:
			case <-tcellCtx.Done():
				return
			}
		}
	}()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	router.Render(scr, time.Now())
	for {
		if done, err := router.Done(); done {
			return err
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
			router.Tick(ctx, time.Now())
			router.Render(scr, time.Now())
		case ev, ok := <-eventCh:
			if !ok {
				return nil
			}
			router.HandleEvent(ctx, ev, scr)
			router.Render(scr, time.Now())
		}
	}
}
