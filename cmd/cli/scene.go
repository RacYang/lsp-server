package main

import (
	"context"
	"time"

	"github.com/gdamore/tcell/v2"
)

// SceneID 标识当前全屏 TUI 的主场景。
type SceneID string

const (
	SceneLobby SceneID = "lobby"
	SceneTable SceneID = "table"
	SceneError SceneID = "error"
)

// Scene 是场景契约：渲染、按键、定时推进。
type Scene interface {
	ID() SceneID
	Render(scr tcell.Screen, now time.Time)
	HandleKey(ctx context.Context, ev *tcell.EventKey)
	Tick(ctx context.Context, now time.Time)
}

// SceneRouter 持有全屏 TUI 的共享状态，在大厅与牌桌间切换。
type SceneRouter struct {
	state   *AppState
	lobbyGW LobbyGateway
	tableGW TableGateway
	cfg     *Config

	lobby      *LobbyScene
	tableScene *TableScene

	frameLog *FrameLogger

	quit bool
	err  error
}

func (r *SceneRouter) SetFrameLog(l *FrameLogger) { r.frameLog = l }

func NewSceneRouter(state *AppState, lobbyGW LobbyGateway, tableGW TableGateway, cfg *Config) *SceneRouter {
	return &SceneRouter{
		state:      state,
		lobbyGW:    lobbyGW,
		tableGW:    tableGW,
		cfg:        cfg,
		lobby:      NewLobbyScene(state, lobbyGW),
		tableScene: NewTableScene(state, lobbyGW, tableGW, cfg),
	}
}

func (r *SceneRouter) CurrentSceneID() SceneID {
	view := r.state.Snapshot()
	if view.Phase == phaseTable && view.RoomID != "" {
		return SceneTable
	}
	return SceneLobby
}

func (r *SceneRouter) Render(scr tcell.Screen, now time.Time) {
	switch r.CurrentSceneID() {
	case SceneTable:
		r.tableScene.Render(scr, now)
	default:
		r.lobby.Render(scr, now)
	}
	scr.Show()
	if r.frameLog != nil {
		r.frameLog.Capture(r.CurrentSceneID(), r.state.Snapshot(), now)
	}
}

func (r *SceneRouter) HandleEvent(ctx context.Context, ev tcell.Event, scr tcell.Screen) {
	switch e := ev.(type) {
	case *tcell.EventResize:
		scr.Sync()
	case *tcell.EventKey:
		r.handleKey(ctx, e)
	}
}

func (r *SceneRouter) handleKey(ctx context.Context, ev *tcell.EventKey) {
	switch r.CurrentSceneID() {
	case SceneTable:
		r.tableScene.HandleKey(ctx, ev)
		if r.tableScene.ShouldQuit() {
			r.quit = true
			r.err = r.tableScene.Err()
		}
	default:
		r.lobby.HandleKey(ctx, ev)
		if r.lobby.ShouldQuit() {
			r.quit = true
		}
	}
}

func (r *SceneRouter) Tick(ctx context.Context, now time.Time) {
	switch r.CurrentSceneID() {
	case SceneTable:
		r.tableScene.Tick(ctx, now)
	default:
		r.lobby.Tick(ctx, now)
	}
}

func (r *SceneRouter) Done() (bool, error) { return r.quit, r.err }
