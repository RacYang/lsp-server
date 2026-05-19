package room

import (
	"strconv"
	"strings"

	"racoo.cn/lsp/internal/mahjong/tile"
)

type meldFact struct {
	Kind        string
	Tiles       []tile.Tile
	ClaimedFrom Seat
	Concealed   bool
}

func encodeMeldFact(kind string, tiles []tile.Tile, claimedFrom Seat) string {
	parts := make([]string, 0, len(tiles))
	for _, t := range tiles {
		if t != 0 {
			parts = append(parts, t.String())
		}
	}
	out := kind + ":" + strings.Join(parts, ",")
	if claimedFrom.Valid() {
		out += ":" + strconv.Itoa(int(claimedFrom))
	}
	return out
}

func parseMeldFact(encoded string) (meldFact, bool) {
	first := strings.Index(encoded, ":")
	if first <= 0 || first == len(encoded)-1 {
		return meldFact{}, false
	}
	kind := encoded[:first]
	rest := encoded[first+1:]
	from := SeatInvalid
	if second := strings.LastIndex(rest, ":"); second >= 0 {
		if n, err := strconv.Atoi(rest[second+1:]); err == nil {
			from = SeatFromInt(n)
			rest = rest[:second]
		}
	}
	raws := strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' })
	tiles := make([]tile.Tile, 0, len(raws))
	for _, raw := range raws {
		if raw == "" || raw == "?" {
			continue
		}
		t, err := tile.Parse(raw)
		if err != nil {
			continue
		}
		tiles = append(tiles, t)
	}
	concealed := kind == "an_gang"
	return meldFact{Kind: kind, Tiles: tiles, ClaimedFrom: from, Concealed: concealed}, true
}

func (rs *RoundState) recordMeldFact(seat Seat, kind string, tiles []tile.Tile, claimedFrom Seat) {
	if rs == nil || seat < 0 || seat > 3 || kind == "" {
		return
	}
	for len(rs.melds) < 4 {
		rs.melds = append(rs.melds, nil)
	}
	rs.melds[seat] = append(rs.melds[seat], encodeMeldFact(kind, tiles, claimedFrom))
}

func (rs *RoundState) upgradePongToBuGang(seat Seat, gangTile tile.Tile) bool {
	if rs == nil || seat < 0 || seat > 3 || int(seat) >= len(rs.melds) {
		return false
	}
	for i, encoded := range rs.melds[seat] {
		fact, ok := parseMeldFact(encoded)
		if !ok || fact.Kind != "pong" || len(fact.Tiles) == 0 || fact.Tiles[0] != gangTile {
			continue
		}
		rs.melds[seat][i] = encodeMeldFact("bu_gang", []tile.Tile{gangTile, gangTile, gangTile, gangTile}, fact.ClaimedFrom)
		return true
	}
	return false
}

func (rs *RoundState) hasPongMeld(seat Seat, target tile.Tile) bool {
	if rs == nil || seat < 0 || seat > 3 || int(seat) >= len(rs.melds) {
		return false
	}
	for _, encoded := range rs.melds[seat] {
		fact, ok := parseMeldFact(encoded)
		if ok && fact.Kind == "pong" && len(fact.Tiles) > 0 && fact.Tiles[0] == target {
			return true
		}
	}
	return false
}
