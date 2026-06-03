package tile

import (
	"fmt"
	"strings"
)

// Suit 表示牌类：万、筒、条、字、花。
type Suit byte

const (
	SuitCharacters Suit = iota // 万
	SuitDots                   // 筒
	SuitBamboo                 // 条
	SuitHonor                  // 字：东南西北中发白
	SuitFlower                 // 花：春夏秋冬梅兰竹菊
)

// Tile 为 8 位编码：高 4 位为 Suit，低 4 位为点数 1..9。
type Tile byte

const tileRankMask = 0x0f
const (
	SuitedTileCount   = 27
	PlayableTileCount = 34
	FullTileCount     = 42
)

// New 构造一张牌。万筒条 rank=1..9，字牌 rank=1..7，花牌 rank=1..8。
func New(s Suit, rank int) (Tile, error) {
	if !validRank(s, rank) {
		return 0, fmt.Errorf("rank out of range: %d", rank)
	}
	return Tile(byte(s)<<4 | byte(rank)), nil //nolint:gosec // validRank bounds suit/rank to 4-bit tile encoding.
}

// Must 与 New 相同，非法参数时 panic，仅用于测试常量初始化。
func Must(s Suit, rank int) Tile {
	t, err := New(s, rank)
	if err != nil {
		panic(err)
	}
	return t
}

// Suit 返回花色。
func (t Tile) Suit() Suit {
	return Suit(byte(t) >> 4)
}

// Rank 返回点数 1..9。
func (t Tile) Rank() int {
	return int(byte(t) & tileRankMask)
}

// Index 返回 0..41 的紧凑下标：万筒条 0..26，字牌 27..33，花牌 34..41。
func (t Tile) Index() int {
	switch t.Suit() {
	case SuitCharacters, SuitDots, SuitBamboo:
		return int(t.Suit())*9 + (t.Rank() - 1)
	case SuitHonor:
		return SuitedTileCount + (t.Rank() - 1)
	case SuitFlower:
		return PlayableTileCount + (t.Rank() - 1)
	default:
		return -1
	}
}

// FromIndex 由紧凑下标还原牌张。
func FromIndex(idx int) (Tile, error) {
	if idx < 0 || idx >= FullTileCount {
		return 0, fmt.Errorf("index out of range: %d", idx)
	}
	if idx < SuitedTileCount {
		return New(Suit(idx/9), idx%9+1)
	}
	if idx < PlayableTileCount {
		return New(SuitHonor, idx-SuitedTileCount+1)
	}
	return New(SuitFlower, idx-PlayableTileCount+1)
}

// String 返回便于测试阅读的短字符串，例如 m3、p9、s1、z5、f8。
func (t Tile) String() string {
	var p byte
	switch t.Suit() {
	case SuitCharacters:
		p = 'm'
	case SuitDots:
		p = 'p'
	case SuitBamboo:
		p = 's'
	case SuitHonor:
		p = 'z'
	case SuitFlower:
		p = 'f'
	default:
		p = '?'
	}
	return fmt.Sprintf("%c%d", p, t.Rank())
}

// Parse 解析 m3 / p9 / s1 / z5 / f8 形式；大小写不敏感。
func Parse(s string) (Tile, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) != 2 {
		return 0, fmt.Errorf("invalid tile string: %q", s)
	}
	var suit Suit
	switch s[0] {
	case 'm':
		suit = SuitCharacters
	case 'p':
		suit = SuitDots
	case 's':
		suit = SuitBamboo
	case 'z':
		suit = SuitHonor
	case 'f':
		suit = SuitFlower
	default:
		return 0, fmt.Errorf("invalid suit in %q", s)
	}
	var rank int
	for _, ch := range s[1:] {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid rank in %q", s)
		}
		rank = rank*10 + int(ch-'0')
	}
	if !validRank(suit, rank) {
		return 0, fmt.Errorf("rank out of range in %q", s)
	}
	return New(suit, rank)
}

// IsSuited 判断是否为可成顺的万筒条。
func (t Tile) IsSuited() bool {
	return t.Suit() == SuitCharacters || t.Suit() == SuitDots || t.Suit() == SuitBamboo
}

// IsHonor 判断是否为字牌。
func (t Tile) IsHonor() bool {
	return t.Suit() == SuitHonor
}

// IsFlower 判断是否为花牌。花牌不参与 14 张牌型分解。
func (t Tile) IsFlower() bool {
	return t.Suit() == SuitFlower
}

// AllSuitTiles 返回某一花色的 1..9 各一张（用于测试或枚举）。
func AllSuitTiles(s Suit) []Tile {
	max := maxRank(s)
	out := make([]Tile, max)
	for r := 1; r <= max; r++ {
		out[r-1] = Must(s, r)
	}
	return out
}

func validRank(s Suit, rank int) bool {
	return rank >= 1 && rank <= maxRank(s)
}

func maxRank(s Suit) int {
	switch s {
	case SuitCharacters, SuitDots, SuitBamboo:
		return 9
	case SuitHonor:
		return 7
	case SuitFlower:
		return 8
	default:
		return 0
	}
}
