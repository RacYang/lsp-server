package main

import "fmt"

// HiddenTilesCompact 用紧凑的“牌背 × 数量”表达隐藏手牌，避免横向长串或竖列堆叠制造噪音。
func HiddenTilesCompact(count int, theme TileTheme) string {
	if count <= 0 {
		return ""
	}
	switch theme {
	case TileThemeASCII:
		return fmt.Sprintf("[]×%d", count)
	default:
		return fmt.Sprintf("▢×%d", count)
	}
}
