package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRenderLinesGoldenFragments(t *testing.T) {
	scenarios := map[string]RoomView{
		"startup":    sampleView(stageIdle),
		"exchange":   sampleView(stageExchange),
		"quemen":     sampleView(stageQueMen),
		"discard":    sampleView(stageDiscard),
		"claim":      sampleView(stageClaim),
		"hu":         sampleView("hu"),
		"settlement": sampleView(stageSettlement),
		"reconnect":  sampleReconnectView(),
	}
	for name, view := range scenarios {
		name, view := name, view
		t.Run(name, func(t *testing.T) {
			rendered := strings.Join(RenderLines(view, RenderOptions{Width: 96, Height: 32}), "\n")
			data, err := os.ReadFile(filepath.Join("testdata", "golden", name+".txt")) // #nosec G304：name 来自固定场景表。
			require.NoError(t, err)
			for _, fragment := range strings.Split(string(data), "\n") {
				fragment = strings.TrimSpace(fragment)
				if fragment == "" {
					continue
				}
				require.Contains(t, rendered, fragment)
			}
		})
	}
}

func sampleView(stage string) RoomView {
	v := RoomView{
		UserID:          "u0",
		Nickname:        "我",
		RoomID:          "r1",
		SeatIndex:       0,
		Stage:           stage,
		DealerSeat:      0,
		ActingSeat:      0,
		PendingTile:     "m9",
		AvailableAction: []string{"discard"},
		ClaimCandidates: map[int32][]string{},
		Log:             []LogEntry{{At: time.Unix(0, 0), Text: "测试事件"}},
	}
	v.QueBySeat = [4]int32{0, 1, 2, -1}
	v.Players[0] = PlayerView{UserID: "u0", Hand: []string{"m1", "m2", "p3", "s9"}, HandCnt: 4, Discards: []string{"p9"}}
	v.Players[1] = PlayerView{UserID: "u1", HandCnt: 13, Discards: []string{"m3"}}
	v.Players[2] = PlayerView{UserID: "u2", HandCnt: 13, Melds: []string{"pong:m5"}}
	v.Players[3] = PlayerView{UserID: "u3", HandCnt: 13, Discards: []string{"s7"}}
	if stage == stageClaim {
		v.AvailableAction = []string{"pong", "hu"}
	}
	return v
}

func sampleReconnectView() RoomView {
	v := sampleView(stageDiscard)
	v.Reconnecting = true
	v.Connected = false
	v.LastError = "重连中"
	return v
}
