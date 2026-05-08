package main

import (
	"context"
	"testing"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

type fakeSceneLobbyGateway struct{}

func (fakeSceneLobbyGateway) LeaveRoom(context.Context) error { return nil }

func (fakeSceneLobbyGateway) ListRules(context.Context) ([]LobbyRuleMeta, error) {
	return []LobbyRuleMeta{
		{RuleID: "sichuan_xuezhandaodi_huansanzhang", DisplayName: "四川血战到底（换三张）", ShortDesc: "换三张、定缺、血战到底"},
		{RuleID: "sichuan_xuezhandaodi_biaozhun", DisplayName: "四川血战到底（标准）", ShortDesc: "定缺、血战到底"},
	}, nil
}

func (fakeSceneLobbyGateway) AutoMatch(context.Context, string) (LobbyJoinResult, error) {
	return LobbyJoinResult{RoomID: "ROOM1", SeatIndex: 0}, nil
}

func (fakeSceneLobbyGateway) ListRooms(context.Context, string) (LobbyRoomList, error) {
	return LobbyRoomList{Rooms: []LobbyRoomMeta{{RoomID: "ROOM1", DisplayName: "Alice 的局", Players: 2, Capacity: 4, RuleID: "sichuan_xuezhandaodi_huansanzhang"}}}, nil
}

func (fakeSceneLobbyGateway) CreateRoom(context.Context, LobbyCreateOpts) (LobbyJoinResult, error) {
	return LobbyJoinResult{RoomID: "ROOM2", SeatIndex: 0}, nil
}

func (fakeSceneLobbyGateway) JoinRoom(context.Context, string) (LobbyJoinResult, error) {
	return LobbyJoinResult{RoomID: "ROOM3", SeatIndex: 1}, nil
}

func (fakeSceneLobbyGateway) ChangeNickname(string) {}

type fakeSceneTableGateway struct{}

func (fakeSceneTableGateway) Ready(context.Context) error                          { return nil }
func (fakeSceneTableGateway) Discard(context.Context, string) error                { return nil }
func (fakeSceneTableGateway) ExchangeThree(context.Context, []string, int32) error { return nil }
func (fakeSceneTableGateway) QueMen(context.Context, int32) error                  { return nil }
func (fakeSceneTableGateway) Pong(context.Context) error                           { return nil }
func (fakeSceneTableGateway) Gang(context.Context, string) error                   { return nil }
func (fakeSceneTableGateway) Hu(context.Context) error                             { return nil }
func (fakeSceneTableGateway) Pass(context.Context) error                           { return nil }
func (fakeSceneTableGateway) LeaveRoom(context.Context) error                      { return nil }
func (fakeSceneTableGateway) AddBot(context.Context, int32) ([]*clientv1.SeatInfo, error) {
	return nil, nil
}

func TestSceneRouterRenderLobby(t *testing.T) {
	scr := makeSimScreen(t, 100, 30)
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseLobby
		v.Connected = true
	})
	router := NewSceneRouter(state, fakeSceneLobbyGateway{}, fakeSceneTableGateway{}, &Config{TileTheme: tileThemeASCII})
	router.lobby.rooms = []LobbyRoomMeta{{RoomID: "ROOM1", DisplayName: "Alice 的局", Players: 2, Capacity: 4}}
	router.Render(scr, time.Unix(0, 0))
	requireGolden(t, "lobby_main_ascii", dumpScreen(scr))
}

func TestSceneRouterRenderRoomPrep(t *testing.T) {
	scr := makeSimScreen(t, 100, 30)
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "ROOM1"
		v.RuleID = "sichuan_xuezhandaodi_huansanzhang"
		v.DisplayName = "四川血战到底（换三张）"
		v.SeatIndex = 0
		v.Connected = true
		v.Players[0] = PlayerView{Nickname: "racoo", Online: true}
		v.Players[1] = PlayerView{Nickname: "bot", IsBot: true, Online: true}
	})
	router := NewSceneRouter(state, fakeSceneLobbyGateway{}, fakeSceneTableGateway{}, &Config{TileTheme: tileThemeASCII})
	router.Render(scr, time.Unix(0, 0))
	requireGolden(t, "room_prep_ascii", dumpScreen(scr))
}

func TestSceneRouterRenderSettle(t *testing.T) {
	scr := makeSimScreen(t, 100, 30)
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "ROOM1"
		v.SeatIndex = 0
		v.RoomState = "settling"
		v.LastSettlement = &clientv1.SettlementNotify{
			RoomId:        "ROOM1",
			WinnerUserIds: []string{"u1"},
			TotalFan:      8,
			SeatScores: []*clientv1.SeatScore{
				{SeatIndex: 0, UserId: "u1", TotalFan: 8},
				{SeatIndex: 1, UserId: "u2", TotalFan: -8},
			},
			PerWinnerBreakdown: []*clientv1.WinnerBreakdown{{SeatIndex: 0, UserId: "u1", Fan: 8, FanNames: []string{"清一色"}}},
		}
		v.UserID = "u1"
		v.Players[0].Nickname = "racoo"
		v.Players[1].Nickname = "alice"
	})
	router := NewSceneRouter(state, fakeSceneLobbyGateway{}, fakeSceneTableGateway{}, &Config{TileTheme: tileThemeASCII})
	router.Render(scr, time.Unix(0, 0))
	requireGolden(t, "settle_ascii", dumpScreen(scr))
}
