// actor 包白盒测试：单元测试 Actor 内部调度逻辑。
package actor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	_ "racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
	eng "racoo.cn/lsp/internal/service/room/engine"
)

// testRuleState 构造四川血战测试用 RuleState。
func testRuleState(missing []int32) rules.RuleState {
	state := struct {
		MissingSuits []int32                  `json:"missing_suits,omitempty"`
		Submitted    map[string][]bool        `json:"submitted,omitempty"`
		Selections   map[string][][]tile.Tile `json:"selections,omitempty"`
		Direction    map[string]int32         `json:"direction,omitempty"`
	}{
		MissingSuits: append([]int32(nil), missing...),
		Selections:   map[string][][]tile.Tile{},
		Direction:    map[string]int32{"sichuan.exchange": -1},
	}
	data, err := json.Marshal(state)
	if err != nil {
		panic(err)
	}
	return rules.RuleState{Data: data}
}

func TestActorSupportsConfiguredNextHand(t *testing.T) {
	t.Parallel()

	r := domainroom.NewRoom("room-next-hand")
	r.MaxHands = 2
	for i := 0; i < 4; i++ {
		_, ok := r.JoinAutoSeat("p" + string(rune('0'+i)))
		require.True(t, ok)
		require.NoError(t, r.SetReady(domainroom.Seat(i), true))
	}
	require.NoError(t, r.StartPlaying())
	a := &Actor{
		Room:  r,
		round: &RoundState{},
	}
	a.closeRoomAfterRound()
	a.round = nil
	require.Nil(t, a.round)
	require.Equal(t, domainroom.StateWaiting, a.Room.FSM.State())
	require.EqualValues(t, 2, a.Room.HandIndex)
}

func TestSubmitActionReturnsCtxErrAfterContextCanceled(t *testing.T) {
	// 行为变更说明（FIX-1）：Submit* 发送成功后立即释放 submitMu，等待 <-res 时已无锁。
	// ctx 取消后 select 优先返回 ctx.Err()，不再持锁等待 actor 的迟到响应。
	t.Parallel()

	a := &Actor{ch: make(chan any)}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		msg := (<-a.ch).(cmdDiscard)
		cancel()
		time.Sleep(10 * time.Millisecond)
		msg.res <- actionResult{notifications: []Notification{{Kind: eng.KindAction}}, err: nil}
	}()

	notifs, err := a.SubmitAction(ctx, cmdDiscard{userID: "u1", tile: "m1", res: make(chan actionResult, 1)})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, notifs)
}

func TestSubmitActionReturnsRateLimitedWhenMailboxFull(t *testing.T) {
	t.Parallel()

	a := &Actor{ch: make(chan any, 1)}
	a.ch <- cmdReady{}
	notifs, err := a.SubmitAction(context.Background(), cmdDiscard{userID: "u1", tile: "m1", res: make(chan actionResult, 1)})
	require.ErrorIs(t, err, ErrRateLimited)
	require.Nil(t, notifs)
}

func TestDoGangClosesRoomAfterSettlement(t *testing.T) {
	t.Parallel()

	r := domainroom.NewRoom("r-gang-close")
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, ok := r.JoinAutoSeat(uid)
		require.True(t, ok)
	}
	for i := 0; i < 4; i++ {
		require.NoError(t, r.SetReady(domainroom.Seat(i), true))
	}
	require.NoError(t, r.StartPlaying())

	rs := eng.NewRoundStateFromConfig(eng.RoundStateConfig{
		RoomID:    "r-gang-close",
		RuleID:    "sichuan_xuezhandaodi_huansanzhang",
		Rule:      rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		PlayerIDs: [4]string{"u0", "u1", "u2", "u3"},
		Wall:      wall.NewFromOrderedTiles(nil),
		Hands: []*hand.Hand{
			hand.FromTiles([]tile.Tile{
				tile.Must(tile.SuitCharacters, 1),
				tile.Must(tile.SuitCharacters, 1),
				tile.Must(tile.SuitCharacters, 1),
				tile.Must(tile.SuitCharacters, 1),
			}),
			hand.New(), hand.New(), hand.New(),
		},
		RuleState:      testRuleState(make([]int32, 4)),
		WaitingDiscard: true,
		Turn:           0,
	})

	a := &Actor{
		Room:   r,
		engine: eng.NewEngine("sichuan_xuezhandaodi_huansanzhang"),
		round:  rs,
	}
	notifs, err := a.doGang("u0", "m1")
	require.NoError(t, err)
	// 暗杠展开为 4 条 + 结算 1 条 = 5 条（荒牌时无摸牌通知）
	require.Len(t, notifs, 5)
	require.Nil(t, a.round)
	require.Equal(t, domainroom.StateClosed, r.FSM.State())
}
