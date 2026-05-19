// Package hu 提供和牌判定：标准形（四面子一对将）与七对子。
package hu

// Counts 为 27 维牌张计数表：万 0..8、筒 9..17、条 18..26。
type Counts [27]int

// Total 返回总张数。
func (c Counts) Total() int {
	n := 0
	for _, v := range c {
		n += v
	}
	return n
}

// IsWinning 判断 14 张牌是否构成和牌形（标准形或七对）。
func IsWinning(c Counts) bool {
	if c.Total() != 14 {
		return false
	}
	// 108 张牌墙下，任意牌张最多 4 枚；超过则不可能为合法和牌形，避免算法在极端计数上误判。
	for _, n := range c {
		if n > 4 {
			return false
		}
	}
	return SevenPairs(c) || StandardForm(c)
}

// IsWinningWithOpenMelds 判断带固定副露面子数的和牌形。
//
// openMelds 表示已经完成的副露面子数量；c 只包含暗手加本次和牌张。
// 无副露时保持普通 14 张判定；有副露时七对子不成立，暗手部分只需要
// 组成剩余面子加一对将。
func IsWinningWithOpenMelds(c Counts, openMelds int) bool {
	if openMelds < 0 || openMelds > 4 {
		return false
	}
	if openMelds == 0 {
		return IsWinning(c)
	}
	if c.Total() != 14-openMelds*3 {
		return false
	}
	for _, n := range c {
		if n > 4 {
			return false
		}
	}
	return StandardFormWithOpenMelds(c, openMelds)
}

// SevenPairs 判断是否为七对子（允许 4 张相同作两对，川麻常见处理）。
func SevenPairs(c Counts) bool {
	if c.Total() != 14 {
		return false
	}
	pairs := 0
	quads := 0
	for _, n := range c {
		switch n {
		case 0:
			continue
		case 2:
			pairs++
		case 4:
			quads += 2 // 视为两对
		default:
			return false
		}
	}
	return pairs+quads == 7
}

func nextNonZero(c Counts) int {
	for i := 0; i < 27; i++ {
		if c[i] > 0 {
			return i
		}
	}
	return -1
}

// StandardForm 判断是否存在一对将 + 四组面子（刻子或顺子）。
func StandardForm(c Counts) bool {
	if c.Total() != 14 {
		return false
	}
	return standardFormMelds(c, 4)
}

// StandardFormWithOpenMelds 判断暗手是否能和已固定副露组成标准形。
func StandardFormWithOpenMelds(c Counts, openMelds int) bool {
	if openMelds < 0 || openMelds > 4 {
		return false
	}
	if c.Total() != 14-openMelds*3 {
		return false
	}
	return standardFormMelds(c, 4-openMelds)
}

func standardFormMelds(c Counts, meldsNeeded int) bool {
	if meldsNeeded < 0 || meldsNeeded > 4 {
		return false
	}
	// 枚举将牌位置
	for i := 0; i < 27; i++ {
		if c[i] < 2 {
			continue
		}
		rest := c
		rest[i] -= 2
		if meldN(rest, meldsNeeded) {
			return true
		}
	}
	return false
}

// meldAll 递归消耗面子：每次必须覆盖当前最小非零下标。
func meldAll(c Counts) bool {
	return meldN(c, c.Total()/3)
}

func meldN(c Counts, need int) bool {
	if need < 0 || c.Total() != need*3 {
		return false
	}
	i := nextNonZero(c)
	if i < 0 {
		return need == 0
	}
	suit := i / 9
	r := i % 9
	base := suit * 9

	// 刻子
	if c[i] >= 3 {
		rest := c
		rest[i] -= 3
		if meldN(rest, need-1) {
			return true
		}
	}

	// 顺子：枚举顺子起点 start（0..6），且必须覆盖当前最小非零位 i。
	for start := r - 2; start <= r; start++ {
		if start < 0 || start > 6 {
			continue
		}
		a, b, cc := base+start, base+start+1, base+start+2
		if a != i && b != i && cc != i {
			continue
		}
		if c[a] >= 1 && c[b] >= 1 && c[cc] >= 1 {
			rest := c
			rest[a]--
			rest[b]--
			rest[cc]--
			if meldN(rest, need-1) {
				return true
			}
		}
	}
	return false
}
