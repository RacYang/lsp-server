package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
)

type lobbyMode int

const (
	lobbyModeHome lobbyMode = iota
	lobbyModeRules
	lobbyModePrivacy
	lobbyModeName
	lobbyModeJoinCode
	lobbyModeRooms
	lobbyModeSettings
	lobbyModeHelp
)

// LobbyScene 是全屏大厅。它用卡片和向导替代旧的 stdin/stdout 菜单。
type LobbyScene struct {
	state *AppState
	gw    LobbyGateway
	cfg   *Config

	mode       lobbyMode
	selected   int
	mu         sync.RWMutex // 保护 message 与 pending（runAsync 后台写，Render ticker 读）
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

func NewLobbyScene(state *AppState, gw LobbyGateway, cfg *Config) *LobbyScene {
	return &LobbyScene{state: state, gw: gw, cfg: cfg}
}

func (s *LobbyScene) ID() SceneID { return SceneLobby }

func (s *LobbyScene) ShouldQuit() bool { return s.shouldQuit }

func (s *LobbyScene) Render(scr tcell.Screen, now time.Time) {
	_ = now
	scr.Clear()
	w, h := scr.Size()
	view := s.state.Snapshot()
	drawBandLine(scr, Region{X: 0, Y: 0, Width: w, Height: 1}, "lsp · 大厅 · "+view.Nickname, networkLabel(view), false, false)
	switch s.mode {
	case lobbyModeRules:
		s.renderRules(scr, w, h)
	case lobbyModePrivacy:
		s.renderPrivacy(scr, w, h)
	case lobbyModeName:
		s.renderName(scr, w, h)
	case lobbyModeJoinCode:
		s.renderJoinCode(scr, w, h)
	case lobbyModeRooms:
		s.renderRooms(scr, w, h)
	case lobbyModeSettings:
		s.renderSettings(scr, w, h)
	case lobbyModeHelp:
		s.renderHelp(scr, w, h)
	default:
		s.renderHome(scr, w, h)
	}
	s.mu.RLock()
	msg := s.message
	s.mu.RUnlock()
	if msg != "" {
		drawClippedText(scr, 2, h-2, defaultStyle().Reverse(true), msg, w-4)
	}
	drawBandLine(scr, Region{X: 0, Y: h - 1, Width: w, Height: 1}, s.keybar(), "", false, false)
}

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
	case lobbyModeSettings:
		s.handleSettingsKey(ev)
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

func (s *LobbyScene) renderHome(scr tcell.Screen, w, h int) {
	cards := []string{"快速开始", "创建房间", "加入房间码", "公开房间"}
	desc := []string{"自动补齐机器人", "选择玩法开局", "好友邀请进入", "挑选等待房"}
	cardW := 18
	total := len(cards)*cardW + (len(cards)-1)*2
	x := renderMaxInt(2, (w-total)/2)
	y := renderMaxInt(3, h/2-5)
	for i, title := range cards {
		r := Region{X: x + i*(cardW+2), Y: y, Width: cardW, Height: 6}
		style := defaultStyle()
		if s.selected == i {
			style = style.Reverse(true)
		}
		drawSimpleBox(scr, r, title)
		drawClippedText(scr, r.X+2, r.Y+2, style, centerVisual(desc[i], r.Width-4), r.Width-4)
		drawClippedText(scr, r.X+2, r.Y+4, style, centerVisual("Enter", r.Width-4), r.Width-4)
	}
	drawClippedText(scr, 2, y+8, defaultStyle(), "公开房间", w-4)
	for i, room := range s.rooms {
		if i >= 5 {
			break
		}
		line := fmt.Sprintf("%-18s  %-18s  %d/%d", displayRoomName(room), room.DisplayName, room.Players, room.Capacity)
		drawClippedText(scr, 4, y+10+i, defaultStyle(), line, w-8)
	}
	if len(s.rooms) == 0 {
		drawClippedText(scr, 4, y+10, defaultStyle(), "暂无公开房，按 Enter 快速开始或创建房间。", w-8)
	}
}

func (s *LobbyScene) renderRules(scr tcell.Screen, w, h int) {
	drawClippedText(scr, 2, 2, defaultStyle().Bold(true), "创建房间 · 选择玩法", w-4)
	y := 5
	for i, rule := range s.rules {
		box := Region{X: 6, Y: y + i*5, Width: renderMaxInt(30, w-12), Height: 4}
		drawSimpleBox(scr, box, displayRuleName(rule))
		style := defaultStyle()
		if s.selected == i {
			style = style.Reverse(true)
		}
		desc := rule.ShortDesc
		if desc == "" {
			desc = strings.Join(rule.EnabledFeatures, " / ")
		}
		if desc == "" {
			desc = "经典玩法"
		}
		drawClippedText(scr, box.X+2, box.Y+2, style, desc, box.Width-4)
	}
	if len(s.rules) == 0 {
		drawClippedText(scr, 4, h/2, defaultStyle(), "正在加载玩法...", w-8)
	}
}

func (s *LobbyScene) renderPrivacy(scr tcell.Screen, w, h int) {
	drawCenteredPanel(scr, w, h, "创建房间 · 公开性", []string{
		selectLabel(!s.private, "公开房间: 出现在公开房列表"),
		selectLabel(s.private, "私密房间: 创建后分享房间码"),
	})
}

func (s *LobbyScene) renderName(scr tcell.Screen, w, h int) {
	name := s.input
	if name == "" {
		name = s.roomName
	}
	drawCenteredPanel(scr, w, h, "创建房间 · 房间名", []string{
		"默认按 Enter 直接创建。",
		"房间名: " + name,
	})
}

func (s *LobbyScene) renderJoinCode(scr tcell.Screen, w, h int) {
	drawCenteredPanel(scr, w, h, "加入房间码", []string{
		"请输入好友给你的房间码。",
		"房间码: " + s.input,
	})
}

func (s *LobbyScene) renderRooms(scr tcell.Screen, w, h int) {
	drawClippedText(scr, 2, 2, defaultStyle().Bold(true), "公开房间", w-4)
	if len(s.rooms) == 0 {
		drawClippedText(scr, 4, 5, defaultStyle(), "暂无公开房。", w-8)
		return
	}
	drawClippedText(scr, 4, 4, defaultStyle().Bold(true), "房间名                 玩法                 人数", w-8)
	for i, room := range s.rooms {
		style := defaultStyle()
		if i == s.selected {
			style = style.Reverse(true)
		}
		line := fmt.Sprintf("%-20s %-20s %d/%d", displayRoomName(room), room.DisplayName, room.Players, room.Capacity)
		drawClippedText(scr, 4, 6+i, style, line, w-8)
	}
}

func (s *LobbyScene) renderSettings(scr tcell.Screen, w, h int) {
	theme := ""
	if s.cfg != nil {
		theme = ParseTileTheme(s.cfg.TileTheme).String()
	}
	drawCenteredPanel(scr, w, h, "设置", []string{
		"牌面主题: " + theme,
		"按 Enter 在 unicode / ascii 间切换。",
	})
}

func (s *LobbyScene) renderHelp(scr tcell.Screen, w, h int) {
	drawCenteredPanel(scr, w, h, "帮助", []string{
		"大厅用方向键选择入口，Enter 确认。",
		"创建房间先选玩法，再选公开性，最后确认房间名。",
		"牌桌中 ←→ 选牌，Enter 出牌，? 查看帮助。",
	})
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
		case 's', 'S':
			s.mode = lobbyModeSettings
		case '?':
			s.mode = lobbyModeHelp
		case 'n', 'N':
			s.mode = lobbyModeJoinCode
			s.mu.Lock()
			s.message = "改名暂沿用配置，下个版本进入独立输入框"
			s.mu.Unlock()
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

func (s *LobbyScene) handleSettingsKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEscape:
		s.mode = lobbyModeHome
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		if s.cfg == nil {
			return
		}
		theme := ParseTileTheme(s.cfg.TileTheme)
		if theme == TileThemeUnicode {
			s.cfg.TileTheme = TileThemeASCII.String()
		} else {
			s.cfg.TileTheme = TileThemeUnicode.String()
		}
		s.mu.Lock()
		s.message = "牌面主题已切换为 " + s.cfg.TileTheme
		s.mu.Unlock()
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
	case lobbyModeSettings:
		return "Enter 切换主题    Esc 返回"
	case lobbyModeHelp:
		return "Enter/Esc 返回"
	default:
		return "←→/↑↓ 选择    Enter 确认    s 设置    ? 帮助    q 退出"
	}
}

func drawCenteredPanel(scr tcell.Screen, w, h int, title string, lines []string) {
	width := renderMaxInt(40, w/2)
	if width > w-4 {
		width = w - 4
	}
	height := len(lines) + 4
	box := Region{X: renderMaxInt(0, (w-width)/2), Y: renderMaxInt(2, (h-height)/2), Width: width, Height: height}
	drawSimpleBox(scr, box, title)
	for i, line := range lines {
		drawClippedText(scr, box.X+2, box.Y+2+i, defaultStyle(), line, box.Width-4)
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
