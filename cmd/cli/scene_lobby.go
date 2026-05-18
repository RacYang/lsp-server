package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"

	"racoo.cn/lsp/cmd/cli/render"
)

type lobbyMode int

const (
	lobbyModeHome lobbyMode = iota
	lobbyModeRules
	lobbyModePrivacy
	lobbyModeName
	lobbyModeJoinCode
	lobbyModeRooms
	lobbyModeHelp
)

// LobbyScene 是全屏大厅。
type LobbyScene struct {
	state *AppState
	gw    LobbyGateway

	mode       lobbyMode
	selected   int
	mu         sync.RWMutex
	message    string
	rules      []LobbyRuleMeta
	rooms      []LobbyRoomMeta
	nextPage   string
	private    bool
	roomName   string
	input      string
	pending    bool
	shouldQuit bool
}

func NewLobbyScene(state *AppState, gw LobbyGateway) *LobbyScene {
	return &LobbyScene{state: state, gw: gw}
}

func (s *LobbyScene) ID() SceneID      { return SceneLobby }
func (s *LobbyScene) ShouldQuit() bool { return s.shouldQuit }

func (s *LobbyScene) Render(scr tcell.Screen, now time.Time) {
	_ = now
	w, h := scr.Size()
	scr.Clear()
	page := render.CalcPage(w, h)

	view := s.state.Snapshot()
	render.DrawBandLine(scr, page.TitleBar, "lsp · 大厅 · "+view.Nickname, networkLabel(view), false, false)

	switch s.mode {
	case lobbyModeRules:
		s.renderRules(scr, page.Content)
	case lobbyModePrivacy:
		s.renderPrivacy(scr, page.Content)
	case lobbyModeName:
		s.renderName(scr, page.Content)
	case lobbyModeJoinCode:
		s.renderJoinCode(scr, page.Content)
	case lobbyModeRooms:
		s.renderRooms(scr, page.Content)
	case lobbyModeHelp:
		s.renderHelp(scr, page.Content)
	default:
		s.renderHome(scr, page.Content)
	}

	s.mu.RLock()
	msg := s.message
	s.mu.RUnlock()
	if msg != "" {
		render.DrawToast(scr, page.Toast, msg)
	}
	render.DrawBandLine(scr, page.KeyBar, s.keybar(), "", false, false)
}

func (s *LobbyScene) renderHome(scr tcell.Screen, content render.Region) {
	cards := []render.Card{
		{Title: "快速开始", Desc: "自动补齐机器人", Hint: "Enter"},
		{Title: "创建房间", Desc: "选择玩法开局", Hint: "Enter"},
		{Title: "加入房间码", Desc: "好友邀请进入", Hint: "Enter"},
		{Title: "公开房间", Desc: "挑选等候房", Hint: "Enter"},
	}
	cardW := 18
	total := len(cards)*cardW + (len(cards)-1)*2
	x := render.MaxInt(2, (content.Width-total)/2+content.X)
	y := render.MaxInt(3, content.Y+content.Height/2-5)
	render.DrawCardGrid(scr, x, y, cards, cardW, 6, s.selected)

	listY := y + 8
	render.DrawClippedText(scr, content.X+2, listY, render.Style(render.SemEmphasis), "公开房间", content.Width-4)
	for i, room := range s.rooms {
		if i >= 5 {
			break
		}
		line := fmt.Sprintf("%-18s  %-18s  %d/%d", displayRoomName(room), room.DisplayName, room.Players, room.Capacity)
		render.DrawClippedText(scr, content.X+4, listY+2+i, render.DefaultStyle(), line, content.Width-8)
	}
	if len(s.rooms) == 0 {
		render.DrawClippedText(scr, content.X+4, listY+2, render.DefaultStyle(), "暂无公开房，按 Enter 快速开始或创建房间", content.Width-8)
	}
}

func (s *LobbyScene) renderRules(scr tcell.Screen, content render.Region) {
	render.DrawClippedText(scr, content.X+2, content.Y+2, render.Style(render.SemEmphasis), "创建房间 · 选择玩法", content.Width-4)
	items := make([]render.ListItem, len(s.rules))
	for i, rule := range s.rules {
		desc := rule.ShortDesc
		if desc == "" {
			desc = strings.Join(rule.EnabledFeatures, " / ")
		}
		if desc == "" {
			desc = "经典玩法"
		}
		items[i] = render.ListItem{
			Text:     displayRuleName(rule) + "  ——  " + desc,
			Selected: i == s.selected,
		}
	}
	listRegion := render.Region{X: content.X + 6, Y: content.Y + 5, Width: render.MaxInt(30, content.Width-12), Height: content.Height - 6}
	render.DrawList(scr, listRegion, items, s.selected)
	if len(s.rules) == 0 {
		render.DrawClippedText(scr, content.X+4, content.Y+content.Height/2, render.DefaultStyle(), "正在加载玩法...", content.Width-8)
	}
}

func (s *LobbyScene) renderPrivacy(scr tcell.Screen, content render.Region) {
	render.DrawPanel(scr, content.Width, content.Height, "创建房间 · 公开性", []string{
		selectLabel(!s.private, "公开房间：出现在公开房列表"),
		selectLabel(s.private, "私密房间：创建后分享房间码"),
	})
}

func (s *LobbyScene) renderName(scr tcell.Screen, content render.Region) {
	name := s.input
	if name == "" {
		name = s.roomName
	}
	render.DrawPanel(scr, content.Width, content.Height, "创建房间 · 房间名", []string{
		"默认按 Enter 直接创建",
		"房间名：" + name,
	})
}

func (s *LobbyScene) renderJoinCode(scr tcell.Screen, content render.Region) {
	render.DrawPanel(scr, content.Width, content.Height, "加入房间码", []string{
		"请输入好友给你的房间码",
		"房间码：" + s.input,
	})
}

func (s *LobbyScene) renderRooms(scr tcell.Screen, content render.Region) {
	render.DrawClippedText(scr, content.X+2, content.Y+2, render.Style(render.SemEmphasis), "公开房间", content.Width-4)
	if len(s.rooms) == 0 {
		render.DrawClippedText(scr, content.X+4, content.Y+5, render.DefaultStyle(), "暂无公开房", content.Width-8)
		return
	}
	items := make([]render.ListItem, len(s.rooms))
	for i, room := range s.rooms {
		items[i] = render.ListItem{
			Text:     fmt.Sprintf("%-20s %-20s %d/%d", displayRoomName(room), room.DisplayName, room.Players, room.Capacity),
			Selected: i == s.selected,
		}
	}
	render.DrawList(scr, render.Region{X: content.X + 4, Y: content.Y + 6, Width: content.Width - 8, Height: content.Height - 7}, items, s.selected)
}

func (s *LobbyScene) renderHelp(scr tcell.Screen, content render.Region) {
	render.DrawPanel(scr, content.Width, content.Height, "帮助", []string{
		"大厅用方向键选择入口，Enter 确认",
		"创建房间先选玩法，再选公开性，最后确认房间名",
		"牌桌中 ←→ 选牌，Enter 出牌，? 查看帮助",
	})
}

// ─── 按键处理 ────────────────────────────────────────

func (s *LobbyScene) HandleKey(ctx context.Context, ev *tcell.EventKey) {
	s.mu.RLock()
	isPending := s.pending
	s.mu.RUnlock()
	if isPending {
		return
	}
	switch s.mode {
	case lobbyModeRules:
		s.handleRulesKey(ctx, ev)
	case lobbyModePrivacy:
		s.handlePrivacyKey(ev)
	case lobbyModeName:
		s.handleNameKey(ctx, ev)
	case lobbyModeJoinCode:
		s.handleJoinCodeKey(ctx, ev)
	case lobbyModeRooms:
		s.handleRoomsKey(ctx, ev)
	case lobbyModeHelp:
		s.handleHelpKey(ev)
	default:
		s.handleHomeKey(ctx, ev)
	}
}

func (s *LobbyScene) Tick(ctx context.Context, now time.Time) {
	_ = ctx
	_ = now
}

func (s *LobbyScene) handleHomeKey(ctx context.Context, ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyLeft, tcell.KeyUp:
		s.move(-1, 4)
	case tcell.KeyRight, tcell.KeyDown:
		s.move(1, 4)
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		switch s.selected {
		case 0:
			s.runAsync(ctx, "正在匹配...", func() error {
				_, err := s.gw.AutoMatch(ctx, "")
				return err
			})
		case 1:
			s.enterRules(ctx)
		case 2:
			s.mode = lobbyModeJoinCode
			s.input = ""
		case 3:
			s.enterRooms(ctx)
		}
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'q', 'Q':
			s.shouldQuit = true
		case '?':
			s.mode = lobbyModeHelp
		}
	}
}

func (s *LobbyScene) handleRulesKey(ctx context.Context, ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyLeft:
		s.move(-1, len(s.rules))
	case tcell.KeyDown, tcell.KeyRight:
		s.move(1, len(s.rules))
	case tcell.KeyEscape:
		s.mode = lobbyModeHome
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		if len(s.rules) == 0 {
			return
		}
		s.private = false
		s.roomName = defaultRoomDisplayName(s.rules[s.selected])
		s.mode = lobbyModePrivacy
	}
	_ = ctx
}

func (s *LobbyScene) handlePrivacyKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyLeft, tcell.KeyRight, tcell.KeyUp, tcell.KeyDown:
		s.private = !s.private
	case tcell.KeyEscape:
		s.mode = lobbyModeRules
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		s.input = s.roomName
		s.mode = lobbyModeName
	}
}

func (s *LobbyScene) handleNameKey(ctx context.Context, ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEscape:
		s.mode = lobbyModePrivacy
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		s.input = trimLastRune(s.input)
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		if len(s.rules) == 0 {
			return
		}
		rule := s.rules[s.selected]
		name := strings.TrimSpace(s.input)
		if name == "" {
			name = defaultRoomDisplayName(rule)
		}
		s.runAsync(ctx, "正在创建房间...", func() error {
			_, err := s.gw.CreateRoom(ctx, LobbyCreateOpts{RuleID: rule.RuleID, DisplayName: name, Private: s.private})
			return err
		})
	case tcell.KeyRune:
		s.input += string(ev.Rune())
	}
}

func (s *LobbyScene) handleJoinCodeKey(ctx context.Context, ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEscape:
		s.mode = lobbyModeHome
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		s.input = trimLastRune(s.input)
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		code := strings.TrimSpace(s.input)
		if len(code) < 4 || len(code) > 16 {
			s.mu.Lock()
			s.message = "房间码长度不正确"
			s.mu.Unlock()
			return
		}
		s.runAsync(ctx, "正在加入房间...", func() error {
			_, err := s.gw.JoinRoom(ctx, code)
			return err
		})
	case tcell.KeyRune:
		s.input += string(ev.Rune())
	}
}

func (s *LobbyScene) handleRoomsKey(ctx context.Context, ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEscape:
		s.mode = lobbyModeHome
	case tcell.KeyUp:
		s.move(-1, len(s.rooms))
	case tcell.KeyDown:
		s.move(1, len(s.rooms))
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		if len(s.rooms) == 0 {
			return
		}
		roomID := s.rooms[s.selected].RoomID
		s.runAsync(ctx, "正在加入房间...", func() error {
			_, err := s.gw.JoinRoom(ctx, roomID)
			return err
		})
	}
}

func (s *LobbyScene) handleHelpKey(ev *tcell.EventKey) {
	if ev.Key() == tcell.KeyEscape || ev.Key() == tcell.KeyEnter || ev.Key() == tcell.KeyCtrlJ {
		s.mode = lobbyModeHome
	}
}

func (s *LobbyScene) enterRules(ctx context.Context) {
	s.mode = lobbyModeRules
	s.selected = 0
	if len(s.rules) > 0 {
		return
	}
	s.runAsync(ctx, "正在加载玩法...", func() error {
		rules, err := s.gw.ListRules(ctx)
		if err != nil {
			s.rules = fallbackRules()
			return err
		}
		s.rules = rules
		return nil
	})
}

func (s *LobbyScene) enterRooms(ctx context.Context) {
	s.mode = lobbyModeRooms
	s.selected = 0
	s.runAsync(ctx, "正在刷新公开房...", func() error {
		list, err := s.gw.ListRooms(ctx, "")
		if err != nil {
			return err
		}
		s.rooms = list.Rooms
		s.nextPage = list.NextPageToken
		return nil
	})
}

func (s *LobbyScene) runAsync(ctx context.Context, pending string, fn func() error) {
	s.mu.Lock()
	s.pending = true
	s.message = pending
	s.mu.Unlock()
	go func() {
		err := fn()
		s.mu.Lock()
		s.pending = false
		if err != nil {
			s.message = err.Error()
		} else {
			s.message = ""
		}
		s.mu.Unlock()
	}()
	_ = ctx
}

func (s *LobbyScene) move(delta, n int) {
	if n <= 0 {
		s.selected = 0
		return
	}
	s.selected = (s.selected + delta + n) % n
}

func (s *LobbyScene) keybar() string {
	s.mu.RLock()
	isPending := s.pending
	s.mu.RUnlock()
	if isPending {
		return "请稍候..."
	}
	switch s.mode {
	case lobbyModeRules:
		return "↑↓ 选择玩法    Enter 下一步    Esc 返回"
	case lobbyModePrivacy:
		return "←→ 切换公开性    Enter 下一步    Esc 返回"
	case lobbyModeName:
		return "输入房间名    Enter 创建    Esc 返回"
	case lobbyModeJoinCode:
		return "输入房间码    Enter 加入    Esc 返回"
	case lobbyModeRooms:
		return "↑↓ 选择房间    Enter 加入    Esc 返回"
	case lobbyModeHelp:
		return "Enter / Esc 返回"
	default:
		return "←→ ↑↓ 选择    Enter 确认    ? 帮助    q 退出"
	}
}

func selectLabel(selected bool, text string) string {
	if selected {
		return "> " + text
	}
	return "  " + text
}

func trimLastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func fallbackRules() []LobbyRuleMeta {
	return []LobbyRuleMeta{
		{RuleID: "sichuan_xuezhandaodi_huansanzhang", DisplayName: "四川血战到底（换三张）", ShortDesc: "换三张、定缺、血战到底"},
		{RuleID: "sichuan_xuezhandaodi_biaozhun", DisplayName: "四川血战到底（标准）", ShortDesc: "定缺、血战到底"},
	}
}
