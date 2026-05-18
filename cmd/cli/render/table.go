package render

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
)

// ─── 渲染数据 ────────────────────────────────────────

// TileFace 是单张牌的可渲染数据。
type TileFace struct {
	Glyph string // 完整中文牌名，如"一万""东风"
	Rank  string // 数字/字牌名，如"一""东"
	Suit  string // 花色，如"万""筒""条"；字牌为空
}

// PlayerInfo 是单个玩家的展示数据（框外渲染）。
type PlayerInfo struct {
	Name      string
	SeatLabel string
	Status    string
	IsFocus   bool
	Que       string
	Melds     string
	Score     string
}

// CursorState 是手牌光标的可渲染状态。
type CursorState struct {
	Mode    string
	Index   int
	Marked  []int
	Pending bool
}

// TableData 是渲染牌桌需要的全部数据。
type TableData struct {
	Phase       string
	RoomLabel   string
	RuleLabel   string
	WallRemain  int
	RoundHand   string
	Players     [4]PlayerInfo
	Hands       [4][]TileFace
	Discards    [4][]TileFace
	Cursor      CursorState
	PhasePrompt string
	Countdown   int
	Events      []string
}

// ─── 主入口 ──────────────────────────────────────────

// DrawTableFrame 绘制四向对称牌桌：框 = 牌桌，框内只有牌和中央信息。
func DrawTableFrame(scr tcell.Screen, layout TableLayout, data TableData) {
	DrawTableEdge(scr, layout.Frame)
	DrawPlayerLabels(scr, layout, data)
	DrawSeatTiles(scr, layout, data)
	DrawPonds(scr, layout, data)
	DrawCenterDial(scr, layout.Center, data.PhasePrompt, data.Countdown)
}

// DrawTableEdge 绘制牌桌方框（硬编码 Unicode 框线）。
func DrawTableEdge(scr tcell.Screen, r Region) {
	if r.Width < 4 || r.Height < 4 {
		return
	}
	style := DefaultStyle()
	scr.SetContent(r.X, r.Y, '┌', nil, style)
	scr.SetContent(r.X+r.Width-1, r.Y, '┐', nil, style)
	for y := r.Y + 1; y < r.Y+r.Height-1; y++ {
		scr.SetContent(r.X, y, '│', nil, style)
		scr.SetContent(r.X+r.Width-1, y, '│', nil, style)
	}
	for x := r.X + 1; x < r.X+r.Width-1; x++ {
		scr.SetContent(x, r.Y, '─', nil, style)
		scr.SetContent(x, r.Y+r.Height-1, '─', nil, style)
	}
	scr.SetContent(r.X, r.Y+r.Height-1, '└', nil, style)
	scr.SetContent(r.X+r.Width-1, r.Y+r.Height-1, '┘', nil, style)
}

// DrawPlayerLabels 在框外四向绘制玩家名字、状态与分数。
func DrawPlayerLabels(scr tcell.Screen, layout TableLayout, data TableData) {
	// 南（框外下）
	south := data.Players[0]
	if south.Name != "" {
		label := fmt.Sprintf("你：%s %s 分：%s", south.Name, south.Status, south.Score)
		if data.WallRemain > 0 {
			label += fmt.Sprintf("  剩%d张", data.WallRemain)
		}
		DrawClippedText(scr, layout.SouthLabel.X, layout.SouthLabel.Y,
			Style(SemEmphasis), CenterVisual(label, layout.SouthLabel.Width), layout.SouthLabel.Width)
	}
	// 北（框外上）
	north := data.Players[2]
	if north.Name != "" {
		label := fmt.Sprintf("对家：%s %s 分：%s", north.Name, north.Status, north.Score)
		DrawClippedText(scr, layout.NorthLabel.X, layout.NorthLabel.Y,
			DefaultStyle(), CenterVisual(label, layout.NorthLabel.Width), layout.NorthLabel.Width)
	}
	// 西（框外左）
	west := data.Players[1]
	if west.Name != "" {
		label := fmt.Sprintf("%s %s", west.Name, west.Status)
		DrawText(scr, layout.WestLabel.X, layout.WestLabel.Y, DefaultStyle(), label)
	}
	// 东（框外右）
	east := data.Players[3]
	if east.Name != "" {
		label := fmt.Sprintf("%s %s", east.Name, east.Status)
		DrawText(scr, layout.EastLabel.X, layout.EastLabel.Y, DefaultStyle(), label)
	}
}

// ─── 手牌 ────────────────────────────────────────────

// DrawSeatTiles 绘制四家手牌。
func DrawSeatTiles(scr tcell.Screen, layout TableLayout, data TableData) {
	// 北家（对家）——一横排牌背
	drawHorizontalBacks(scr, layout.NorthHand, len(data.Hands[2]), layout.TileStep-2)
	// 西家——一竖排牌背
	drawVerticalBacks(scr, layout.WestWall, len(data.Hands[1]))
	// 东家——一竖排牌背
	drawVerticalBacks(scr, layout.EastWall, len(data.Hands[3]))
	// 南家（自己）——一横排竖排显示
	drawSouthVerticalHand(scr, layout.SouthHand, data.Hands[0], data.Cursor, layout.TileStep)
}

func drawSouthVerticalHand(scr tcell.Screen, r Region, tiles []TileFace, cursor CursorState, step int) {
	if r.Empty() || len(tiles) == 0 {
		return
	}
	if step < 2 {
		step = 2
	}
	for i, tile := range tiles {
		x := r.X + i*step
		if x+2 > r.X+r.Width {
			break
		}
		st := tileStyleForCursor(cursor, i)
		DrawVerticalTile(scr, x, r.Y, st, tile.Rank, tile.Suit)
	}
}

func drawHorizontalBacks(scr tcell.Screen, r Region, count, spacing int) {
	if r.Empty() || count <= 0 {
		return
	}
	DrawTileBackRow(scr, r.X, r.Y, DefaultStyle(), count, spacing)
}

func drawVerticalBacks(scr tcell.Screen, r Region, count int) {
	if r.Empty() || count <= 0 {
		return
	}
	DrawTileBackCol(scr, r.X, r.Y, DefaultStyle(), count, 0)
}

func tileStyleForCursor(cursor CursorState, idx int) tcell.Style {
	if cursor.Mode == "none" {
		return DefaultStyle()
	}
	cursorOn := cursor.Index == idx
	marked := false
	for _, m := range cursor.Marked {
		if m == idx {
			marked = true
			break
		}
	}
	if cursorOn && !cursor.Pending {
		return Style(SemEmphasis)
	}
	if marked && !cursor.Pending {
		return Style(SemWarning)
	}
	if cursor.Pending && (cursorOn || marked) {
		return Style(SemDim)
	}
	return DefaultStyle()
}

// ─── 牌河 ────────────────────────────────────────────

// DrawPonds 绘制四家牌河（手牌与中央之间）。
func DrawPonds(scr tcell.Screen, layout TableLayout, data TableData) {
	drawPondRow(scr, layout.NorthPond, data.Discards[2])
	drawPondRow(scr, layout.SouthPond, data.Discards[0])
	drawPondCol(scr, layout.WestPond, data.Discards[1])
	drawPondCol(scr, layout.EastPond, data.Discards[3])
}

func drawPondRow(scr tcell.Screen, r Region, tiles []TileFace) {
	if r.Empty() || len(tiles) == 0 {
		return
	}
	const step = 5 // 牌名 4 cell + 1 cell 间距，保证牌河可扫读。
	cols := (r.Width + 1) / step
	if cols < 1 {
		cols = 1
	}
	for i, tile := range tiles {
		row := i / cols
		col := i % cols
		if row >= r.Height {
			break
		}
		x := r.X + col*step
		y := r.Y + row
		DrawText(scr, x, y, DefaultStyle(), tile.Glyph)
	}
}

func drawPondCol(scr tcell.Screen, r Region, tiles []TileFace) {
	if r.Empty() || len(tiles) == 0 {
		return
	}
	const step = 5
	rows := r.Height
	if rows < 1 {
		rows = 1
	}
	for i, tile := range tiles {
		row := i % rows
		col := i / rows
		if col >= r.Width/step {
			break
		}
		x := r.X + col*step
		y := r.Y + row
		if x < r.X+r.Width && y < r.Y+r.Height {
			DrawText(scr, x, y, DefaultStyle(), tile.Glyph)
		}
	}
}

// ─── 中央信息 ────────────────────────────────────────

// DrawCenterDial 绘制中央区域：阶段提示 + 倒计时。
func DrawCenterDial(scr tcell.Screen, r Region, prompt string, countdown int) {
	if r.Empty() {
		return
	}
	if prompt != "" && r.Height >= 1 {
		DrawClippedText(scr, r.X, r.Y, Style(SemEmphasis),
			CenterVisual(prompt, r.Width), r.Width)
	}
	if countdown > 0 && r.Height >= 2 {
		cdLabel := fmt.Sprintf("%ds", countdown)
		cdSt := DefaultStyle()
		if countdown <= 5 {
			cdSt = Style(SemWarning)
		}
		DrawClippedText(scr, r.X, r.Y+1, cdSt,
			CenterVisual(cdLabel, r.Width), r.Width)
	}
}

// ─── 阶段化提示 ──────────────────────────────────────

// BuildSeatPrepLines 构建预备阶段座位展示行。
func BuildSeatPrepLines(players [4]PlayerInfo) []string {
	lines := make([]string, 0, 6)
	top := players[2]
	lines = append(lines, CenterVisual(prepPlayerLabel(top), 30))
	west := players[1]
	east := players[3]
	sideLine := fmt.Sprintf("%-14s %s", prepPlayerLabel(west), prepPlayerLabel(east))
	lines = append(lines, CenterVisual(sideLine, 30))
	bottom := players[0]
	lines = append(lines, CenterVisual(prepPlayerLabel(bottom), 30))
	return lines
}

func prepPlayerLabel(p PlayerInfo) string {
	if p.Name == "" {
		return fmt.Sprintf("%s %s 空座", p.Status, p.SeatLabel)
	}
	return fmt.Sprintf("%s %s %s", p.Status, p.SeatLabel, p.Name)
}

// BuildSettlementLines 构建结算展示行。
func BuildSettlementLines(title string, fans []string, totalFan int, scores []ScoreItem) []string {
	lines := []string{CenterVisual(title, 40)}
	for _, f := range fans {
		lines = append(lines, CenterVisual(f, 40))
	}
	lines = append(lines, CenterVisual(fmt.Sprintf("──── 共 %d 番 ────", totalFan), 40))
	for _, s := range scores {
		sign := ""
		if s.Value > 0 {
			sign = "+"
		}
		lines = append(lines, CenterVisual(fmt.Sprintf("%s   %s%d", s.Label, sign, s.Value), 40))
	}
	return lines
}

// ScoreItem 是计分项。
type ScoreItem struct {
	Label string
	Value int
}
