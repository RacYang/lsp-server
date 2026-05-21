package room

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/fan"
	_ "racoo.cn/lsp/internal/mahjong/guobiao/jingji"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

func TestAppendHuEntriesTsumoAndDiscard(t *testing.T) {
	rs := scoreRoundState()
	breakdown := fan.Breakdown{}
	breakdown.Add(fan.KindPingHu, 1, "平胡")

	appendHuEntries(rs, 0, 2, rules.HuSourceTsumo, -1, breakdown)
	require.Len(t, rs.scoreEvents, 3)
	require.EqualValues(t, 6, seatBalancesFromScoreEvents(rs.scoreEvents)[0])

	rs.scoreEvents = nil
	appendHuEntries(rs, 2, 3, rules.HuSourceDiscard, 1, breakdown)
	require.Equal(t, []int32{0, -3, 3, 0}, seatBalancesFromScoreEvents(rs.scoreEvents))
}

func TestAppendHuEntriesQiangGangAddsBaoPaiName(t *testing.T) {
	rs := scoreRoundState()
	breakdown := fan.Breakdown{}
	breakdown.Add(fan.KindQiangGangHu, 1, "抢杠胡")

	appendHuEntries(rs, 3, 4, rules.HuSourceQiangGang, 0, breakdown)
	require.Len(t, rs.scoreEvents, 1)
	require.Contains(t, rs.scoreEvents[0].FanNames, xuezhandaodi.ReasonBaoPai)
}

func TestAppendGangEntriesRecordsScoreEventsAndHistory(t *testing.T) {
	rs := scoreRoundState()
	appendGangEntries(rs, 1, tile.Must(tile.SuitCharacters, 5), rules.GangKindAn, -1)

	require.Len(t, rs.scoreEvents, 3)
	require.Len(t, rs.gangRecords, 1)
	require.True(t, rs.lastGangFollowUp)
	require.Equal(t, []int32{-2, 6, -2, -2}, seatBalancesFromScoreEvents(rs.scoreEvents))
}

func TestPlayAutoRoundEmitsStructuredSettlement(t *testing.T) {
	notifications, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").PlayAutoRound(context.Background(), "auto-score-room", [4]string{"u0", "u1", "u2", "u3"})
	require.NoError(t, err)
	require.NotEmpty(t, notifications)
	require.Contains(t, openingDoneKinds(notifications), "exchange_done")
	require.Contains(t, openingDoneKinds(notifications), "missing_suit_done")

	huActionsBeforeSettlement := 0
	previousWasHu := false
	for _, notification := range notifications {
		var env clientv1.Envelope
		require.NoError(t, proto.Unmarshal(notification.Payload, &env))
		if action := env.GetAction(); action != nil && action.GetAction() == "hu" {
			huActionsBeforeSettlement++
			previousWasHu = true
			continue
		}
		if notification.Kind == KindSettlement {
			require.False(t, previousWasHu && huActionsBeforeSettlement < 3, "自动回放不得在首家或第二家胡牌后立刻结算")
			break
		}
		previousWasHu = false
	}

	last := notifications[len(notifications)-1]
	require.Equal(t, KindSettlement, last.Kind)
	var env clientv1.Envelope
	require.NoError(t, proto.Unmarshal(last.Payload, &env))
	require.NotNil(t, env.GetSettlement())
	require.Len(t, env.GetSettlement().GetSeatScores(), 4)
}

func TestPlayAutoRoundGuobiaoSkipsSichuanOpeningNotifications(t *testing.T) {
	notifications, err := NewEngine("guobiao_jingji_biaozhun").PlayAutoRound(context.Background(), "auto-guobiao-room", [4]string{"u0", "u1", "u2", "u3"})
	require.NoError(t, err)
	require.NotEmpty(t, notifications)

	kinds := notificationKinds(notifications)
	require.NotContains(t, kinds, KindOpeningDone)
	require.Contains(t, kinds, KindStartGame)
	require.Equal(t, KindSettlement, notifications[len(notifications)-1].Kind)
}

func notificationKinds(notifications []Notification) map[Kind]struct{} {
	out := make(map[Kind]struct{}, len(notifications))
	for _, notification := range notifications {
		out[notification.Kind] = struct{}{}
	}
	return out
}

func openingDoneKinds(notifications []Notification) map[string]struct{} {
	out := map[string]struct{}{}
	for _, notification := range notifications {
		if notification.Kind != KindOpeningDone {
			continue
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(notification.Payload, &env); err != nil {
			continue
		}
		if done := env.GetOpeningDone(); done != nil {
			out[done.GetKind()] = struct{}{}
		}
	}
	return out
}

func scoreRoundState() *RoundState {
	return &RoundState{
		roomID:          "score-room",
		ruleID:          "sichuan_xuezhandaodi_huansanzhang",
		rule:            rules.MustGet("sichuan_xuezhandaodi_huansanzhang"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles(nil),
		hands:           []*hand.Hand{hand.New(), hand.New(), hand.New(), hand.New()},
		ruleState:       testRuleState(make([]int32, 4)),
		lastDiscardSeat: -1,
		scoreEvents:     make([]rules.ScoreEvent, 0, 4),
	}
}
