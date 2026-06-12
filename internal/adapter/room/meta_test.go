package roomadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/redis"
)

// fakeMetaRooms 只覆盖 persistRoomMeta 依赖的两个查询方法，其余方法走零值接口（不会被调用）。
type fakeMetaRooms struct {
	roomsvc.RoomService
	players    []string
	state      string
	snapshotOK bool
	roundJSON  []byte
	roundErr   error
}

func (f *fakeMetaRooms) RoomSnapshot(string) ([]string, string, [4]bool, bool) {
	return f.players, f.state, [4]bool{}, f.snapshotOK
}

func (f *fakeMetaRooms) RoundPersistSnapshot(context.Context, string) ([]byte, error) {
	return f.roundJSON, f.roundErr
}

// TestPersistRoomMetaConsistencyInvariant 断言快照一致性不变量：snapmeta 只能整体派生自
// 一次成功读取的内存切片——任何来源读取失败必须保留上一份一致快照，绝不写入部分有效数据；
// 只有"局已结束"才允许清空 RoundJSON；Seq 单调不减。
func TestPersistRoomMetaConsistencyInvariant(t *testing.T) {
	t.Parallel()

	const roomID = "r-meta-invariant"
	players := []string{"u1", "u2", "u3", "u4"}
	prev := redis.RoomSnapMeta{
		Seq:       9,
		PlayerIDs: players,
		QueSuits:  []int32{0, 1, 2, 0},
		State:     "playing",
		RoundJSON: `{"schema_version":7,"rule_id":"sichuan_xuezhandaodi_huansanzhang"}`,
	}

	tests := []struct {
		name  string
		rooms *fakeMetaRooms
		seq   int64
		want  redis.RoomSnapMeta
	}{
		{
			name:  "局内快照读取失败时保留上一份一致快照",
			rooms: &fakeMetaRooms{players: players, state: "playing", snapshotOK: true, roundErr: errors.New("actor 提交超时")},
			seq:   12,
			want:  prev,
		},
		{
			name:  "房间内存快照读取失败时保留上一份一致快照",
			rooms: &fakeMetaRooms{snapshotOK: false},
			seq:   12,
			want:  prev,
		},
		{
			name:  "局已结束时合法清空 RoundJSON 并推进序号",
			rooms: &fakeMetaRooms{players: players, state: "closed", snapshotOK: true},
			seq:   12,
			want: redis.RoomSnapMeta{
				Seq:       12,
				PlayerIDs: players,
				QueSuits:  []int32{0, 1, 2, 0},
				State:     "closed",
			},
		},
		{
			name:  "传入序号低于上一份快照时不回退",
			rooms: &fakeMetaRooms{players: players, state: "playing", snapshotOK: true, roundJSON: []byte(`{"schema_version":7,"step":5}`)},
			seq:   3,
			want: redis.RoomSnapMeta{
				Seq:       9,
				PlayerIDs: players,
				QueSuits:  []int32{0, 1, 2, 0},
				State:     "playing",
				RoundJSON: `{"schema_version":7,"step":5}`,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mr, err := miniredis.Run()
			require.NoError(t, err)
			t.Cleanup(mr.Close)
			rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = rcli.Close() })
			rdb := redis.NewClientFromUniversal(rcli)

			ctx := context.Background()
			require.NoError(t, rdb.PutRoomSnapMeta(ctx, roomID, prev, 0))

			srv := &GRPCServer{rooms: tc.rooms, rdb: rdb}
			srv.persistRoomMeta(ctx, roomID, tc.seq, nil)

			got, ok, err := rdb.GetRoomSnapMeta(ctx, roomID)
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, tc.want, got)
		})
	}
}
