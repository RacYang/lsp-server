package main

// OverlayKind 标识当前打开的叠加层。
type OverlayKind int

const (
	OverlayNone     OverlayKind = iota
	OverlayRoomInfo             // i 键：房间信息
	OverlayPlayers              // Tab 键：玩家详情
	OverlayMenu                 // Esc 键：局内菜单
	OverlayHelp                 // ? 键：快速参考
)

// OverlayMenuAction 是局内菜单项的触发动作。
type OverlayMenuAction string

const (
	OverlayMenuActionNone      OverlayMenuAction = ""
	OverlayMenuActionLeaveRoom OverlayMenuAction = "leave_room"
	OverlayMenuActionResume    OverlayMenuAction = "resume"
)

// OverlayState 维护当前叠加层种类与菜单选中项。
type OverlayState struct {
	Kind          OverlayKind
	SelectedIndex int
}

func (o *OverlayState) IsOpen() bool { return o.Kind != OverlayNone }

func (o *OverlayState) Toggle(kind OverlayKind) {
	if o.Kind == kind {
		o.Close()
		return
	}
	o.Kind = kind
	o.SelectedIndex = 0
}

func (o *OverlayState) Close() {
	o.Kind = OverlayNone
	o.SelectedIndex = 0
}

func (o *OverlayState) MenuMove(delta int) {
	if o.Kind != OverlayMenu {
		return
	}
	items := overlayMenuItems()
	n := len(items)
	if n == 0 {
		return
	}
	o.SelectedIndex = (o.SelectedIndex + delta + n) % n
}

func (o *OverlayState) MenuSelect() OverlayMenuAction {
	if o.Kind != OverlayMenu {
		return OverlayMenuActionNone
	}
	items := overlayMenuItems()
	if o.SelectedIndex < 0 || o.SelectedIndex >= len(items) {
		return OverlayMenuActionNone
	}
	return items[o.SelectedIndex].Action
}

type overlayMenuItem struct {
	Label  string
	Action OverlayMenuAction
}

func overlayMenuItems() []overlayMenuItem {
	return []overlayMenuItem{
		{Label: "返回大厅", Action: OverlayMenuActionLeaveRoom},
		{Label: "继续游戏", Action: OverlayMenuActionResume},
	}
}

// OverlayContext 给叠加层提供 RoomView 之外的辅助上下文。
type OverlayContext struct {
	RuleID string
}

// ─── 叠加层内容行 ────────────────────────────────────

func overlayRoomInfoLines(view RoomView) []string {
	roomID := view.RoomID
	if roomID == "" {
		roomID = "(未进房)"
	}
	count := 0
	for _, p := range view.Players {
		if p.UserID != "" {
			count++
		}
	}
	return []string{
		"房号: " + roomID,
		"规则: " + ruleLabel(view),
		"人数: " + itoa(count) + " / 4",
		"",
		"按 i 关闭",
	}
}

func overlayPlayersLines(view RoomView) []string {
	lines := make([]string, 0, len(view.Players)+2)
	for i, p := range view.Players {
		nickname := p.Nickname
		if nickname == "" && p.UserID == "" {
			nickname = "(空座)"
		} else if nickname == "" {
			nickname = p.UserID
		}
		mark := " "
		if i == int(view.SeatIndex) {
			mark = "★"
		}
		ready := ""
		if p.Ready {
			ready = "  ✓ ready"
		}
		if p.IsBot {
			ready += "  [BOT]"
		}
		if p.Surrendered {
			ready += "  ▲ 弃局"
		}
		if p.Hued {
			ready += "  ✓ 已胡"
		}
		lines = append(lines, " "+mark+" "+itoa(i+1)+" 号位  "+nickname+"  手:"+itoa(p.HandCnt)+ready)
	}
	lines = append(lines, "")
	lines = append(lines, "按 Tab 关闭")
	return lines
}

func overlayHelpLines(view RoomView) []string {
	phase := DerivePhase(view, nil)
	lines := []string{
		"←→ 选牌    Enter 出牌 / 确认",
		"换三张: Space 标记三张，Enter 提交换牌",
		"定缺: m 缺万 / p 缺筒 / s 缺条",
		"碰杠胡: p 碰 / g 杠 / h 胡 / n 过",
		"机器人: waiting 阶段 b 补一个 / B 补满",
		"信息: i 房间信息 / Tab 玩家详情",
		"离桌: q 返回大厅 / Esc 菜单",
		"",
		"按 ? 或Enter关闭",
	}
	switch phase {
	case PhaseClaim:
		lines = append([]string{"当前模式: -- 鸣牌 --", "h 胡 / g 杠 / p 碰 / n 过 / Enter确认", ""}, lines...)
	case PhaseExchange:
		lines = append([]string{"当前模式: -- 换三张 --", "Enter标记三张 / Enter提交", ""}, lines...)
	case PhaseSettlement:
		lines = append([]string{"当前模式: -- 结算 --", "R 再来一局 / L 离桌 / Enter停留", ""}, lines...)
	}
	return lines
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
