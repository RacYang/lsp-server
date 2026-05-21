package room

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestoreRoundRejectsMissingAndOldSchemas(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{}`,
		`{"schema_version":1}`,
		`{"schema_version":4,"que_by_seat":[0,1,2,0],"ledger":[{"reason":"legacy"}]}`,
		`{"schema_version":5,"waiting_exchange":true,"waiting_que_men":false}`,
	} {
		_, err := RestoreRoundFromPersistJSON("room-old-schema", []byte(raw))
		require.ErrorIs(t, err, ErrRoundPersistUnsupportedSchema, raw)
	}
}

func TestRestoreRoundAcceptsCurrentSchemaOnly(t *testing.T) {
	t.Parallel()

	rs := &RoundState{
		roomID:    "room-current-schema",
		ruleID:    "sichuan_xuezhandaodi_huansanzhang",
		playerIDs: [4]string{"p0", "p1", "p2", "p3"},
	}
	data, err := rs.MarshalRoundPersistJSON()
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "schema_version")

	restored, err := RestoreRoundFromPersistJSON("room-current-schema", data)
	require.NoError(t, err)
	require.Equal(t, "sichuan_xuezhandaodi_huansanzhang", restored.ruleID)
	require.Equal(t, [4]string{"p0", "p1", "p2", "p3"}, restored.playerIDs)
}
