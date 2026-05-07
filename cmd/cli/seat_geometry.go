package main

// RotateForSeat 把南家基准矩形旋转到目标方位。
//
// 当前实现只给测试和后续扩展保留几何语义：桌内各槽位由 tableInnerSlots 明确给出，
// 这里负责保证四方映射的边界行为稳定。
func RotateForSeat(base, frame Region, pos SeatPosition) Region {
	switch pos {
	case SeatPosBottom:
		return base
	case SeatPosTop:
		dx := base.X - frame.X
		dy := base.Y - frame.Y
		return Region{
			X:      frame.X + frame.Width - dx - base.Width,
			Y:      frame.Y + frame.Height - dy - base.Height,
			Width:  base.Width,
			Height: base.Height,
		}
	case SeatPosLeft:
		dx := base.X - frame.X
		dy := base.Y - frame.Y
		return Region{
			X:      frame.X + dy,
			Y:      frame.Y + frame.Width - dx - base.Width,
			Width:  base.Height,
			Height: base.Width,
		}
	case SeatPosRight:
		dx := base.X - frame.X
		dy := base.Y - frame.Y
		return Region{
			X:      frame.X + frame.Height - dy - base.Height,
			Y:      frame.Y + dx,
			Width:  base.Height,
			Height: base.Width,
		}
	default:
		return base
	}
}

func PondDirForSeat(pos SeatPosition) PondDir {
	switch pos {
	case SeatPosBottom:
		return PondUp
	case SeatPosLeft:
		return PondRight
	case SeatPosRight:
		return PondLeft
	default:
		return PondDown
	}
}
