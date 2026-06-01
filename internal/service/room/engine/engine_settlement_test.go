package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi"
)

func TestFinishRoundBuildsSettlementEnvelope(t *testing.T) {
	rs := scoreRoundState()
	rs.winEvents = []rules.WinEvent{{Seat: 1, Source: rules.HuSourceDiscard, FromSeat: 0, TotalFan: 2, FanNames: []string{"平胡"}}}
	rs.scoreEvents = []rules.ScoreEvent{{
		Reason:     xuezhandaodi.ReasonHuDiscard,
		FromSeat:   0,
		ToSeat:     1,
		Amount:     2,
		WinnerSeat: 1,
		WinnerFan:  2,
		FanNames:   []string{"平胡"},
	}}

	notification, err := rs.finishRound()
	require.NoError(t, err)
	require.Equal(t, KindSettlement, notification.Kind)
	require.True(t, rs.closed)
	require.False(t, rs.waitingDiscard)
	require.False(t, rs.waitingTsumo)

	var env clientv1.Envelope
	require.NoError(t, proto.Unmarshal(notification.Payload, &env))
	settlement := env.GetSettlement()
	require.NotNil(t, settlement)
	require.Equal(t, "score-room", settlement.GetRoomId())
	require.Equal(t, []string{"u1"}, settlement.GetWinnerUserIds())
	require.Len(t, settlement.GetSeatScores(), 4)
	require.Len(t, settlement.GetPerWinnerBreakdown(), 1)
	require.Contains(t, settlement.GetPerWinnerBreakdown()[0].GetFanNames(), "平胡")
}
