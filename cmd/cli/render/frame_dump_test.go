// 逐帧 dump 测试——将牌桌各阶段渲染结果写入文件，便于人工审查。
package render

import (
	"fmt"
	"os"
	"testing"
)

func TestDumpAllPhases(t *testing.T) {
	if os.Getenv("DUMP_FRAMES") != "1" {
		t.Skip("set DUMP_FRAMES=1 to write frame dumps")
	}

	phases := []struct {
		name      string
		phase     string
		prompt    string
		countdown int
	}{
		{"room_prep", "room_prep", "", 0},
		{"playing", "playing", "◆ 该你出牌 ◆", 12},
		{"settlement", "settlement", "", 0},
	}

	for _, p := range phases {
		scr := makeSimScreen(140, 40)
		data := sampleTableData(p.phase)
		data.PhasePrompt = p.prompt
		data.Countdown = p.countdown
		if p.phase == "room_prep" {
			data.Cursor = CursorState{Mode: "none"}
		}

		layout, _ := CalcTable(140, 40)
		DrawTableFrame(scr, layout, data)

		if p.phase == "settlement" {
			scores := []ScoreItem{
				{Label: "你", Value: 2400},
				{Label: "alice", Value: -800},
				{Label: "bob", Value: 1200},
				{Label: "BOT-0", Value: -2800},
			}
			lines := BuildSettlementLines("胡 了 !", []string{"清一色 +6", "对对胡 +2"}, 8, scores)
			DrawDialog(scr, layout.Frame, "本局结算", lines, BorderDouble)
		}
		if p.phase == "room_prep" {
			prepPlayers := [4]PlayerInfo{
				{Name: "racoo", SeatLabel: "南", Status: "√"},
				{Name: "alice", SeatLabel: "西", Status: "●"},
				{Name: "bob", SeatLabel: "北", Status: "√"},
				{Name: "", SeatLabel: "东", Status: "□"},
			}
			prepLines := BuildSeatPrepLines(prepPlayers)
			DrawDialog(scr, layout.Frame, "座位", prepLines, BorderSingle)
		}

		scr.Show()
		dump := dumpScreen(scr)

		path := fmt.Sprintf("testdata/frame_%s.txt", p.name)
		if err := os.WriteFile(path, []byte(dump+"\n"), 0o600); err != nil {
			t.Fatalf("write frame dump: %v", err)
		}
		t.Logf("wrote %s (%d bytes)", path, len(dump))

		scr.Fini()
	}
}
