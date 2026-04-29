package main

import (
	"strings"
)

func tileRow(tiles []string, highlightLast bool, opts RenderOptions) []string {
	if len(tiles) == 0 {
		return []string{"-"}
	}
	lines := []string{"", "", "", ""}
	for i, tile := range tiles {
		style := "normal"
		if highlightLast && i == len(tiles)-1 {
			style = "highlight"
		}
		block := renderTile(tile, style, opts)
		for row := range lines {
			if i > 0 && highlightLast && i == len(tiles)-1 {
				lines[row] += " "
			}
			lines[row] += block[row]
		}
	}
	return lines
}

func backStack(count int, opts RenderOptions) string {
	if count <= 0 {
		return "-"
	}
	block := renderTile("", "back", opts)
	return block[0] + "x" + itoa(count)
}

func tileGrid(tiles []string, width int, opts RenderOptions) []string {
	if len(tiles) == 0 {
		return []string{"-"}
	}
	perLine := width / 4
	if perLine <= 0 {
		perLine = 1
	}
	if perLine > len(tiles) {
		perLine = len(tiles)
	}
	var out []string
	for start := 0; start < len(tiles); start += perLine {
		end := start + perLine
		if end > len(tiles) {
			end = len(tiles)
		}
		out = append(out, tileRow(tiles[start:end], false, opts)...)
	}
	if len(out) > 8 {
		out = append([]string{"..."}, out[len(out)-8:]...)
	}
	return out
}

func padRight(s string, width int) string {
	if len([]rune(s)) >= width {
		rs := []rune(s)
		return string(rs[:width])
	}
	return s + strings.Repeat(" ", width-len([]rune(s)))
}

func centerText(s string, width int) string {
	rs := []rune(s)
	if len(rs) >= width {
		return string(rs[:width])
	}
	left := (width - len(rs)) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-len(rs)-left)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
