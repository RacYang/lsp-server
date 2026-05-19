package main

import (
	"fmt"
	"strings"

	"racoo.cn/lsp/cmd/cli/render"
)

func AdaptRenderTable(model TableFrontendModel) render.TableData {
	data := render.TableData{
		Phase:       string(model.ScreenPhase),
		RoomLabel:   model.RoomLabel,
		RuleLabel:   model.RuleLabel,
		WallRemain:  int(model.WallRemaining),
		RoundHand:   fmt.Sprintf("第%d局 第%d手", model.RoundIndex+1, model.HandIndex+1),
		PhasePrompt: model.Prompt,
		Events:      append([]string(nil), model.Events...),
		Cursor: render.CursorState{
			Mode:    cursorModeString(model.Cursor.Mode),
			Index:   model.Cursor.Index,
			Marked:  append([]int(nil), model.Cursor.Marked...),
			Pending: model.Cursor.Pending,
		},
	}
	if model.HasCountdown {
		data.Countdown = model.CountdownSeconds
	}
	if model.ScreenPhase == TableScreenRoomPrep {
		data.Phase = "room_prep"
		data.PhasePrompt = ""
		data.Countdown = 0
	}
	if model.ScreenPhase == TableScreenSettlement {
		data.Phase = "settlement"
		data.PhasePrompt = "本局结束"
		data.Countdown = 0
	}
	for rel := 0; rel < 4; rel++ {
		seat := model.Seats[rel]
		name := strings.TrimPrefix(seat.Name, "★ ")
		data.Players[rel] = render.PlayerInfo{
			Name:      name,
			SeatLabel: seat.Label,
			Status:    seat.Status,
			IsFocus:   seat.IsFocus,
			Que:       seat.Que,
			Melds:     seat.Melds,
			Score:     seat.Score,
		}
		data.HandCounts[rel] = seat.HandCount
		for _, t := range seat.Hand {
			data.Hands[rel] = append(data.Hands[rel], tileFace(t))
		}
		for _, t := range seat.Discards {
			data.Discards[rel] = append(data.Discards[rel], tileFace(t))
		}
	}
	return data
}

func tileFace(t string) render.TileFace {
	return render.TileFace{
		Glyph: render.TileGlyph(t),
		Rank:  render.TileRank(t),
		Suit:  render.TileSuit(t),
	}
}
