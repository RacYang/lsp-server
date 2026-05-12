package main

import (
	"context"
	"strings"
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

// TestPlayerJourney_L2_3_LobbyHasNoProtocolIDs 锁定大厅场景对玩家不暴露协议字段。
//
// 玩家旅程规范要求大厅界面只展示规则显示名、房名、人数与可读化的状态文案，
// 不得把规则编号、翻页游标、请求编号等内部字段直接渲染出来。这条规则确保
// 玩家不会在大厅看到诸如下划线全小写命名的字段，从而保留产品语言的整洁。
//
// 本用例构造一桌公开房间数据，让规则编号与翻页游标都进入场景状态，
// 然后渲染大厅主屏并断言屏幕文本不含任何协议字段子串；若回归后又把规则
// 编号意外渲染到玩家面前，本用例会立刻报错并打印实际帧文本辅助定位。
func TestPlayerJourney_L2_3_LobbyHasNoProtocolIDs(t *testing.T) {
	scr := makeSimScreen(t, 100, 30)
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseLobby
		v.Connected = true
	})
	router := NewSceneRouter(state, fakeSceneLobbyGateway{}, fakeSceneTableGateway{}, &Config{TileTheme: tileThemeASCII})
	router.lobby.rooms = []LobbyRoomMeta{
		{RoomID: "ROOM1", DisplayName: "Alice 的局", Players: 2, Capacity: 4, RuleID: "sichuan_xuezhandaodi_huansanzhang"},
	}
	router.lobby.nextPage = "cursor:abc"
	router.Render(scr, time.Unix(0, 0))

	frame := dumpScreen(scr)
	forbidden := []string{
		"sichuan_xuezhandaodi_huansanzhang",
		"sichuan_xuezhandaodi_biaozhun",
		"rule_id",
		"page_token",
		"req_id",
		"cursor:abc",
	}
	for _, needle := range forbidden {
		if strings.Contains(frame, needle) {
			t.Fatalf("[L2.3] 大厅渲染不得包含协议 ID/分页 token: %q\nframe=%q", needle, frame)
		}
	}
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

func TestSceneRouterOpeningExchangeUsesTableScene(t *testing.T) {
	state := NewAppState("racoo")
	state.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{UserId: "u0"}}})
	state.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_AutoMatchResp{AutoMatchResp: &clientv1.AutoMatchResponse{RoomId: "ROOM1", SeatIndex: 0}}})
	state.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_InitialDeal{InitialDeal: &clientv1.InitialDealNotify{
		SeatIndex: 0,
		Tiles:     []string{"m1", "m2", "m3", "m4", "m5"},
	}}})
	state.Apply(&clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{
		SeatIndex:   0,
		Action:      "exchange_three",
		ActingSeats: []int32{0, 1, 2, 3},
	}}})

	router := NewSceneRouter(state, fakeSceneLobbyGateway{}, fakeSceneTableGateway{}, &Config{TileTheme: tileThemeASCII})
	if got := router.CurrentSceneID(); got != SceneTable {
		t.Fatalf("opening exchange should render table, got %s", got)
	}
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

// renderPrepFrame 构造预备页快照并返回帧文本，便于多个 [Gxx]/[L5.x]/[P4.x] 用例复用。
// 通过 mutate 注入测试想要的 RoomView 状态，覆盖 AutoPlay/Surrendered/Private 等开关。
func renderPrepFrame(t *testing.T, mutate func(v *RoomView)) string {
	t.Helper()
	scr := makeSimScreen(t, 100, 30)
	state := NewAppState("racoo")
	state.Mutate(func(v *RoomView) {
		v.Phase = phaseTable
		v.RoomID = "PRIV-AB12CD"
		v.RuleID = "sichuan_xuezhandaodi_huansanzhang"
		v.DisplayName = "四川血战到底（换三张）"
		v.SeatIndex = 0
		v.Connected = true
		v.Players[0] = PlayerView{Nickname: "racoo", Online: true}
		v.Players[1] = PlayerView{Nickname: "alice", Online: true}
		v.Players[2] = PlayerView{Nickname: "bob", Online: true}
		v.Players[3] = PlayerView{Nickname: "carol", Online: true}
		mutate(v)
	})
	router := NewSceneRouter(state, fakeSceneLobbyGateway{}, fakeSceneTableGateway{}, &Config{TileTheme: tileThemeASCII})
	router.Render(scr, time.Unix(0, 0))
	return dumpScreen(scr)
}

// TestPlayerJourney_G12_NoAutoPlayMark 锁定 cli 渲染层下线托管图标 ◐ 与「托管」字样。
//
// 玩家旅程 v0.5 把「托管」整体作为一项独立功能延后；在它正式落地之前，
// 不论服务端 SeatInfo.auto_play 如何变化，客户端都必须只显示在线/离线/弃局/
// 已胡/机器人/空座这六个状态，绝不能再绘制半月形托管图标 ◐ 或文字「托管」。
//
// 本用例直接把座位 AutoPlay 字段写为 true，渲染预备页一帧，断言这帧文本
// 既不包含 ◐ 也不包含「托管」二字。若回归后再次把 AutoPlay 引入渲染分支，
// 本用例会立刻报错，提示玩家旅程 [G12] 条款被破坏。
func TestPlayerJourney_G12_NoAutoPlayMark(t *testing.T) {
	frame := renderPrepFrame(t, func(v *RoomView) {
		v.Players[0].AutoPlay = true
		v.Players[1].AutoPlay = true
		v.Players[2].AutoPlay = true
		v.Players[3].AutoPlay = true
	})
	if strings.Contains(frame, "◐") {
		t.Fatalf("[G12] 预备页不得渲染托管图标 ◐，frame=%q", frame)
	}
	if strings.Contains(frame, "托管") {
		t.Fatalf("[G12] 预备页不得渲染「托管」字样，frame=%q", frame)
	}
}

// TestPlayerJourney_G13_SurrenderRendersTriangle 锁定弃局态独立渲染为 ▲。
//
// 玩家旅程 [G13] 要求把弃局与托管彻底分开：之前 dialog_overlay 把 Surrendered
// 误标为「托管中」会同时违反 [G12] 与 [G13]。本用例显式触发 Surrendered=true，
// 渲染预备页并断言屏幕含 ▲ 图标，同时不含「托管」二字，覆盖 [G13] 与 [G12]。
func TestPlayerJourney_G13_SurrenderRendersTriangle(t *testing.T) {
	frame := renderPrepFrame(t, func(v *RoomView) {
		v.Players[1].Surrendered = true
		v.Players[1].Status = "surrendered"
	})
	if !strings.Contains(frame, "▲") {
		t.Fatalf("[G13] 弃局座位必须渲染 ▲ 图标，frame=%q", frame)
	}
	if strings.Contains(frame, "托管") {
		t.Fatalf("[G13] 弃局态不得渲染「托管」字样，frame=%q", frame)
	}
}

// TestPlayerJourney_L5_2_PrivateRoomCodeVisibleInPrep 锁定私密房房间码必须醒目持续展示。
//
// 玩家旅程 [L5.2] 要求私密房创建后，预备页必须把房间码作为分享凭据持续显示。
// 本用例打开 Private 标记后渲染一帧，断言屏幕同时含房间码本身、明确的「私密」
// 文案与高亮前缀 ★；若任何一项缺失都意味着分享体验被破坏。
func TestPlayerJourney_L5_2_PrivateRoomCodeVisibleInPrep(t *testing.T) {
	frame := renderPrepFrame(t, func(v *RoomView) { v.Private = true })
	for _, needle := range []string{"PRIV-AB12CD", "私密", "★"} {
		if !strings.Contains(frame, needle) {
			t.Fatalf("[L5.2] 私密房预备页必须含 %q，frame=%q", needle, frame)
		}
	}
}

// TestPlayerJourney_P4_2_PrivateRoomCodePersistsUntilPlaying 覆盖 waiting 与 ready 两个稳定态。
//
// 玩家旅程 [P4.2] 要求私密房房间码在整个预备阶段持续展示，无论 SeatRoster
// 经历多少次准备/取消准备的扰动。本用例分别在 waiting 与 ready 两个 RoomState
// 下渲染预备页，断言两帧都仍然含房间码；进入 playing 后 cli 已自然切到 round
// 场景，本断言不再适用。
func TestPlayerJourney_P4_2_PrivateRoomCodePersistsUntilPlaying(t *testing.T) {
	for _, stage := range []string{"waiting", "ready"} {
		frame := renderPrepFrame(t, func(v *RoomView) {
			v.Private = true
			v.RoomState = stage
			if stage == "ready" {
				for i := range v.Players {
					v.Players[i].Ready = true
					v.Players[i].Status = "ready"
				}
			}
		})
		if !strings.Contains(frame, "PRIV-AB12CD") {
			t.Fatalf("[P4.2] stage=%s 私密房码必须持续可见，frame=%q", stage, frame)
		}
	}
}
