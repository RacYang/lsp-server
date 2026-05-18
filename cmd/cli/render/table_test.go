package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
)

func makeSimScreen(w, h int) tcell.SimulationScreen {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		panic(err)
	}
	scr.SetSize(w, h)
	return scr
}

func dumpScreen(scr tcell.SimulationScreen) string {
	contents, w, h := scr.GetContents()
	var lines []string
	for y := 0; y < h; y++ {
		var line strings.Builder
		skip := 0
		for x := 0; x < w; x++ {
			if skip > 0 {
				skip--
				continue
			}
			cell := contents[y*w+x]
			if len(cell.Runes) == 0 || cell.Runes[0] == 0 {
				line.WriteByte(' ')
			} else {
				r := cell.Runes[0]
				line.WriteRune(r)
				wid := VisualWidth(string(r))
				if wid > 1 {
					skip = wid - 1
				}
			}
		}
		str := strings.TrimRight(line.String(), " ")
		lines = append(lines, str)
	}
	return strings.Join(lines, "\n")
}

func sampleTableData(phase string) TableData {
	data := TableData{
		Phase:       phase,
		RoomLabel:   "测试局-001",
		RuleLabel:   "川麻血战",
		WallRemain:  12,
		RoundHand:   "第3局 第5手",
		PhasePrompt: "◆ 该你出牌 ◆",
		Countdown:   12,
	}

	data.Players[0] = PlayerInfo{
		Name: "racoo", SeatLabel: "南", Status: "●",
		Que: "万", Melds: "", Score: "2400",
	}
	data.Players[1] = PlayerInfo{
		Name: "alice", SeatLabel: "西", Status: "●",
		Que: "筒", Melds: "碰三筒", Score: "-800",
	}
	data.Players[2] = PlayerInfo{
		Name: "bob", SeatLabel: "北", Status: "●",
		Que: "条", Melds: "", Score: "1200",
	}
	data.Players[3] = PlayerInfo{
		Name: "BOT-0", SeatLabel: "东", Status: "机",
		Que: "万", Melds: "杠七条", Score: "-2800",
	}

	for i := 0; i < 4; i++ {
		for j := 0; j < 13; j++ {
			t := fmt.Sprintf("%c%d", "mpsz"[j%4], j%9+1)
			f := decodeTile(t)
			data.Hands[i] = append(data.Hands[i], TileFace{
				Glyph: TileGlyph(t),
				Rank:  f.rank,
				Suit:  f.suit,
			})
		}
		for j := 0; j < 6; j++ {
			t := fmt.Sprintf("%c%d", "mps"[j%3], (j+3)%9+1)
			f := decodeTile(t)
			data.Discards[i] = append(data.Discards[i], TileFace{
				Glyph: TileGlyph(t),
				Rank:  f.rank,
				Suit:  f.suit,
			})
		}
	}
	data.Cursor = CursorState{Mode: "single", Index: 6}

	return data
}

func TestRenderTableFramePlayingFull(t *testing.T) {
	scr := makeSimScreen(140, 40)
	defer scr.Fini()

	data := sampleTableData("playing")
	layout, ok := CalcTable(140, 40)
	require.True(t, ok)

	DrawTableFrame(scr, layout, data)
	scr.Show()

	dump := dumpScreen(scr)

	require.Contains(t, dump, "racoo", "must show self name")
	require.Contains(t, dump, "alice", "must show west player")
	require.Contains(t, dump, "bob", "must show north player")
	require.Contains(t, dump, "┌", "must have table frame")
	require.Contains(t, dump, "┐", "must have table frame")
	require.Contains(t, dump, "└", "must have table frame")
	require.Contains(t, dump, "┘", "must have table frame")
	require.Contains(t, dump, "◆ 该你出牌 ◆", "center must show prompt")
	require.Contains(t, dump, "12s", "center must show countdown")
}

func TestRenderTableFrameRoomPrep(t *testing.T) {
	scr := makeSimScreen(140, 40)
	defer scr.Fini()

	data := sampleTableData("room_prep")
	data.PhasePrompt = ""
	data.Countdown = 0
	data.Cursor = CursorState{Mode: "none"}

	layout, ok := CalcTable(140, 40)
	require.True(t, ok)

	DrawTableFrame(scr, layout, data)
	scr.Show()

	dump := dumpScreen(scr)
	require.Contains(t, dump, "┌", "table frame must be visible in room prep")
	require.Contains(t, dump, "牌", "tile backs must be visible")
}

func TestRenderTableFrameSettlement(t *testing.T) {
	scr := makeSimScreen(140, 40)
	defer scr.Fini()

	data := sampleTableData("settlement")
	data.PhasePrompt = ""
	data.Countdown = 0

	layout, ok := CalcTable(140, 40)
	require.True(t, ok)

	DrawTableFrame(scr, layout, data)
	scr.Show()

	dump := dumpScreen(scr)
	require.Contains(t, dump, "┌", "table frame must be visible in settlement")
}

func TestRenderTableFrameStandard(t *testing.T) {
	scr := makeSimScreen(100, 30)
	defer scr.Fini()

	data := sampleTableData("playing")
	layout, ok := CalcTable(100, 30)
	require.True(t, ok)

	DrawTableFrame(scr, layout, data)
	scr.Show()

	dump := dumpScreen(scr)
	lines := strings.Split(dump, "\n")
	nonEmpty := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty++
		}
	}
	require.Greater(t, nonEmpty, 15, "must render meaningful content")
}

func TestCalcTableMinSize(t *testing.T) {
	_, ok := CalcTable(90, 25)
	require.False(t, ok, "below minimum size should fail")

	_, ok = CalcTable(100, 30)
	require.True(t, ok)
}

func TestDrawDialogSingleBorder(t *testing.T) {
	scr := makeSimScreen(80, 40)
	defer scr.Fini()

	center := Region{X: 20, Y: 10, Width: 40, Height: 20}
	lines := []string{"第一行", "第二行测试", "第三行"}
	DrawDialog(scr, center, "测试标题", lines, BorderSingle)
	scr.Show()

	dump := dumpScreen(scr)
	require.Contains(t, dump, "测试标题")
	require.Contains(t, dump, "第一行")
	require.Contains(t, dump, "┌")
	require.NotContains(t, dump, "╔", "single border must not use double line chars")
}

func TestDrawDialogDoubleBorder(t *testing.T) {
	scr := makeSimScreen(80, 40)
	defer scr.Fini()

	center := Region{X: 20, Y: 10, Width: 40, Height: 20}
	lines := []string{"胡了！", "清一色 +6"}
	DrawDialog(scr, center, "结算", lines, BorderDouble)
	scr.Show()

	dump := dumpScreen(scr)
	require.Contains(t, dump, "结算")
	require.Contains(t, dump, "胡了！")
	require.Contains(t, dump, "╔", "double border must use double line chars")
}

func TestBuildSeatPrepLines(t *testing.T) {
	players := [4]PlayerInfo{
		{Name: "racoo", SeatLabel: "南", Status: "✓"},
		{Name: "alice", SeatLabel: "西", Status: "●"},
		{Name: "bob", SeatLabel: "北", Status: "✓"},
		{Name: "", SeatLabel: "东", Status: "空"},
	}
	lines := BuildSeatPrepLines(players)
	require.NotEmpty(t, lines)
	require.Contains(t, lines[0], "bob", "top line should show north player")
}

func TestBuildSettlementLines(t *testing.T) {
	lines := BuildSettlementLines("胡 了 !", []string{"清一色 +6", "对对胡 +2"}, 8, []ScoreItem{
		{Label: "你", Value: 2400},
		{Label: "alice", Value: -800},
	})
	require.NotEmpty(t, lines)
	require.Contains(t, lines[0], "胡 了 !")
	require.Contains(t, strings.Join(lines, "\n"), "清一色")
}

func TestDrawPanel(t *testing.T) {
	scr := makeSimScreen(80, 30)
	defer scr.Fini()

	DrawPanel(scr, 80, 30, "帮助", []string{"大厅用方向键选择入口", "Enter 确认"})
	scr.Show()

	dump := dumpScreen(scr)
	require.Contains(t, dump, "帮助")
	require.Contains(t, dump, "大厅用方向键选择入口")
}

func TestDrawCardGrid(t *testing.T) {
	scr := makeSimScreen(100, 30)
	defer scr.Fini()

	cards := []Card{
		{Title: "快速开始", Desc: "自动补齐机器人", Hint: "Enter"},
		{Title: "创建房间", Desc: "选择玩法开局", Hint: "Enter"},
	}
	DrawCardGrid(scr, 20, 10, cards, 18, 6, 0)
	scr.Show()

	dump := dumpScreen(scr)
	require.Contains(t, dump, "快速开始")
	require.Contains(t, dump, "创建房间")
}

func TestSemanticStyles(t *testing.T) {
	require.NotPanics(t, func() {
		for _, s := range []Semantic{SemDefault, SemEmphasis, SemDim, SemWarning, SemSuccess, SemDanger} {
			_ = Style(s)
		}
	})
}

func TestTileGlyph(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		rank  string
		suit  string
	}{
		{"一万", "m1", "一万", "一", "万"},
		{"五筒", "p5", "五筒", "五", "筒"},
		{"九条", "s9", "九条", "九", "条"},
		{"东风", "z1", "东风", "东", "风"},
		{"红中", "z5", "红中", "红", "中"},
		{"白板", "z7", "白板", "白", "板"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, TileGlyph(tt.input))
			require.Equal(t, tt.rank, TileRank(tt.input))
			require.Equal(t, tt.suit, TileSuit(tt.input))
		})
	}
}

func TestDrawVerticalTile(t *testing.T) {
	scr := makeSimScreen(40, 10)
	defer scr.Fini()

	DrawVerticalTile(scr, 10, 2, DefaultStyle(), "一", "万")
	DrawVerticalTile(scr, 13, 2, Style(SemEmphasis), "东", "风")
	scr.Show()

	dump := dumpScreen(scr)
	require.Contains(t, dump, "一")
	require.Contains(t, dump, "万")
	require.Contains(t, dump, "东")
	require.Contains(t, dump, "风")
}

func TestDrawTileBackRow(t *testing.T) {
	scr := makeSimScreen(40, 5)
	defer scr.Fini()

	DrawTileBackRow(scr, 2, 1, DefaultStyle(), 5, 1)
	scr.Show()

	dump := dumpScreen(scr)
	rCount := strings.Count(dump, "牌")
	require.Equal(t, 5, rCount, "should draw 5 tile backs")
}

func TestDrawTileBackCol(t *testing.T) {
	scr := makeSimScreen(10, 20)
	defer scr.Fini()

	DrawTileBackCol(scr, 2, 1, DefaultStyle(), 5, 1)
	scr.Show()

	dump := dumpScreen(scr)
	rCount := strings.Count(dump, "牌")
	require.Equal(t, 5, rCount, "should draw 5 tile backs vertically")
}

func TestCalcTableCompact(t *testing.T) {
	layout, ok := CalcTable(100, 30)
	require.True(t, ok)
	require.True(t, layout.Compact)

	require.False(t, layout.Frame.Empty())
	require.False(t, layout.NorthHand.Empty())
	require.False(t, layout.SouthHand.Empty())
	require.False(t, layout.WestWall.Empty())
	require.False(t, layout.EastWall.Empty())
	require.False(t, layout.Center.Empty())
}

func TestCalcTableLoose(t *testing.T) {
	layout, ok := CalcTable(140, 40)
	require.True(t, ok)
	require.False(t, layout.Compact)

	require.False(t, layout.NorthLabel.Empty())
	require.False(t, layout.SouthLabel.Empty())
}

func TestVisualWidthCJK(t *testing.T) {
	require.Equal(t, 4, VisualWidth("一万"))
	require.Equal(t, 2, VisualWidth("一"))
	require.Equal(t, 2, VisualWidth("万"))
	require.Equal(t, 4, VisualWidth("东风"))
	require.Equal(t, 2, VisualWidth("牌"))
	require.Equal(t, 1, VisualWidth("a"))
	require.Equal(t, 1, VisualWidth("●"))
}

func TestHiddenGlyphIsChinese(t *testing.T) {
	require.Equal(t, "牌", HiddenGlyph())
}
