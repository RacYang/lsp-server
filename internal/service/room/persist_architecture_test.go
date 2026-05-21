package room

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/rules"
)

func TestMarshalRoundPersistJSONDoesNotWriteLegacyRuleFields(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		ruleID:      "sichuan_xuezhandaodi_huansanzhang",
		playerIDs:   [4]string{"p0", "p1", "p2", "p3"},
		winEvents:   []rules.WinEvent{{Seat: domainroom.SeatFromInt(0), Step: 1}},
		scoreEvents: []rules.ScoreEvent{{Reason: "test", ToSeat: domainroom.SeatFromInt(0), Amount: 1, Step: 1}},
	}
	data, err := rs.MarshalRoundPersistJSON()
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	for _, key := range []string{
		"que_by_seat",
		"hued_seats",
		"ledger",
		"winner_seat",
		"winner_seats",
		"total_fan_by_seat",
		"exchange_tiles",
		"exchange_done",
		"que_done",
		"exchange_dir",
		"waiting_exchange",
		"waiting_que_men",
	} {
		require.NotContains(t, raw, key, "new round snapshots must not write legacy rule field %q", key)
	}
	require.Contains(t, raw, "rule_state")
	require.Contains(t, raw, "win_events")
	require.Contains(t, raw, "score_events")
}
