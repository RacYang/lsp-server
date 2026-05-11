package main

// SeatPosition 表示某个玩家在屏幕上的相对方位（以"我"为南家固定坐标）。
type SeatPosition int

const (
	// SeatPosBottom 是自己（南家）。
	SeatPosBottom SeatPosition = iota
	// SeatPosTop 是对家（北家）——绝对座位上的下家+2。
	SeatPosTop
	// SeatPosLeft 是上家（西家）。
	SeatPosLeft
	// SeatPosRight 是下家（东家）。
	SeatPosRight
)

// String 给调试与 golden 测试一个稳定可读的方位标签。
func (p SeatPosition) String() string {
	switch p {
	case SeatPosBottom:
		return "bottom"
	case SeatPosTop:
		return "top"
	case SeatPosLeft:
		return "left"
	case SeatPosRight:
		return "right"
	default:
		return "?"
	}
}

// RelativeSeat 把绝对座位（0=东 1=南 2=西 3=北）映射为以 selfSeat 为底部的相对方位。
//
// 麻将协议层用四个绝对座位编号 0..3，但出牌顺序是逆时针（东→南→西→北）。
// 主流麻将客户端（雀魂、QQ 麻将、欢乐麻将）的布局约定是：
//   - 自己 → bottom
//   - 下家（出牌顺序的下一家，selfSeat+1）→ left（左侧）
//   - 对家（selfSeat+2）→ top
//   - 上家（出牌顺序的前一家，selfSeat-1）→ right（右侧）
//
// 当 selfSeat 越界或 targetSeat 越界时返回 SeatPosBottom 兜底，避免 panic。
func RelativeSeat(selfSeat, targetSeat int32) SeatPosition {
	if selfSeat < 0 || selfSeat > 3 || targetSeat < 0 || targetSeat > 3 {
		return SeatPosBottom
	}
	diff := (targetSeat - selfSeat + 4) % 4
	switch diff {
	case 0:
		return SeatPosBottom
	case 1:
		return SeatPosLeft
	case 2:
		return SeatPosTop
	case 3:
		return SeatPosRight
	}
	return SeatPosBottom
}

// MinTableWidth / MinTableHeight 是牌桌全屏渲染的最小可工作尺寸。
//
// 低于这个尺寸时 RunTableScreen 会拒绝进入牌桌，提示玩家放大终端。
const (
	MinTableWidth  = 100
	MinTableHeight = 30

	RecommendedTableWidth  = 140
	RecommendedTableHeight = 40
)

// LayoutTier 是雀魂式居中牌桌在 16:9 / 16:10 桌面终端上的三个密度档位。
type LayoutTier int

const (
	// LayoutTierStandard 是 100×30 起步的紧凑桌面布局，桌外形 26×52。
	LayoutTierStandard LayoutTier = iota
	// LayoutTierWide 是 120×36 起步的推荐布局，桌外形 32×64。
	LayoutTierWide
	// LayoutTierFull 是 140×40 起步的全屏布局，桌外形 36×72。
	LayoutTierFull
)

// TableLayout 描述牌桌每个区域在屏幕上的起始坐标与尺寸。
//
// 所有数值以左上角为原点；行号 y、列号 x；尺寸单位为终端 cell（CJK 字符算 2）。
// 这里只描述"块"的起点与高度,具体内容由 table_render 写入。
type TableLayout struct {
	Width, Height int

	TitleBar        Region // 顶部系统信息：房间、局数、剩牌与战绩。
	NorthBand       Region // 北家紧凑信息与焦点提示。
	LeftPlayerSlot  Region // 西家无框玩家信息块。
	TableFrame      Region // 严格正方形牌桌，Width 必须等于 Height*2。
	RightPlayerSlot Region // 东家无框玩家信息块。
	SouthBand       Region // 南家（自己）紧凑信息与操作提示。
	KeyBar          Region // 底部阶段化按键栏。

	// CenterArea 只作为弹窗/结算层的居中锚点保留，指向 TableFrame。
	// 渲染主流程不再把它当成中央信息盒使用。
	CenterArea Region

	Tier  LayoutTier
	Slots TableInnerSlots
}

// TableInnerSlots 描述 TableFrame 内部所有"牌"相关子区域。
//
// 坐标均为屏幕绝对坐标；TableFrame 自身含一圈桌沿，下面区域在桌沿内部。
type TableInnerSlots struct {
	NorthHand Region
	NorthPond Region
	WestWall  Region
	WestPond  Region
	Dial      Region
	EastPond  Region
	EastWall  Region
	SouthPond Region
	SouthHand Region
}

// Region 是屏幕上的一个矩形区域。
type Region struct {
	X, Y          int
	Width, Height int
}

// Empty 表示矩形是否退化为空。
func (r Region) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

// CalcLayout 根据屏幕尺寸切分雀魂式居中牌桌；不满足最小尺寸时返回 zero layout 与 false。
//
// 可选的 lastTier 用于 resize 过程中的 5 cell 滞回：升档需要越过阈值 5 cell，
// 降档立即发生，避免窗口边界轻微抖动时反复重排。
func CalcLayout(width, height int, lastTierHint ...LayoutTier) (TableLayout, bool) {
	if width < MinTableWidth || height < MinTableHeight {
		return TableLayout{}, false
	}
	lastTier := LayoutTierStandard
	if len(lastTierHint) > 0 {
		lastTier = lastTierHint[0]
	}
	tier := ResolveLayoutTier(width, height, lastTier)
	tableH, tableW := tableSizeForTier(tier)
	if tableW > width || tableH+4 > height {
		tableH, tableW = largestCenteredTable(width, height)
	}
	if tableW < 52 || tableH < 26 {
		return TableLayout{}, false
	}

	tableX := (width - tableW) / 2
	tableY := (height - tableH) / 2
	if tableY < 2 {
		tableY = 2
	}
	if tableY+tableH+2 > height {
		tableY = height - tableH - 2
	}

	l := TableLayout{Width: width, Height: height, Tier: tier}
	l.TitleBar = Region{X: 0, Y: 0, Width: width, Height: 1}
	l.NorthBand = Region{X: 0, Y: tableY - 1, Width: width, Height: 1}
	l.TableFrame = Region{X: tableX, Y: tableY, Width: tableW, Height: tableH}
	l.CenterArea = l.TableFrame
	l.LeftPlayerSlot = Region{X: 0, Y: tableY, Width: tableX, Height: tableH}
	l.RightPlayerSlot = Region{X: tableX + tableW, Y: tableY, Width: width - tableX - tableW, Height: tableH}
	l.SouthBand = Region{X: 0, Y: tableY + tableH, Width: width, Height: 1}
	l.KeyBar = Region{X: 0, Y: height - 1, Width: width, Height: 1}
	l.Slots = tableInnerSlots(l.TableFrame, tier)
	return l, true
}

func ResolveLayoutTier(width, height int, lastTier LayoutTier) LayoutTier {
	target := rawLayoutTier(width, height)
	if target > lastTier && !layoutTierCanPromote(width, height, target) {
		return lastTier
	}
	return target
}

func rawLayoutTier(width, height int) LayoutTier {
	switch {
	case width >= 140 && height >= 40:
		return LayoutTierFull
	case width >= 120 && height >= 36:
		return LayoutTierWide
	default:
		return LayoutTierStandard
	}
}

func layoutTierCanPromote(width, height int, target LayoutTier) bool {
	switch target {
	case LayoutTierFull:
		return width >= 145 && height >= 45
	case LayoutTierWide:
		return width >= 125 && height >= 41
	default:
		return true
	}
}

func tableSizeForTier(tier LayoutTier) (height, width int) {
	switch tier {
	case LayoutTierFull:
		return 36, 72
	case LayoutTierWide:
		return 32, 64
	default:
		return 26, 52
	}
}

func largestCenteredTable(width, height int) (tableH, tableW int) {
	tableH = height - 4
	if tableH%2 != 0 {
		tableH--
	}
	tableW = tableH * 2
	if tableW > width {
		tableW = width
		if tableW%2 != 0 {
			tableW--
		}
		tableH = tableW / 2
	}
	return tableH, tableW
}

func tableInnerSlots(frame Region, tier LayoutTier) TableInnerSlots {
	inner := Region{X: frame.X + 1, Y: frame.Y + 1, Width: frame.Width - 2, Height: frame.Height - 2}
	return TableInnerSlots{
		NorthHand: horizontalTileLine(inner, inner.Y+1, hiddenHandWidth(defaultStartingHandSize)),
		NorthPond: Region{
			X:      inner.X + (inner.Width-26)/2,
			Y:      inner.Y + 3,
			Width:  26,
			Height: tablePondRows(tier),
		},
		WestWall: Region{
			X:      inner.X + 1,
			Y:      inner.Y + tableWallY(inner.Height),
			Width:  2,
			Height: defaultStartingHandSize,
		},
		WestPond: Region{
			X:      inner.X + 4,
			Y:      inner.Y + tableSidePondY(inner.Height),
			Width:  18,
			Height: tableSidePondRows(tier),
		},
		Dial: Region{
			X:      inner.X + (inner.Width-7)/2,
			Y:      inner.Y + inner.Height/2 - 1,
			Width:  7,
			Height: 1,
		},
		EastPond: Region{
			X:      inner.X + inner.Width - 22,
			Y:      inner.Y + tableSidePondY(inner.Height),
			Width:  18,
			Height: tableSidePondRows(tier),
		},
		EastWall: Region{
			X:      inner.X + inner.Width - 2,
			Y:      inner.Y + tableWallY(inner.Height),
			Width:  2,
			Height: defaultStartingHandSize,
		},
		SouthPond: Region{
			X:      inner.X + (inner.Width-26)/2,
			Y:      inner.Y + inner.Height - 8,
			Width:  26,
			Height: tablePondRows(tier),
		},
		SouthHand: horizontalTileLine(inner, inner.Y+inner.Height-2, visibleHandWidth(14)),
	}
}

func horizontalTileLine(inner Region, y, tileWidth int) Region {
	return Region{X: inner.X + (inner.Width-tileWidth)/2, Y: y, Width: tileWidth, Height: 1}
}

func visibleHandWidth(count int) int {
	if count <= 0 {
		return 0
	}
	return count*2 + (count - 1)
}

func hiddenHandWidth(count int) int { return visibleHandWidth(count) }

func tablePondRows(tier LayoutTier) int {
	if tier == LayoutTierStandard {
		return 4
	}
	return 5
}

func tableSidePondRows(tier LayoutTier) int {
	if tier == LayoutTierStandard {
		return 5
	}
	return 6
}

func tableWallY(innerH int) int {
	y := (innerH - defaultStartingHandSize) / 2
	if y < 0 {
		return 0
	}
	return y
}

func tableSidePondY(innerH int) int {
	y := innerH/2 - 3
	if y < 0 {
		return 0
	}
	return y
}
