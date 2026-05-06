package main

import (
	"fmt"
	"strings"

	"github.com/rivo/uniseg"
)

// TileTheme 决定字符画牌的视觉风格：拟物 (中文 + Unicode 框线) 或 ASCII 简版。
type TileTheme int

const (
	// TileThemeUnicode 默认拟物风格，使用中文「一二三...九」「万 筒 条」与 ┌─┐│└─┘ 框线。
	// 牌占 4 个 cell 宽 × 3 行高（1 列宽度 = 1 个 ASCII char；CJK 字符占 2 列）。
	TileThemeUnicode TileTheme = iota
	// TileThemeASCII 是降级风格，使用纯 ASCII 字符 +-|，便于在 CJK 渲染异常的终端上保留可读性。
	// 牌占 4 个 cell 宽 × 3 行高，与 Unicode 主题宽度一致，方便切换主题不用重排版。
	TileThemeASCII
)

// String 帮助调试输出与 config 互转。
func (t TileTheme) String() string {
	switch t {
	case TileThemeASCII:
		return tileThemeASCII
	default:
		return tileThemeUnicode
	}
}

// ParseTileTheme 把配置文件中的字符串映射回 TileTheme，未知值回退到 unicode。
func ParseTileTheme(s string) TileTheme {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case tileThemeASCII:
		return TileThemeASCII
	default:
		return TileThemeUnicode
	}
}

// TileArtHeight 是一张牌的固定行数：顶边框 + rank + suit + 底边框 = 4。
const TileArtHeight = 4

// TileArtWidth 是一张牌的"显示宽度"（按终端 cell 计）：
// 左竖线 + 双宽 CJK 字符 + 右竖线 = 1+2+1 = 4 cell。
// ASCII 主题用 +-+|，每个字符 1 cell，宽度同样为 4。
const TileArtWidth = 4

// TileArt 是一张牌渲染后的固定四行字符画。
//
// 四行的"显示宽度"固定为 TileArtWidth=4，相邻牌可以直接横向拼接。
type TileArt struct {
	Lines [TileArtHeight]string
	// Width 返回每行的显示宽度——按终端 cell 数（CJK 字符算 2）。
	Width int
}

// RenderTile 把协议层的牌名（如 1m / 5p / 9s / 1z）渲染成三行字符画。
//
// 若牌名格式不识别，返回带问号的占位牌而不是 panic，便于在异常数据时仍能渲染界面。
func RenderTile(name string, theme TileTheme) TileArt {
	face := decodeTile(name)
	switch theme {
	case TileThemeASCII:
		return renderASCII(face)
	default:
		return renderUnicode(face)
	}
}

// tileFace 把协议字符串解析后的"两个中文字符"或"ASCII 短码"做统一表示。
type tileFace struct {
	rank string // 中文数字或 ASCII 数字（双宽时长度=3 字节，例如 "一"）
	suit string // 中文花色或 ASCII 字母
	// asciiShort 用于 ASCII 主题：保留两字符短码 (如 "1m" / "9s" / "中")
	asciiShort string
}

// decodeTile 解析协议层的牌名为可视化字段。
//
// 服务端的 tile.String() 使用「花色字母 + rank 数字」格式（如 m3 / p9 / s1）；
// 字牌（z1..z7）也走相同形态。本函数与之一致：[0]=suit，[1]=rank。
func decodeTile(name string) tileFace {
	name = strings.TrimSpace(strings.ToLower(name))
	if len(name) < 2 {
		return tileFace{rank: "?", suit: "?", asciiShort: "??"}
	}
	suitCh := name[0]
	rankCh := name[1]
	switch suitCh {
	case 'm':
		return tileFace{rank: numberCN(rankCh), suit: "万", asciiShort: name[:2]}
	case 'p':
		return tileFace{rank: numberCN(rankCh), suit: "筒", asciiShort: name[:2]}
	case 's':
		return tileFace{rank: numberCN(rankCh), suit: "条", asciiShort: name[:2]}
	case 'z':
		return tileFace{rank: honorCN(rankCh), suit: "", asciiShort: honorAscii(rankCh)}
	default:
		return tileFace{rank: "?", suit: "?", asciiShort: name[:2]}
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
		return "东"
	case '2':
		return "南"
	case '3':
		return "西"
	case '4':
		return "北"
	case '5':
		return "中"
	case '6':
		return "发"
	case '7':
		return "白"
	default:
		return "?"
	}
}

func honorAscii(b byte) string {
	switch b {
	case '1':
		return "E "
	case '2':
		return "S "
	case '3':
		return "W "
	case '4':
		return "N "
	case '5':
		return "Z "
	case '6':
		return "F "
	case '7':
		return "B "
	default:
		return "??"
	}
}

// renderUnicode 输出双宽中文字符 + 框线，每张牌占 4 cell 宽 × 4 行高：
//
//	┌──┐
//	│一│
//	│万│
//	└──┘
//
// 字牌（如东风）只有花色字，把字放在 rank 行，suit 行留空白填充。
func renderUnicode(face tileFace) TileArt {
	rank := face.rank
	suit := face.suit
	rankLine := "│" + padCJK(rank, 2) + "│"
	suitLine := "│" + padCJK(suit, 2) + "│"
	if suit == "" {
		// 字牌：rank 行放中文，suit 行只有空白，保持 4 行布局对齐。
		suitLine = "│" + strings.Repeat(" ", 2) + "│"
	}
	return TileArt{
		Lines: [TileArtHeight]string{
			"┌──┐",
			rankLine,
			suitLine,
			"└──┘",
		},
		Width: TileArtWidth,
	}
}

// padCJK 把单个 CJK 字符按"占 2 cell"对齐，目标宽度内不足时填空白。
//
// 这里假定输入只有 1 个 CJK 字符或 1 个空白；用于宽度=2 的固定槽位。
func padCJK(s string, cells int) string {
	w := visualWidth(s)
	if w >= cells {
		return s
	}
	return s + strings.Repeat(" ", cells-w)
}

// visualWidth 估算字符串的终端 cell 宽度：CJK、全角标点、emoji 等宽字符算 2,其他算 1。
//
// 直接复用 rivo/uniseg.StringWidth,它按 East Asian Width 与 grapheme cluster 计算,
// 兼容全角符号（如「（」「：」）与组合字符,避免老实现把 0x2E80~0x9FFF 之外的全角符号
// 当成单宽字符导致界面错位。
func visualWidth(s string) int {
	return uniseg.StringWidth(s)
}

// renderASCII 输出 4 字符宽 × 4 行的 ASCII 拟物牌：
//
//	+--+
//	|1m|
//	|  |
//	+--+
//
// 中间空行让 ASCII 与 Unicode 主题在垂直方向严格等高，便于上下行字符画切换主题不重排。
// 字牌（如东风）在 ASCII 模式下用 "E " "S " 等短码替代。
func renderASCII(face tileFace) TileArt {
	short := face.asciiShort
	if len(short) < 2 {
		short = "??"
	}
	if len(short) > 2 {
		short = short[:2]
	}
	return TileArt{
		Lines: [TileArtHeight]string{
			"+--+",
			fmt.Sprintf("|%s|", short),
			"|  |",
			"+--+",
		},
		Width: TileArtWidth,
	}
}

// JoinTilesHorizontally 把多张牌按水平方向拼接成 TileArtHeight 行字符串。
//
// 同一花色内连续牌之间无间隔，不同花色组之间间隔 2 个空白 cell。
// 调用方传入"分组"以指定哪些牌之间需要插入间隔。
//
//	groups: [{m, m, m}, {p, p}, {s}] → 三段，段间各加 2 个空格
func JoinTilesHorizontally(groups [][]TileArt, sep string) [TileArtHeight]string {
	if sep == "" {
		sep = "  "
	}
	var rows [TileArtHeight]strings.Builder
	for gi, group := range groups {
		if gi > 0 {
			for r := 0; r < TileArtHeight; r++ {
				_, _ = rows[r].WriteString(sep)
			}
		}
		for _, tile := range group {
			for r := 0; r < TileArtHeight; r++ {
				_, _ = rows[r].WriteString(tile.Lines[r])
			}
		}
	}
	var out [TileArtHeight]string
	for r := 0; r < TileArtHeight; r++ {
		out[r] = rows[r].String()
	}
	return out
}

// tileSortKey 给 m/p/s/z 四种花色一个稳定排序权重，便于 sortedTiles 输出"清一色后字"的顺序。
//
// 协议形态是「花色字母 + rank 数字」（如 m3、p9、z1），所以 [0]=suit、[1]=rank。
// 返回字符串方便 sort 直接 lex 比较。
func tileSortKey(name string) string {
	if len(name) < 2 {
		return "z9_" + name
	}
	suit := name[0]
	prefix := "z8"
	switch suit {
	case 'm':
		prefix = "a"
	case 'p':
		prefix = "b"
	case 's':
		prefix = "c"
	case 'z':
		prefix = "d"
	}
	return prefix + string(name[1])
}

// HiddenTilesRow 渲染对家手牌的"背面排列"，用于不暴露牌面但展示数量。
//
// 例如 13 张就是 "▢ ▢ ▢ ..." 13 个；ASCII 模式用 [].
func HiddenTilesRow(count int, theme TileTheme) string {
	if count <= 0 {
		return ""
	}
	switch theme {
	case TileThemeASCII:
		return strings.Repeat("[]", count)
	default:
		return strings.Repeat("▢ ", count-1) + "▢"
	}
}
