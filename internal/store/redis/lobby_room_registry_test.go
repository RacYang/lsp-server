package redis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	lobbysvc "racoo.cn/lsp/internal/service/lobby"
)

func TestLobbyRoomRegistryUpsertAndListAll(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t)
	reg := NewLobbyRoomRegistry(c)
	ctx := context.Background()

	tests := []struct {
		name string
		rec  lobbysvc.RoomRecord
	}{
		{
			name: "无座位房间",
			rec: lobbysvc.RoomRecord{
				RoomID:      "r-a",
				NodeID:      "room-local",
				RuleID:      "sichuan_xuezhandaodi_huansanzhang",
				DisplayName: "测试房",
				Private:     false,
				CreatedAtMs: 1000,
				MaxSeats:    4,
				Seats:       map[string]int32{},
			},
		},
		{
			name: "含座位房间",
			rec: lobbysvc.RoomRecord{
				RoomID:      "r-b",
				NodeID:      "room-local",
				RuleID:      "sichuan_xuezhandaodi_huansanzhang",
				DisplayName: "有人房",
				Private:     true,
				CreatedAtMs: 2000,
				MaxSeats:    4,
				Seats:       map[string]int32{"u1": 0, "u2": 1},
			},
		},
	}
	for _, tc := range tests {
		require.NoError(t, reg.UpsertRoom(ctx, tc.rec))
	}

	all, err := reg.ListAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)

	byID := make(map[string]lobbysvc.RoomRecord, 2)
	for _, r := range all {
		byID[r.RoomID] = r
	}
	require.Equal(t, "测试房", byID["r-a"].DisplayName)
	require.Equal(t, int32(0), byID["r-b"].Seats["u1"])
	require.Equal(t, int32(1), byID["r-b"].Seats["u2"])
	require.True(t, byID["r-b"].Private)
}

func TestLobbyRoomRegistryDeleteRoom(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t)
	reg := NewLobbyRoomRegistry(c)
	ctx := context.Background()

	rec := lobbysvc.RoomRecord{RoomID: "r-del", NodeID: "room-local", MaxSeats: 4, Seats: map[string]int32{}}
	require.NoError(t, reg.UpsertRoom(ctx, rec))

	all, _ := reg.ListAll(ctx)
	require.Len(t, all, 1)

	require.NoError(t, reg.DeleteRoom(ctx, "r-del"))
	all, err := reg.ListAll(ctx)
	require.NoError(t, err)
	require.Empty(t, all)
}

func TestLobbyRoomRegistryUpsertOverwrites(t *testing.T) {
	t.Parallel()
	c, _ := newTestClient(t)
	reg := NewLobbyRoomRegistry(c)
	ctx := context.Background()

	rec := lobbysvc.RoomRecord{RoomID: "r-ow", NodeID: "room-local", MaxSeats: 4, Seats: map[string]int32{}}
	require.NoError(t, reg.UpsertRoom(ctx, rec))

	// 更新座位后覆盖写入
	rec.Seats = map[string]int32{"u1": 0}
	require.NoError(t, reg.UpsertRoom(ctx, rec))

	all, err := reg.ListAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Len(t, all[0].Seats, 1)
	require.Equal(t, int32(0), all[0].Seats["u1"])
}
