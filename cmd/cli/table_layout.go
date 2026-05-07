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
	MinTableWidth  = 80
	MinTableHeight = 24
)

// TableLayout 描述牌桌每个区域在屏幕上的起始坐标与尺寸。
//
// 所有数值以左上角为原点；行号 y、列号 x；尺寸单位为终端 cell（CJK 字符算 2）。
// 这里只描述"块"的起点与高度,具体内容由 table_render 写入。
type TableLayout struct {
	Width, Height int

	StatusBar        Region // 顶部状态栏: 房号/规则/剩牌/庄家/上一动作
	TopArea          Region // 北家（对家）信息块
	LeftArea         Region // 西家（上家）信息块
	RightArea        Region // 东家（下家）信息块
	CenterArea       Region // 中央提示语/浮窗区
	SelfArea         Region // 自己区域：牌河/鸣牌/手牌/摸牌
	SelfDiscardsArea Region // 自己牌河（最近若干弃张）;与其他三家"打: ..."保持对称
	SelfMeldsArea    Region // 自己鸣牌（碰/杠/吃）;与其他三家"鸣: ..."保持对称
	HandArea         Region // 自己手牌字符画
	KeyBar           Region // 底部按键栏
	HintArea         Region // 兼容旧测试与调用；等同 KeyBar

	DiscardColumns int  // 每行展示多少张弃牌
	Wide           bool // 是否为宽屏布局
}

// Region 是屏幕上的一个矩形区域。
type Region struct {
	X, Y          int
	Width, Height int
}

// Empty 表示矩形是否退化为空。
func (r Region) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

// CalcLayout 根据屏幕尺寸切分各区域；不满足最小尺寸时返回 zero layout 与 false。
//
// 区域划分（最小 80×24 时）：
//
//	行 0      顶部状态栏
//	行 1..5   顶部对家(高 5)
//	行 6..11  左右上下家信息(高 6)
//	行 12..16 中央信息盒(高 5)
//	行 17     自己的牌河行（"打: ..."）
//	行 18     自己的鸣牌行（"鸣: ..."）
//	行 19     凸起预留(光标选中时,手牌顶端会上移 1 行到此处)
//	行 20..23 你的手牌(高 4)
//	末行      底部按键栏
//
// 自家"鸣"行紧贴手牌（与其他家"鸣 紧贴 hand row"对称）,"打"行在更外侧（更接近中央桌面）,
// 与对家「行 2 鸣 / 行 3 打」相对手牌的远近对称。
//
// 实际实现保留少量自适应：屏幕越大,中央区会按比例向下放。
func CalcLayout(width, height int) (TableLayout, bool) {
	if width < MinTableWidth || height < MinTableHeight {
		return TableLayout{}, false
	}
	l := TableLayout{Width: width, Height: height}
	l.Wide = width >= 100
	l.DiscardColumns = 4
	if l.Wide {
		l.DiscardColumns = 6
	}

	l.StatusBar = Region{X: 0, Y: 0, Width: width, Height: 1}
	l.KeyBar = Region{X: 0, Y: height - 1, Width: width, Height: 1}
	l.HintArea = l.KeyBar

	leftWidth := (width - 2) / 2
	rightX := leftWidth + 1
	rightWidth := width - rightX
	l.TopArea = Region{X: 0, Y: 1, Width: width, Height: 5}
	l.LeftArea = Region{X: 0, Y: 6, Width: leftWidth, Height: 6}
	l.RightArea = Region{X: rightX, Y: 6, Width: rightWidth, Height: 6}

	handY := height - TileArtHeight - 1
	selfY := handY - 3
	l.SelfArea = Region{X: 0, Y: selfY, Width: width, Height: height - selfY - 1}
	centerY := 12
	centerHeight := selfY - centerY
	if centerHeight < 3 {
		centerHeight = 3
	}
	l.CenterArea = Region{X: 0, Y: centerY, Width: width, Height: centerHeight}

	// 紧贴 HandArea 上方依次是: protrusion 预留(handY-1) -> 鸣 (handY-2) -> 打 (handY-3)。
	selfMeldsY := handY - 2
	selfDiscardsY := handY - 3
	floor := centerY + centerHeight
	if selfDiscardsY < floor {
		selfDiscardsY = floor
	}
	if selfMeldsY < selfDiscardsY+1 {
		selfMeldsY = selfDiscardsY + 1
	}
	l.SelfDiscardsArea = Region{X: 0, Y: selfDiscardsY, Width: width, Height: 1}
	l.SelfMeldsArea = Region{X: 0, Y: selfMeldsY, Width: width, Height: 1}
	l.HandArea = Region{X: 0, Y: handY, Width: width, Height: TileArtHeight}
	l.HintArea = Region{X: 0, Y: height - 1, Width: width, Height: 1}
	return l, true
}
