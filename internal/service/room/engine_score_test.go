package room

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/sichuanxzdd"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

func TestAppendHuEntriesTsumoAndDiscard(t *testing.T) {
	rs := scoreRoundState()
	breakdown := fan.Breakdown{}
	breakdown.Add(fan.KindPingHu, 1, "平胡")

	appendHuEntries(rs, 0, 2, rules.HuSourceTsumo, -1, breakdown)
	require.Len(t, rs.ledger, 3)
	require.EqualValues(t, 6, seatBalancesFromLedger(rs.ledger)[0])

	rs.ledger = nil
	appendHuEntries(rs, 2, 3, rules.HuSourceDiscard, 1, breakdown)
	require.Equal(t, []int32{0, -3, 3, 0}, seatBalancesFromLedger(rs.ledger))
}

func TestAppendHuEntriesQiangGangAddsBaoPaiName(t *testing.T) {
	rs := scoreRoundState()
	breakdown := fan.Breakdown{}
	breakdown.Add(fan.KindQiangGangHu, 1, "抢杠胡")

	appendHuEntries(rs, 3, 4, rules.HuSourceQiangGang, 0, breakdown)
	require.Len(t, rs.ledger, 1)
	require.Contains(t, rs.ledger[0].FanNames, sichuanxzdd.ReasonBaoPai)
}

func TestAppendGangEntriesRecordsLedgerAndHistory(t *testing.T) {
	rs := scoreRoundState()
	appendGangEntries(rs, 1, tile.Must(tile.SuitCharacters, 5), rules.GangKindAn, -1)

	require.Len(t, rs.ledger, 3)
	require.Len(t, rs.gangRecords, 1)
	require.True(t, rs.lastGangFollowUp)
	require.Equal(t, []int32{-2, 6, -2, -2}, seatBalancesFromLedger(rs.ledger))
}

func TestLegacyLedgerFromTotals(t *testing.T) {
	ledger := legacyLedgerFromTotals([]int32{3, -2, 0, 1})
	require.Len(t, ledger, 3)
	require.Equal(t, []int32{3, -2, 0, 1}, seatBalancesFromLedger(ledger))
}

func TestPlayAutoRoundEmitsStructuredSettlement(t *testing.T) {
	notifications, err := NewEngine("sichuan_xzdd").PlayAutoRound(context.Background(), "auto-score-room", [4]string{"u0", "u1", "u2", "u3"})
	require.NoError(t, err)
	require.NotEmpty(t, notifications)

	last := notifications[len(notifications)-1]
	require.Equal(t, KindSettlement, last.Kind)
	var env clientv1.Envelope
	require.NoError(t, proto.Unmarshal(last.Payload, &env))
	require.NotNil(t, env.GetSettlement())
	require.Len(t, env.GetSettlement().GetSeatScores(), 4)
}

func scoreRoundState() *RoundState {
	return &RoundState{
		roomID:          "score-room",
		ruleID:          "sichuan_xzdd",
		rule:            rules.MustGet("sichuan_xzdd"),
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		wall:            wall.NewFromOrderedTiles(nil),
		hands:           []*hand.Hand{hand.New(), hand.New(), hand.New(), hand.New()},
		queBySeat:       make([]int32, 4),
		lastDiscardSeat: -1,
		huedSeats:       make([]bool, 4),
		winnerSeats:     make([]int, 0, 3),
		ledger:          make([]sichuanxzdd.ScoreEntry, 0, 4),
	}
}
