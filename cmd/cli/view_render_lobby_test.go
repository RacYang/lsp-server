package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func TestRenderLobbyPageGoldenFragments(t *testing.T) {
	scenarios := map[string]string{
		"login":       strings.Join(RenderLoginLines(sampleLoginView(), 80), "\n"),
		"lobby_empty": strings.Join(RenderLobbyLines(sampleLobbyView(false), 96), "\n"),
		"lobby_rooms": strings.Join(RenderLobbyLines(sampleLobbyView(true), 96), "\n"),
	}
	for name, rendered := range scenarios {
		name, rendered := name, rendered
		t.Run(name, func(t *testing.T) {
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

func sampleLoginView() RoomView {
	return RoomView{Nickname: "终端玩家", ServerURL: "wss://racoo.cn/ws", Phase: phaseLogin}
}

func sampleLobbyView(withRooms bool) RoomView {
	v := RoomView{UserID: "u0", Phase: phaseLobby}
	if !withRooms {
		return v
	}
	v.RoomList = []*clientv1.RoomMeta{
		{RoomId: "ROOM01", RuleId: "sichuan_xzdd", DisplayName: "公开桌一", SeatCount: 1, MaxSeats: 4, Stage: "waiting"},
		{RoomId: "ROOM02", RuleId: "sichuan_xzdd", DisplayName: "公开桌二", SeatCount: 3, MaxSeats: 4, Stage: "waiting"},
	}
	return v
}
