package main

// renderEmoji 输出 Unicode Mahjong Tiles 区块中的单字符牌面。
//
// 这些字符在多数现代终端中按双宽 emoji 渲染；外层补空格到 TileArtWidth，
// 保持与 unicode/ascii 主题同样的 4 cell 宽、4 行高。
func renderEmoji(face tileFace) TileArt {
	glyph := emojiTileGlyph(face.asciiShort)
	line := padCJK(glyph, TileArtWidth)
	return TileArt{
		Lines: [TileArtHeight]string{
			"    ",
			line,
			"    ",
			"    ",
		},
		Width: TileArtWidth,
	}
}

func emojiTileGlyph(short string) string {
	switch short {
	case "m1":
		return "🀇"
	case "m2":
		return "🀈"
	case "m3":
		return "🀉"
	case "m4":
		return "🀊"
	case "m5":
		return "🀋"
	case "m6":
		return "🀌"
	case "m7":
		return "🀍"
	case "m8":
		return "🀎"
	case "m9":
		return "🀏"
	case "s1":
		return "🀐"
	case "s2":
		return "🀑"
	case "s3":
		return "🀒"
	case "s4":
		return "🀓"
	case "s5":
		return "🀔"
	case "s6":
		return "🀕"
	case "s7":
		return "🀖"
	case "s8":
		return "🀗"
	case "s9":
		return "🀘"
	case "p1":
		return "🀙"
	case "p2":
		return "🀚"
	case "p3":
		return "🀛"
	case "p4":
		return "🀜"
	case "p5":
		return "🀝"
	case "p6":
		return "🀞"
	case "p7":
		return "🀟"
	case "p8":
		return "🀠"
	case "p9":
		return "🀡"
	case "E ":
		return "🀀"
	case "S ":
		return "🀁"
	case "W ":
		return "🀂"
	case "N ":
		return "🀃"
	case "Z ":
		return "🀄"
	case "F ":
		return "🀅"
	case "B ":
		return "🀆"
	default:
		return "?"
	}
}
