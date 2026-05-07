package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/require"
)

// dumpScreen 把 SimulationScreen 当前内容转成可读多行字符串。
//
// 双宽字符（CJK 表意、全角标点、Emoji 等）会占用 2 个 cell；tcell 在第二个
// cell 留空 rune,我们按 uniseg.RuneWidth 判定首 cell 的宽度后跳过紧随的尾 cell,
// 保持与生产渲染（drawText / visualWidth）完全一致的 cell 占用判定,
// 避免出现"老 dumpScreen 只识别 CJK 表意,新生产代码识别全角标点"的偏差。
// 空 cell 用空格代替。末尾空白会被裁剪以稳定 golden（避免行末空白噪声）。
func dumpScreen(scr tcell.SimulationScreen) string {
	cells, w, h := scr.GetContents()
	var out strings.Builder
	for y := 0; y < h; y++ {
		line := make([]rune, 0, w)
		skipTail := false
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if skipTail {
				skipTail = false
				continue
			}
			var r rune
			if len(c.Runes) > 0 && c.Runes[0] != 0 {
				r = c.Runes[0]
			} else {
				r = ' '
			}
			line = append(line, r)
			if uniseg.StringWidth(string(r)) >= 2 {
				skipTail = true
			}
		}
		out.WriteString(strings.TrimRight(string(line), " "))
		out.WriteByte('\n')
	}
	return out.String()
}

// goldenPath 返回 testdata/golden/<name>.golden 的绝对路径。
func goldenPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", "golden", name+".golden")
}

// requireGolden 比对实际输出与 golden 文件；环境变量 UPDATE_GOLDEN=1 时覆盖写盘。
func requireGolden(t *testing.T, name, actual string) {
	t.Helper()
	path := goldenPath(t, name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(actual), 0o600))
		return
	}
	want, err := os.ReadFile(path) // #nosec G304: 测试路径由代码常量构造。
	require.NoErrorf(t, err, "golden %s 不存在,请用 UPDATE_GOLDEN=1 重生成", path)
	require.Equal(t, string(want), actual)
}

// makeSimScreen 创建大小固定的 SimulationScreen 用于 golden 测试。
func makeSimScreen(t *testing.T, w, h int) tcell.SimulationScreen {
	t.Helper()
	scr := tcell.NewSimulationScreen("UTF-8")
	require.NoError(t, scr.Init())
	scr.SetSize(w, h)
	t.Cleanup(scr.Fini)
	return scr
}

func newWaitingTableView() RoomView {
	v := RoomView{
		Nickname:      "racoo",
		Phase:         phaseTable,
		SeatIndex:     1,
		ActingSeat:    -1,
		WaitingAction: "none",
	}
	v.Players[0].Nickname = "carl"
	v.Players[0].HandCnt = 13
	v.Players[1].Nickname = "racoo"
	v.Players[1].HandCnt = 13
	v.Players[2].Nickname = "alice"
	v.Players[2].HandCnt = 13
	v.Players[3].Nickname = "bob"
	v.Players[3].HandCnt = 13
	v.Players[1].Hand = []string{"m1", "m2", "m3", "m5", "m5", "p5", "p6", "p7", "s1", "s2", "s3", "s5", "s9"}
	for i := range v.QueBySeat {
		v.QueBySeat[i] = -1
	}
	return v
}

func TestRenderFrameWaitingASCII(t *testing.T) {
	scr := makeSimScreen(t, MinTableWidth, MinTableHeight)
	view := newWaitingTableView()
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	RenderFrame(scr, FrameInputs{View: view, Layout: layout, Theme: TileThemeASCII})
	scr.Show()
	requireGolden(t, "table_waiting_ascii", dumpScreen(scr))
}

func TestRenderFrameMyTurnDiscardASCII(t *testing.T) {
	scr := makeSimScreen(t, MinTableWidth, MinTableHeight)
	view := newWaitingTableView()
	view.ActingSeat = view.SeatIndex
	view.WaitingAction = "discard"
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	RenderFrame(scr, FrameInputs{View: view, Layout: layout, Theme: TileThemeASCII})
	scr.Show()
	requireGolden(t, "table_my_turn_discard_ascii", dumpScreen(scr))
}

func TestRenderFrameWithCursorSelectedASCII(t *testing.T) {
	scr := makeSimScreen(t, MinTableWidth, MinTableHeight)
	view := newWaitingTableView()
	view.ActingSeat = view.SeatIndex
	view.WaitingAction = "discard"
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	cursor := &HandCursor{Mode: CursorModeSingle, Index: 12}
	RenderFrame(scr, FrameInputs{View: view, Layout: layout, Theme: TileThemeASCII, Cursor: cursor})
	scr.Show()
	requireGolden(t, "table_cursor_selected_ascii", dumpScreen(scr))
}

func TestRenderFrameWithCursorPendingASCII(t *testing.T) {
	scr := makeSimScreen(t, MinTableWidth, MinTableHeight)
	view := newWaitingTableView()
	view.ActingSeat = view.SeatIndex
	view.WaitingAction = "discard"
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	cursor := &HandCursor{Mode: CursorModeSingle, Index: 12, Pending: true}
	RenderFrame(scr, FrameInputs{View: view, Layout: layout, Theme: TileThemeASCII, Cursor: cursor})
	scr.Show()
	requireGolden(t, "table_cursor_pending_ascii", dumpScreen(scr))
}

func TestRenderFrameMyTurnDiscardUnicode(t *testing.T) {
	// CJK 主题下双宽字符不能错位；dumpScreen 把双宽字符的尾 cell 跳过，golden 仍稳定可比。
	scr := makeSimScreen(t, MinTableWidth, MinTableHeight)
	view := newWaitingTableView()
	view.ActingSeat = view.SeatIndex
	view.WaitingAction = "discard"
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	RenderFrame(scr, FrameInputs{View: view, Layout: layout, Theme: TileThemeUnicode})
	scr.Show()
	requireGolden(t, "table_my_turn_discard_unicode", dumpScreen(scr))
}

func TestRenderFrameWaitingWideASCII(t *testing.T) {
	scr := makeSimScreen(t, 120, MinTableHeight)
	view := newWaitingTableView()
	view.Players[2].Discards = []string{"m1", "m2", "m3", "m4", "m5", "m6"}
	view.Players[3].Discards = []string{"p1", "p2", "p3", "p4", "p5", "p6"}
	layout, ok := CalcLayout(120, MinTableHeight)
	require.True(t, ok)
	RenderFrame(scr, FrameInputs{View: view, Layout: layout, Theme: TileThemeASCII})
	scr.Show()
	requireGolden(t, "table_waiting_wide_ascii", dumpScreen(scr))
}

func TestRenderFrameWaitingWideUnicode(t *testing.T) {
	scr := makeSimScreen(t, 120, MinTableHeight)
	view := newWaitingTableView()
	view.Players[2].Discards = []string{"m1", "m2", "m3", "m4", "m5", "m6"}
	view.Players[3].Discards = []string{"p1", "p2", "p3", "p4", "p5", "p6"}
	layout, ok := CalcLayout(120, MinTableHeight)
	require.True(t, ok)
	RenderFrame(scr, FrameInputs{View: view, Layout: layout, Theme: TileThemeUnicode})
	scr.Show()
	requireGolden(t, "table_waiting_wide_unicode", dumpScreen(scr))
}

func TestRenderFrameSelfMeldsRendered(t *testing.T) {
	// 自己也有碰/杠时,SelfMeldsArea 应该把它们渲染在手牌上方,
	// 与对家/上家/下家的"鸣: ..."区块对齐,避免玩家看不到自己的鸣牌。
	scr := makeSimScreen(t, MinTableWidth, MinTableHeight)
	view := newWaitingTableView()
	view.ActingSeat = view.SeatIndex
	view.WaitingAction = "discard"
	view.Players[view.SeatIndex].Melds = []string{"pong:p5", "gang:m9"}
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	RenderFrame(scr, FrameInputs{View: view, Layout: layout, Theme: TileThemeASCII})
	scr.Show()
	requireGolden(t, "table_self_melds_ascii", dumpScreen(scr))
}

func TestRenderFrameSelfDiscardsRendered(t *testing.T) {
	// 自己也已经打过牌时,SelfDiscardsArea 应该把"打: ..."渲染在自家鸣牌正上方,
	// 与对家/上家/下家的"打: ..."区块对称,避免界面在 self 这一侧"少一行"。
	scr := makeSimScreen(t, MinTableWidth, MinTableHeight)
	view := newWaitingTableView()
	view.ActingSeat = view.SeatIndex
	view.WaitingAction = "discard"
	view.Players[view.SeatIndex].Melds = []string{"pong:p5"}
	view.Players[view.SeatIndex].Discards = []string{"m9", "s2", "p3", "m1", "s7", "z5"}
	layout, ok := CalcLayout(MinTableWidth, MinTableHeight)
	require.True(t, ok)
	RenderFrame(scr, FrameInputs{View: view, Layout: layout, Theme: TileThemeASCII})
	scr.Show()
	requireGolden(t, "table_self_discards_ascii", dumpScreen(scr))
}

func TestCentralPromptStates(t *testing.T) {
	v := newWaitingTableView()
	require.Equal(t, "已自动准备,等待其他玩家就位", centralPrompt(v, nil))

	v.ActingSeat = v.SeatIndex
	v.WaitingAction = "discard"
	require.Equal(t, "◆ 该 你 出牌 ◆", centralPrompt(v, nil))

	v.ActingSeat = v.SeatIndex
	v.WaitingAction = "que_men"
	require.Contains(t, centralPrompt(v, nil), "定缺")

	v.ActingSeat = 0
	v.WaitingAction = "discard"
	require.Contains(t, centralPrompt(v, nil), "等待 carl")

	v.ActingSeat = v.SeatIndex
	v.WaitingAction = "discard"
	cursor := &HandCursor{Mode: CursorModeSingle, Index: 0}
	require.Contains(t, centralPrompt(v, cursor), "已选 一万")
	cursor.Pending = true
	require.Contains(t, centralPrompt(v, cursor), "出牌中")

	v.WaitingAction = "exchange_three"
	multi := &HandCursor{Mode: CursorModeMulti3, Index: 0, Marked: []int{1, 2}}
	require.Contains(t, centralPrompt(v, multi), "还需 1")
	multi.Marked = []int{1, 2, 3}
	require.Contains(t, centralPrompt(v, multi), "已选 3 张")
}

func TestPrettifyMeld(t *testing.T) {
	require.Equal(t, "[五筒]碰", prettifyMeld("pong:p5"))
	require.Equal(t, "[五万]杠", prettifyMeld("gang:m5"))
	require.Equal(t, "[一万 二万 三万]吃", prettifyMeld("chow:m1 m2 m3"))
	require.Equal(t, "raw", prettifyMeld("raw"))
}
