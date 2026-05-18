package render

// Region 是屏幕上的一个矩形区域。
type Region struct {
	X, Y          int
	Width, Height int
}

// Empty 表示矩形是否退化为空。
func (r Region) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

// MinTableWidth / MinTableHeight 是牌桌全屏渲染的最小可工作尺寸。
const (
	MinTableWidth  = 100
	MinTableHeight = 30

	RecommendedTableWidth  = 140
	RecommendedTableHeight = 40
)

// Page 是全局页面模板，所有场景共用。
type Page struct {
	TitleBar Region
	Content  Region
	Toast    Region
	KeyBar   Region
}

// CalcPage 按屏幕尺寸计算全局页面骨架。
func CalcPage(w, h int) Page {
	return Page{
		TitleBar: Region{X: 0, Y: 0, Width: w, Height: 1},
		KeyBar:   Region{X: 0, Y: h - 1, Width: w, Height: 1},
		Toast:    Region{X: 0, Y: h - 2, Width: w, Height: 1},
		Content:  Region{X: 0, Y: 1, Width: w, Height: h - 3},
	}
}

const (
	wallTileCount = 13
	maxHandTiles  = 14
)

// TableLayout 描述四向对称牌桌的全部区域。
// 框 = 牌桌，框内只有牌和中央信息；玩家信息在框外紧贴。
type TableLayout struct {
	Width, Height int

	Frame      Region
	Inner      Region
	Compact    bool
	TileStep   int // 牌间距步长（宽松=3，紧凑=2）
	HandWidth  int // 手牌横排宽度
	HandHeight int // 手牌竖排高度（2 行）

	NorthHand  Region
	NorthPond  Region
	NorthLabel Region
	WestWall   Region
	WestPond   Region
	WestLabel  Region
	EastWall   Region
	EastPond   Region
	EastLabel  Region
	SouthHand  Region
	SouthPond  Region
	SouthLabel Region
	Center     Region
}

// CalcTable 根据屏幕尺寸计算四向对称牌桌布局。
func CalcTable(w, h int) (TableLayout, bool) {
	if w < MinTableWidth || h < MinTableHeight {
		return TableLayout{}, false
	}

	compact := w < 120
	spacing := 1
	if compact {
		spacing = 0
	}
	tileStep := 2 + spacing // 牌宽 + 间距

	l := TableLayout{
		Width:    w,
		Height:   h,
		Compact:  compact,
		TileStep: tileStep,
	}

	// 手牌尺寸以 14 张为上限，避免摸牌后第 14 张被布局裁掉。
	l.HandWidth = maxHandTiles*2 + (maxHandTiles-1)*spacing // 41 宽松 / 28 紧凑
	l.HandHeight = 2                                        // 竖排牌：数字行 + 花色行

	// 牌桌框是主视觉对象。宽度优先给中央信息和四家牌河留呼吸空间，
	// 再确保 14 张南家手牌可以完整渲染。
	sideWallW := 2
	sidePondW := 12
	frameW := 84
	if compact {
		sidePondW = 10
		frameW = 72
	}
	maxFrameW := w - 24 // 左右玩家标签各预留 10 列和间距。
	if maxFrameW < l.HandWidth+8 {
		maxFrameW = w - 4
	}
	if frameW > maxFrameW {
		frameW = maxFrameW
	}
	if minW := l.HandWidth + 8; frameW < minW {
		frameW = minW
	}

	// 牌桌框高。最小终端 100x30 时仍能完整容纳东西 13 张牌背。
	northPondH := 3
	southPondH := 3
	frameH := 30
	if h < 36 {
		frameH = 24
	}
	maxFrameH := h - 6 // 标题、toast、键位栏与上下玩家标签。
	if frameH > maxFrameH {
		frameH = maxFrameH
	}
	minFrameH := 1 + 1 + northPondH + wallTileCount + southPondH + l.HandHeight + 1
	if frameH < minFrameH {
		frameH = minFrameH
	}
	middleH := frameH - 2 - 1 - northPondH - southPondH - l.HandHeight
	if middleH < wallTileCount {
		middleH = wallTileCount
		frameH = 2 + 1 + northPondH + middleH + southPondH + l.HandHeight
	}

	// 框定位
	frameX := (w - frameW) / 2
	frameY := (h - frameH) / 2
	if frameY < 2 {
		frameY = 2
	}
	if frameY+frameH+3 > h {
		frameY = h - frameH - 3
	}

	l.Frame = Region{X: frameX, Y: frameY, Width: frameW, Height: frameH}
	l.Inner = Region{X: frameX + 1, Y: frameY + 1, Width: frameW - 2, Height: frameH - 2}

	// 水平居中手牌位置
	northHandX := l.Inner.X + (l.Inner.Width-l.HandWidth)/2
	southHandX := northHandX // 上下对齐

	// 垂直位置（从外到内）
	northHandY := l.Inner.Y
	northPondY := northHandY + 1
	middleY := northPondY + northPondH
	southHandY := l.Inner.Y + l.Inner.Height - l.HandHeight
	southPondY := southHandY - southPondH

	l.NorthHand = Region{X: northHandX, Y: northHandY, Width: l.HandWidth, Height: 1}
	l.NorthPond = Region{X: l.Inner.X + 2, Y: northPondY, Width: l.Inner.Width - 4, Height: northPondH}
	l.SouthHand = Region{X: southHandX, Y: southHandY, Width: l.HandWidth, Height: l.HandHeight}
	l.SouthPond = Region{X: l.Inner.X + 2, Y: southPondY, Width: l.Inner.Width - 4, Height: southPondH}

	// 西/东墙
	l.WestWall = Region{X: l.Inner.X, Y: middleY, Width: sideWallW, Height: middleH}
	l.EastWall = Region{X: l.Inner.X + l.Inner.Width - sideWallW, Y: middleY, Width: sideWallW, Height: middleH}

	// 西/东牌河
	l.WestPond = Region{X: l.Inner.X + sideWallW + 2, Y: middleY, Width: sidePondW, Height: middleH}
	l.EastPond = Region{X: l.Inner.X + l.Inner.Width - sideWallW - sidePondW - 2, Y: middleY, Width: sidePondW, Height: middleH}

	// 中央区域
	centerX := l.WestPond.X + l.WestPond.Width + 2
	centerEndX := l.EastPond.X - 2
	centerH := 5
	if middleH < centerH {
		centerH = middleH
	}
	l.Center = Region{X: centerX, Y: middleY + (middleH-centerH)/2, Width: centerEndX - centerX, Height: centerH}

	// 框外玩家标签
	l.NorthLabel = Region{X: frameX, Y: frameY - 1, Width: frameW, Height: 1}
	l.SouthLabel = Region{X: frameX, Y: frameY + frameH, Width: frameW, Height: 1}
	l.WestLabel = Region{X: frameX - 10, Y: middleY, Width: 9, Height: 3}
	l.EastLabel = Region{X: frameX + frameW + 1, Y: middleY, Width: 9, Height: 3}

	return l, true
}

// HandColX 返回手牌第 i 张牌的 x 坐标。
func (l TableLayout) HandColX(x, i int) int { return x + i*l.TileStep }

// MaxInt 返回两个整数中的较大值。
func MaxInt(a, b int) int {
	if a >= b {
		return a
	}
	return b
}
