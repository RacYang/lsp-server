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
	entries := []struct {
		title string
		desc  string
	}{
		{title: "快速开始", desc: "自动找一桌能开的牌局"},
		{title: "创建房间", desc: "选玩法，等朋友或机器人入座"},
		{title: "加入房间", desc: "输入好友给你的房间码"},
		{title: "公开房", desc: "看看正在等人的牌局"},
	}

	titleY := render.MaxInt(content.Y+2, content.Y+content.Height/2-8)
	render.DrawClippedText(scr, content.X, titleY, render.DefaultStyle(),
		render.CenterVisual("文字牌局", content.Width), content.Width)
	render.DrawClippedText(scr, content.X, titleY+2, render.Style(render.SemDim),
		render.CenterVisual("选一个入口，开始一桌川麻", content.Width), content.Width)

	menuY := titleY + 5
	total := 0
	labels := make([]string, len(entries))
	for i, entry := range entries {
		label := entry.title
		if i == s.selected {
			label = " " + entry.title + " "
		}
		labels[i] = label
		total += render.VisualWidth(label)
	}
	gap := 8
	total += gap * (len(entries) - 1)
	x := content.X + (content.Width-total)/2
	if x < content.X+2 {
		x = content.X + 2
	}
	for i, label := range labels {
		st := render.DefaultStyle()
		if i == s.selected {
			st = render.Style(render.SemEmphasis)
		}
		x = render.DrawText(scr, x, menuY, st, label)
		x += gap
	}
	if s.selected >= 0 && s.selected < len(entries) {
		render.DrawClippedText(scr, content.X, menuY+2, render.DefaultStyle(),
			render.CenterVisual(entries[s.selected].desc, content.Width), content.Width)
	}

	listY := menuY + 5
	render.DrawClippedText(scr, content.X+2, listY, render.Style(render.SemEmphasis), "公开房间", content.Width-4)
	for i, room := range s.rooms {
		if i >= 5 {
			break
		}
		line := fmt.Sprintf("%-18s  %-18s  %d/%d", displayRoomName(room), room.DisplayName, room.Players, room.Capacity)
		render.DrawClippedText(scr, content.X+4, listY+2+i, render.DefaultStyle(), line, content.Width-8)
	}
	if len(s.rooms) == 0 {
		render.DrawClippedText(scr, content.X+4, listY+2, render.Style(render.SemDim), "暂无公开房", content.Width-8)
	}
}

func (s *LobbyScene) renderRules(scr tcell.Screen, content render.Region) {
	y := drawLobbyHeading(scr, content, "创建房间", "先选一种玩法")
	if len(s.rules) == 0 {
		render.DrawClippedText(scr, content.X, y+3, render.DefaultStyle(),
			render.CenterVisual("正在加载玩法...", content.Width), content.Width)
		return
	}
	x, width := lobbyTextColumn(content, 72)
	for i, rule := range s.rules {
		desc := rule.ShortDesc
		if desc == "" {
			desc = strings.Join(rule.EnabledFeatures, " / ")
		}
		if desc == "" {
			desc = "经典玩法"
		}
		rowY := y + 3 + i*3
		if rowY+1 >= content.Y+content.Height {
			break
		}
		st := render.DefaultStyle()
		name := displayRuleName(rule)
		if i == s.selected {
			st = render.Style(render.SemEmphasis)
			name = " " + name + " "
		}
		render.DrawClippedText(scr, x, rowY, st, name, width)
		render.DrawClippedText(scr, x+2, rowY+1, render.Style(render.SemDim), desc, width-2)
	}
}

func (s *LobbyScene) renderPrivacy(scr tcell.Screen, content render.Region) {
	y := drawLobbyHeading(scr, content, "创建房间", "这桌让谁看见")
	x, width := lobbyTextColumn(content, 64)
	drawLobbyOption(scr, x, y+3, width, "公开房间", "出现在公开房列表，适合等人加入", !s.private)
	drawLobbyOption(scr, x, y+6, width, "私密房间", "创建后分享房间码，只给朋友加入", s.private)
}

func (s *LobbyScene) renderName(scr tcell.Screen, content render.Region) {
	name := s.input
	if name == "" {
		name = s.roomName
	}
	y := drawLobbyHeading(scr, content, "创建房间", "给这桌起个名字")
	x, width := lobbyTextColumn(content, 64)
	render.DrawClippedText(scr, x, y+3, render.Style(render.SemDim), "默认名字可以直接 Enter 接受", width)
	render.DrawClippedText(scr, x, y+5, render.DefaultStyle(), "房间名", width)
	render.DrawClippedText(scr, x, y+7, render.Style(render.SemEmphasis), " "+name+" ", width)
}

func (s *LobbyScene) renderJoinCode(scr tcell.Screen, content render.Region) {
	y := drawLobbyHeading(scr, content, "加入房间", "输入好友给你的房间码")
	x, width := lobbyTextColumn(content, 64)
	value := s.input
	if value == "" {
		value = "等待输入"
	}
	render.DrawClippedText(scr, x, y+4, render.DefaultStyle(), "房间码", width)
	render.DrawClippedText(scr, x, y+6, render.Style(render.SemEmphasis), " "+value+" ", width)
}

func (s *LobbyScene) renderRooms(scr tcell.Screen, content render.Region) {
	y := drawLobbyHeading(scr, content, "公开房", "选择一桌正在等人的牌局")
	x, width := lobbyTextColumn(content, 84)
	if len(s.rooms) == 0 {
		render.DrawClippedText(scr, x, y+4, render.Style(render.SemDim), "暂无公开房", width)
		return
	}
	for i, room := range s.rooms {
		rowY := y + 3 + i*2
		if rowY >= content.Y+content.Height {
			break
		}
		line := fmt.Sprintf("%-20s %-20s %d/%d", displayRoomName(room), room.DisplayName, room.Players, room.Capacity)
		st := render.DefaultStyle()
		if i == s.selected {
			st = render.Style(render.SemEmphasis)
			line = " " + strings.TrimSpace(line) + " "
		}
		render.DrawClippedText(scr, x, rowY, st, line, width)
	}
}

func (s *LobbyScene) renderHelp(scr tcell.Screen, content render.Region) {
	y := drawLobbyHeading(scr, content, "帮助", "当前只显示玩家需要的动作")
	x, width := lobbyTextColumn(content, 72)
	lines := []string{
		"大厅：方向键选择入口，Enter 确认",
		"创建房间：选玩法，选公开性，确认房间名",
		"牌局：看底部动作句，按当前状态操作",
	}
	for i, line := range lines {
		render.DrawClippedText(scr, x, y+3+i*2, render.DefaultStyle(), line, width)
	}
}

func drawLobbyHeading(scr tcell.Screen, content render.Region, title, subtitle string) int {
	y := render.MaxInt(content.Y+2, content.Y+content.Height/2-8)
	render.DrawClippedText(scr, content.X, y, render.Style(render.SemEmphasis),
		render.CenterVisual(title, content.Width), content.Width)
	if subtitle != "" {
		render.DrawClippedText(scr, content.X, y+2, render.Style(render.SemDim),
			render.CenterVisual(subtitle, content.Width), content.Width)
	}
	return y
}

func lobbyTextColumn(content render.Region, maxWidth int) (int, int) {
	width := maxWidth
	if width > content.Width-8 {
		width = content.Width - 8
	}
	if width < 24 {
		width = render.MaxInt(1, content.Width-2)
	}
	x := content.X + (content.Width-width)/2
	if x < content.X+1 {
		x = content.X + 1
	}
	return x, width
}

func drawLobbyOption(scr tcell.Screen, x, y, width int, title, desc string, selected bool) {
	st := render.DefaultStyle()
	label := title
	if selected {
		st = render.Style(render.SemEmphasis)
		label = " " + title + " "
	}
	render.DrawClippedText(scr, x, y, st, label, width)
	render.DrawClippedText(scr, x+2, y+1, render.Style(render.SemDim), desc, width-2)
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
		return "选玩法：↑↓ 移动　Enter 下一步　Esc 返回"
	case lobbyModePrivacy:
		return "选公开性：←→ 切换　Enter 下一步　Esc 返回"
	case lobbyModeName:
		return "房间名：输入文字　Enter 创建　Esc 返回"
	case lobbyModeJoinCode:
		return "加入房间：输入房间码　Enter 加入　Esc 返回"
	case lobbyModeRooms:
		return "公开房：↑↓ 选择　Enter 加入　Esc 返回"
	case lobbyModeHelp:
		return "帮助：Enter 返回　Esc 返回"
	default:
		return "大厅：←→/↑↓ 选择入口　Enter 确认　? 帮助　q 退出"
	}
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
		{RuleID: "guobiao_jingji_biaozhun", DisplayName: "国标麻将（竞技标准）", ShortDesc: "完整牌组、吃碰杠、花牌补花、8 分起胡"},
		{RuleID: "sichuan_xuezhandaodi_huansanzhang", DisplayName: "四川血战到底（换三张）", ShortDesc: "换三张、定缺、血战到底"},
		{RuleID: "sichuan_xuezhandaodi_biaozhun", DisplayName: "四川血战到底（标准）", ShortDesc: "定缺、血战到底"},
	}
}
