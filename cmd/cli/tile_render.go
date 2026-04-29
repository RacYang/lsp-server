package main

import (
	"fmt"
	"strconv"
	"strings"
)

// RenderOptions 控制牌张与整屏渲染。
type RenderOptions struct {
	Width    int
	Height   int
	CJKTiles bool
	NoColor  bool
}

func renderTile(tile string, style string, opts RenderOptions) []string {
	if style == "back" {
		return []string{"+--+", "|##|", "|##|", "+--+"}
	}
	suit, rank := splitTile(tile)
	if opts.CJKTiles {
		label := cjkSuit(suit)
		if style == "highlight" {
			return []string{"╔══╗", "║" + label + "║", fmt.Sprintf("║%2s║", rank), "╚══╝"}
		}
		return []string{"┌──┐", "│" + label + "│", fmt.Sprintf("│%2s│", rank), "└──┘"}
	}
	if style == "highlight" {
		return []string{"+==+", "|" + suit + " |", fmt.Sprintf("|%-2s|", rank), "+==+"}
	}
	return []string{"+--+", "|" + suit + " |", fmt.Sprintf("|%-2s|", rank), "+--+"}
}

func splitTile(raw string) (string, string) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if len(s) < 2 {
		return "?", "?"
	}
	switch s[0] {
	case 'm', 'p', 's':
		return s[:1], s[1:]
	default:
		last := s[len(s)-1:]
		if last == "m" || last == "p" || last == "s" {
			return last, s[:len(s)-1]
		}
		return s[:1], s[1:]
	}
}

func cjkSuit(suit string) string {
	switch suit {
	case "m":
		return "万"
	case "p":
		return "筒"
	case "s":
		return "条"
	default:
		return "？"
	}
}

func tileSortKey(raw string) string {
	suit, rank := splitTile(raw)
	suitRank := map[string]string{"m": "0", "p": "1", "s": "2"}[suit]
	if suitRank == "" {
		suitRank = "9"
	}
	n, _ := strconv.Atoi(rank)
	return fmt.Sprintf("%s%02d%s", suitRank, n, raw)
}
