// Package tile 定义万、筒、条三类序数牌的编码与解析，不含字牌与花牌。
package tile

import (
	"fmt"
	"strings"
)

// Suit 表示花色：万、筒、条。
type Suit byte

const (
	SuitCharacters Suit = iota // 万
	SuitDots                   // 筒
	SuitBamboo                 // 条
)

// Tile 为 8 位编码：高 4 位为 Suit，低 4 位为点数 1..9。
type Tile byte

const tileRankMask = 0x0f

// New 构造一张序数牌；rank 取值 1..9。
func New(s Suit, rank int) (Tile, error) {
	if rank < 1 || rank > 9 {
		return 0, fmt.Errorf("rank out of range: %d", rank)
	}
	return Tile(byte(s)<<4 | byte(rank)), nil
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

// Index 返回 0..26 的紧凑下标：万 0..8、筒 9..17、条 18..26。
func (t Tile) Index() int {
	s := int(t.Suit())
	r := t.Rank()
	return s*9 + (r - 1)
}

// FromIndex 由紧凑下标还原牌张。
func FromIndex(idx int) (Tile, error) {
	if idx < 0 || idx > 26 {
		return 0, fmt.Errorf("index out of range: %d", idx)
	}
	suit := Suit(idx / 9)
	rank := idx%9 + 1
	return New(suit, rank)
}

// String 返回便于测试阅读的短字符串，例如 m3、t9、s1。
func (t Tile) String() string {
	var p byte
	switch t.Suit() {
	case SuitCharacters:
		p = 'm'
	case SuitDots:
		p = 'p'
	case SuitBamboo:
		p = 's'
	default:
		p = '?'
	}
	return fmt.Sprintf("%c%d", p, t.Rank())
}

// Parse 解析 m3 / p9 / s1 形式；大小写不敏感。
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
	if rank < 1 || rank > 9 {
		return 0, fmt.Errorf("rank out of range in %q", s)
	}
	return New(suit, rank)
}

// AllSuitTiles 返回某一花色的 1..9 各一张（用于测试或枚举）。
func AllSuitTiles(s Suit) []Tile {
	out := make([]Tile, 9)
	for r := 1; r <= 9; r++ {
		out[r-1] = Must(s, r)
	}
	return out
}
