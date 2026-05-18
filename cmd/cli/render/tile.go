package render

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// TileGlyph 返回协议牌名的完整中文牌面，如"一万""东风"。
func TileGlyph(name string) string {
	f := decodeTile(name)
	if f.suit == "" {
		return f.rank
	}
	return f.rank + f.suit
}

// TileRank 返回牌面数字或字牌名，如"一""东"。
func TileRank(name string) string { return decodeTile(name).rank }

// TileSuit 返回牌面花色，如"万""筒""条"；字牌返回空串。
func TileSuit(name string) string { return decodeTile(name).suit }

// HiddenGlyph 返回牌背字符。
func HiddenGlyph() string { return "牌" }

// DrawVerticalTile 在 (x,y) 绘制竖排牌面：第一行数字/字牌名，第二行花色。
// 每张牌占 2 行 × 2 列，光标/标记由调用方在传入的 style 中表达。
func DrawVerticalTile(scr tcell.Screen, x, y int, st tcell.Style, rank, suit string) {
	DrawText(scr, x, y, st, PadRightVisual(rank, 2))
	if suit != "" {
		DrawText(scr, x, y+1, st, PadRightVisual(suit, 2))
	}
}

// DrawTileBack 在 (x,y) 绘制单张牌背（占 2 列 × 1 行）。
func DrawTileBack(scr tcell.Screen, x, y int, st tcell.Style) {
	DrawText(scr, x, y, st, PadRightVisual(HiddenGlyph(), 2))
}

// DrawTileBackRow 从 (x,y) 开始水平绘制 count 张牌背，间距 spacing 列。
func DrawTileBackRow(scr tcell.Screen, x, y int, st tcell.Style, count, spacing int) {
	step := 2 + spacing
	for i := 0; i < count; i++ {
		DrawTileBack(scr, x+i*step, y, st)
	}
}

// DrawTileBackCol 从 (x,y) 开始垂直绘制 count 张牌背，间距 spacing 行。
func DrawTileBackCol(scr tcell.Screen, x, y int, st tcell.Style, count, spacing int) {
	for i := 0; i < count; i++ {
		DrawTileBack(scr, x, y+i*(1+spacing), st)
	}
}

// ─── 牌名解码 ────────────────────────────────────────

type tileFace struct {
	rank string
	suit string
}

func decodeTile(name string) tileFace {
	name = strings.TrimSpace(strings.ToLower(name))
	if len(name) < 2 {
		return tileFace{rank: "?", suit: "?"}
	}
	suitCh := name[0]
	rankCh := name[1]
	switch suitCh {
	case 'm':
		return tileFace{rank: numberCN(rankCh), suit: "万"}
	case 'p':
		return tileFace{rank: numberCN(rankCh), suit: "筒"}
	case 's':
		return tileFace{rank: numberCN(rankCh), suit: "条"}
	case 'z':
		full := honorCN(rankCh)
		runes := []rune(full)
		if len(runes) == 2 {
			return tileFace{rank: string(runes[0]), suit: string(runes[1])}
		}
		return tileFace{rank: full, suit: ""}
	default:
		return tileFace{rank: "?", suit: "?"}
	}
}

func numberCN(b byte) string {
	switch b {
	case '1':
		return "一"
	case '2':
		return "二"
	case '3':
		return "三"
	case '4':
		return "四"
	case '5':
		return "五"
	case '6':
		return "六"
	case '7':
		return "七"
	case '8':
		return "八"
	case '9':
		return "九"
	default:
		return "?"
	}
}

func honorCN(b byte) string {
	switch b {
	case '1':
		return "东风"
	case '2':
		return "南风"
	case '3':
		return "西风"
	case '4':
		return "北风"
	case '5':
		return "红中"
	case '6':
		return "发财"
	case '7':
		return "白板"
	default:
		return "?"
	}
}
