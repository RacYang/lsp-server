package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func TestBuildTableFrontendModelUsesHandCountsForOpponents(t *testing.T) {
	for self := int32(0); self < 4; self++ {
		view := RoomView{
			Phase:       phaseTable,
			SeatIndex:   self,
			RoomState:   "playing",
			ActingSeat:  self,
			ActingSeats: []int32{self},
		}
		for seat := int32(0); seat < 4; seat++ {
			view.Players[seat] = PlayerView{
				UserID:   "u",
				Nickname: "p",
				HandCnt:  13 + int(seat%2),
			}
		}
		view.Players[self].Hand = []string{"m1", "m2", "m3"}
		view.Players[self].HandCnt = len(view.Players[self].Hand)

		model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
		require.Equal(t, self, model.Seats[0].AbsSeat)
		require.Equal(t, view.Players[(self+1)%4].HandCnt, model.Seats[1].HandCount)
		require.Equal(t, view.Players[(self+2)%4].HandCnt, model.Seats[2].HandCount)
		require.Equal(t, view.Players[(self+3)%4].HandCnt, model.Seats[3].HandCount)
	}
}

func TestBuildTableFrontendModelClaimWindowOnlyForCandidate(t *testing.T) {
	view := RoomView{
		Phase:         phaseTable,
		SeatIndex:     0,
		RoomState:     "playing",
		ActingSeat:    2,
		ActingSeats:   []int32{2},
		LastStep:      8,
		RoundPhase:    clientv1.Phase_PHASE_CLAIM,
		WaitingAction: "claim_window",
		PendingTile:   "p3",
		ClaimCandidates: map[int32][]string{
			2: {"pong", "pass"},
		},
	}
	require.Nil(t, BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0)).ActionWindow)

	view.SeatIndex = 2
	model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
	require.NotNil(t, model.ActionWindow)
	require.Equal(t, []PlayerAction{ActionPong, ActionPass}, model.AllowedActions)
	require.Equal(t, ActionWindowClaim, model.ActionWindow.Kind)
}

func TestBuildTableFrontendModelClaimWindowSupportsChi(t *testing.T) {
	view := RoomView{
		Phase:         phaseTable,
		SeatIndex:     1,
		RoomState:     "playing",
		ActingSeat:    1,
		ActingSeats:   []int32{1},
		LastStep:      9,
		RoundPhase:    clientv1.Phase_PHASE_CLAIM,
		WaitingAction: "claim_window",
		PendingTile:   "m2",
		ClaimCandidates: map[int32][]string{
			1: {"chi", "pass"},
		},
	}
	model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
	require.NotNil(t, model.ActionWindow)
	require.Equal(t, []PlayerAction{ActionChi, ActionPass}, model.AllowedActions)
	require.Equal(t, []ClaimAction{ClaimActionChow, ClaimActionPass}, model.ActionWindow.Actions)
	require.Contains(t, model.KeyHint, "c 吃")
}

func TestBuildTableFrontendModelTsumoWindow(t *testing.T) {
	view := RoomView{
		Phase:            phaseTable,
		SeatIndex:        1,
		RoomState:        "playing",
		ActingSeat:       1,
		ActingSeats:      []int32{1},
		LastStep:         12,
		RoundPhase:       clientv1.Phase_PHASE_TSUMO,
		WaitingAction:    "tsumo_window",
		PendingTile:      "m9",
		AvailableActions: []string{"hu", "pass"},
	}
	model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
	require.NotNil(t, model.ActionWindow)
	require.Equal(t, ActionWindowTsumo, model.ActionWindow.Kind)
	require.Equal(t, []PlayerAction{ActionHu, ActionPass}, model.AllowedActions)
	require.Equal(t, ClaimActionHu, model.ActionWindow.Actions[0])
}

func TestBuildTableFrontendModelClaimHuPongDefaultsToHu(t *testing.T) {
	view := RoomView{
		Phase:         phaseTable,
		SeatIndex:     0,
		RoomState:     "playing",
		ActingSeat:    0,
		ActingSeats:   []int32{0},
		RoundPhase:    clientv1.Phase_PHASE_CLAIM,
		WaitingAction: "claim_window",
		PendingTile:   "s2",
		ClaimCandidates: map[int32][]string{
			0: {"hu", "pong", "pass"},
		},
	}
	model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
	require.NotNil(t, model.ActionWindow)
	require.Equal(t, []PlayerAction{ActionHu, ActionPong, ActionPass}, model.AllowedActions)
	require.Equal(t, []ClaimAction{ClaimActionHu, ClaimActionPong, ClaimActionPass}, model.ActionWindow.Actions)
	require.Equal(t, 0, model.ActionWindow.Selected)
}

func TestBuildTableFrontendModelHuedSelfCannotDiscard(t *testing.T) {
	view := RoomView{
		Phase:            phaseTable,
		SeatIndex:        0,
		RoomState:        "playing",
		ActingSeat:       0,
		ActingSeats:      []int32{0},
		RoundPhase:       clientv1.Phase_PHASE_DISCARD,
		WaitingAction:    "discard",
		AvailableActions: []string{"discard"},
	}
	view.Players[0] = PlayerView{
		Nickname: "self",
		Hand:     []string{"m1", "m2"},
		HandCnt:  2,
		Hued:     true,
	}

	model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
	require.Empty(t, model.AllowedActions)
	require.Equal(t, "你已胡牌，等待本局结束", model.DisabledReason)
	require.Equal(t, "你已胡牌，等待本局结束", model.Prompt)
	require.Equal(t, CursorModeNone, DeriveCursorMode(view))
}

func TestPromptFromModel(t *testing.T) {
	baseNow := time.Unix(1000, 0)

	fullView := RoomView{Phase: phaseTable, RoomState: "playing", ActingSeat: 1, ActingSeats: []int32{1}}
	for i := range fullView.Players {
		fullView.Players[i] = PlayerView{UserID: fmt.Sprintf("u%d", i), Nickname: fmt.Sprintf("p%d", i)}
	}

	tests := []struct {
		desc  string
		view  RoomView
		local TableLocalUI
		model TableFrontendModel
		now   time.Time
		want  string
	}{
		{
			desc: "活跃 UXNotice 优先返回",
			view: RoomView{
				UXNotice:      "连接不稳定",
				UXNoticeUntil: baseNow.Add(5 * time.Second),
			},
			model: TableFrontendModel{PendingFeedback: "其它提示"},
			now:   baseNow,
			want:  "连接不稳定",
		},
		{
			desc:  "PendingFeedback 次优先",
			view:  RoomView{},
			model: TableFrontendModel{PendingFeedback: "操作已收到"},
			now:   baseNow,
			want:  "操作已收到",
		},
		{
			desc: "ActionWindow 存在时提示决定",
			view: RoomView{},
			model: TableFrontendModel{
				ActionWindow:   &ActionWindowModel{},
				AllowedActions: []PlayerAction{ActionPong, ActionPass},
			},
			now:  baseNow,
			want: "请决定：碰 / 过",
		},
		{
			desc: "DisabledReason 且无可用动作时直接展示",
			view: RoomView{},
			model: TableFrontendModel{
				DisabledReason: "你已胡牌，等待本局结束",
				AllowedActions: nil,
			},
			now:  baseNow,
			want: "你已胡牌，等待本局结束",
		},
		{
			desc:  "RoomPrep 有空位",
			view:  RoomView{},
			model: TableFrontendModel{ScreenPhase: TableScreenRoomPrep},
			now:   baseNow,
			want:  "座位未满 - b 补一个 / B 补满",
		},
		{
			desc:  "RoomPrep 人已坐齐",
			view:  fullView,
			model: TableFrontendModel{ScreenPhase: TableScreenRoomPrep},
			now:   baseNow,
			want:  "人已坐齐：按 Enter 准备开局",
		},
		{
			desc:  "Settlement 阶段",
			view:  RoomView{},
			model: TableFrontendModel{ScreenPhase: TableScreenSettlement},
			now:   baseNow,
			want:  "本局结束",
		},
		{
			desc:  "换三张：提示已选数量",
			view:  RoomView{Phase: phaseTable, RoomState: "playing", WaitingAction: "exchange_three"},
			local: TableLocalUI{Cursor: HandCursor{Marked: []int{0, 1}}},
			model: TableFrontendModel{ScreenPhase: TableScreenPlaying},
			now:   baseNow,
			want:  "换三张：已选 2/3，须同花色",
		},
		{
			desc:  "定缺提示",
			view:  RoomView{Phase: phaseTable, RoomState: "playing", WaitingAction: "que_men"},
			model: TableFrontendModel{ScreenPhase: TableScreenPlaying},
			now:   baseNow,
			want:  "定缺：选一门不要，选定后不可更改",
		},
		{
			desc: "discard 且己方可出牌",
			view: RoomView{Phase: phaseTable, RoomState: "playing", WaitingAction: "discard"},
			model: TableFrontendModel{
				ScreenPhase:    TableScreenPlaying,
				AllowedActions: []PlayerAction{ActionDiscard},
			},
			now:  baseNow,
			want: "轮到你：选择一张牌打出",
		},
		{
			desc: "discard 但己方无法出牌（等待他人）",
			view: RoomView{
				Phase:         phaseTable,
				RoomState:     "playing",
				WaitingAction: "discard",
				ActingSeat:    2,
				ActingSeats:   []int32{2},
			},
			model: TableFrontendModel{ScreenPhase: TableScreenPlaying, AllowedActions: nil},
			now:   baseNow,
			want:  "等待对家操作",
		},
		{
			desc: "claim_window 等待阶段",
			view: RoomView{
				Phase:         phaseTable,
				RoomState:     "playing",
				WaitingAction: "claim_window",
				ActingSeat:    3,
				ActingSeats:   []int32{3},
			},
			model: TableFrontendModel{ScreenPhase: TableScreenPlaying, AllowedActions: nil},
			now:   baseNow,
			want:  "等待上家操作",
		},
		{
			desc:  "默认返回等待开始",
			view:  RoomView{},
			model: TableFrontendModel{ScreenPhase: TableScreenPlaying},
			now:   baseNow,
			want:  "等待开始",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got := promptFromModel(tc.view, tc.local, tc.model, tc.now)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestKeyHintFromModel(t *testing.T) {
	baseView := RoomView{Phase: phaseTable, RoomState: "playing", SeatIndex: 0}

	fullView := baseView
	for i := range fullView.Players {
		fullView.Players[i] = PlayerView{UserID: fmt.Sprintf("u%d", i), Nickname: fmt.Sprintf("p%d", i)}
	}

	tests := []struct {
		desc  string
		view  RoomView
		local TableLocalUI
		model TableFrontendModel
		want  string
	}{
		{
			desc: "DisabledReason 且无动作时追加帮助提示",
			view: baseView,
			model: TableFrontendModel{
				DisabledReason: "你已胡牌，等待本局结束",
				AllowedActions: nil,
			},
			want: "你已胡牌，等待本局结束　? 帮助",
		},
		{
			desc: "ActionWindow 存在时返回抢答快捷键",
			view: baseView,
			model: TableFrontendModel{
				ActionWindow:   &ActionWindowModel{},
				AllowedActions: []PlayerAction{ActionPong, ActionPass},
			},
			want: claimKeyHint(TableUXModel{AllowedActions: []PlayerAction{ActionPong, ActionPass}}),
		},
		{
			desc:  "换三张动作提示",
			view:  baseView,
			local: TableLocalUI{Cursor: HandCursor{Marked: []int{0}}},
			model: TableFrontendModel{
				AllowedActions: []PlayerAction{ActionExchangeThree},
				ScreenPhase:    TableScreenPlaying,
			},
			want: "换三张：已选 1/3　←→ 选牌　Space 标记　Enter 确认",
		},
		{
			desc: "定缺动作提示",
			view: baseView,
			model: TableFrontendModel{
				AllowedActions: []PlayerAction{ActionQueMen},
				ScreenPhase:    TableScreenPlaying,
			},
			want: "定缺：←→ 选择缺门　Enter 确认　m/p/s 快捷",
		},
		{
			desc: "出牌且可杠",
			view: baseView,
			model: TableFrontendModel{
				AllowedActions: []PlayerAction{ActionDiscard, ActionGang},
				ScreenPhase:    TableScreenPlaying,
			},
			want: "轮到你：←→ 选牌　Enter 打出　g 杠",
		},
		{
			desc: "仅出牌",
			view: baseView,
			model: TableFrontendModel{
				AllowedActions: []PlayerAction{ActionDiscard},
				ScreenPhase:    TableScreenPlaying,
			},
			want: "轮到你：←→ 选牌　Enter 打出",
		},
		{
			desc: "结算阶段",
			view: baseView,
			model: TableFrontendModel{
				ScreenPhase:    TableScreenSettlement,
				AllowedActions: nil,
			},
			want: "本局结束：r 再开一桌　l 离桌　Enter 停留",
		},
		{
			desc: "RoomPrep 有空位",
			view: RoomView{},
			model: TableFrontendModel{
				ScreenPhase:    TableScreenRoomPrep,
				AllowedActions: nil,
			},
			want: "等人入座：b 补一个机器人　B 补满",
		},
		{
			desc: "RoomPrep 人已坐齐",
			view: fullView,
			model: TableFrontendModel{
				ScreenPhase:    TableScreenRoomPrep,
				AllowedActions: nil,
			},
			want: "准备开局：Enter 确认　? 帮助",
		},
		{
			desc: "默认等待提示",
			view: baseView,
			model: TableFrontendModel{
				ScreenPhase:    TableScreenPlaying,
				AllowedActions: nil,
			},
			want: "等待：? 帮助　i 房间信息　Tab 玩家",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got := keyHintFromModel(tc.view, tc.local, tc.model)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestBuildTableFrontendModelSettlementOverridesHuedWaitingPrompt(t *testing.T) {
	view := RoomView{
		Phase:          phaseTable,
		SeatIndex:      0,
		RoomState:      "settling",
		LastSettlement: &clientv1.SettlementNotify{RoomId: "r1"},
	}
	view.Players[0] = PlayerView{Hued: true}

	model := BuildTableFrontendModel(view, TableLocalUI{}, time.Unix(0, 0))
	require.Equal(t, TableScreenSettlement, model.ScreenPhase)
	require.Empty(t, model.DisabledReason)
	require.Equal(t, "本局结束", model.Prompt)
	require.Equal(t, "本局结束：r 再开一桌　l 离桌　Enter 停留", model.KeyHint)
}
