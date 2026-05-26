package lobby

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// memRegistry 是用于测试的内存 RoomRegistry 实现。
type memRegistry struct {
	mu      sync.Mutex
	records map[string]RoomRecord
}

func newMemRegistry() *memRegistry {
	return &memRegistry{records: make(map[string]RoomRecord)}
}

func (r *memRegistry) UpsertRoom(_ context.Context, rec RoomRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[rec.RoomID] = rec
	return nil
}

func (r *memRegistry) DeleteRoom(_ context.Context, roomID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, roomID)
	return nil
}

func (r *memRegistry) ListAll(_ context.Context) ([]RoomRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RoomRecord, 0, len(r.records))
	for _, rec := range r.records {
		out = append(out, rec)
	}
	return out, nil
}

func (r *memRegistry) get(roomID string) (RoomRecord, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[roomID]
	return rec, ok
}

func TestNew(t *testing.T) {
	t.Parallel()
	s := New()
	require.NotNil(t, s)
}

func TestCreateJoinGetRoom(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()

	created, err := s.CreateRoom(ctx, "r1")
	require.NoError(t, err)
	require.Equal(t, "room-local", created)

	joined, err := s.JoinRoom(ctx, "r1", "u1")
	require.NoError(t, err)
	require.EqualValues(t, 0, joined)

	got, err := s.GetRoom(ctx, "r1")
	require.NoError(t, err)
	require.Equal(t, "room-local", got)
}

func TestListRulesReturnsRegisteredRuleMeta(t *testing.T) {
	t.Parallel()
	s := New()
	rules, err := s.ListRules(context.Background())
	require.NoError(t, err)
	require.Len(t, rules, 3)
	require.Equal(t, "guobiao_jingji_biaozhun", rules[0].RuleID)
	require.Equal(t, "sichuan_xuezhandaodi_biaozhun", rules[1].RuleID)
	require.Equal(t, "sichuan_xuezhandaodi_huansanzhang", rules[2].RuleID)
	require.NotEmpty(t, rules[0].DisplayName)
	require.Contains(t, rules[0].EnabledFeatures, "mcr_81_fans")
	require.Contains(t, rules[0].EnabledFeatures, "full_tiles")
	require.NotContains(t, rules[1].EnabledFeatures, "exchange_three")
	require.Contains(t, rules[2].EnabledFeatures, "exchange_three")
}

func TestLeaveRoomFreesSeatForImmediateAutoMatch(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	roomID, _, err := s.CreateRoomWithMeta(ctx, "sichuan_xuezhandaodi_huansanzhang", "公开桌", false, "u1")
	require.NoError(t, err)
	for _, userID := range []string{"u2", "u3", "u4"} {
		_, err = s.JoinRoom(ctx, roomID, userID)
		require.NoError(t, err)
	}
	_, err = s.JoinRoom(ctx, roomID, "u5")
	require.ErrorIs(t, err, ErrRoomFull)

	require.NoError(t, s.LeaveRoom(ctx, roomID, "u2"))
	joined, seat, err := s.AutoMatch(ctx, "sichuan_xuezhandaodi_huansanzhang", "u5")
	require.NoError(t, err)
	require.Equal(t, roomID, joined)
	require.EqualValues(t, 1, seat)
}

func TestListRoomsFiltersPrivateAndFullRooms(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()

	publicID, _, err := s.CreateRoomWithMeta(ctx, "sichuan_xuezhandaodi_huansanzhang", "公开桌", false, "u1")
	require.NoError(t, err)
	privateID, _, err := s.CreateRoomWithMeta(ctx, "sichuan_xuezhandaodi_huansanzhang", "私密桌", true, "u2")
	require.NoError(t, err)
	for _, userID := range []string{"u3", "u4", "u5"} {
		_, err = s.JoinRoom(ctx, publicID, userID)
		require.NoError(t, err)
	}
	_, err = s.JoinRoom(ctx, publicID, "u6")
	require.ErrorIs(t, err, ErrRoomFull)

	rooms, next, err := s.ListRooms(ctx, 20, "")
	require.NoError(t, err)
	require.Empty(t, next)
	require.Empty(t, rooms)

	_, err = s.JoinRoom(ctx, privateID, "u7")
	require.NoError(t, err)
	rooms, _, err = s.ListRooms(ctx, 20, "")
	require.NoError(t, err)
	require.Empty(t, rooms)
}

func TestAutoMatchUsesOldestOpenRoomOrCreatesFallback(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	s.newRoomID = fixedRoomIDs("AAA111", "BBB222", "CCC333")

	first, _, err := s.CreateRoomWithMeta(ctx, "sichuan_xuezhandaodi_huansanzhang", "一号桌", false, "u1")
	require.NoError(t, err)
	second, _, err := s.CreateRoomWithMeta(ctx, "other", "其它规则桌", false, "u2")
	require.NoError(t, err)

	roomID, seat, err := s.AutoMatch(ctx, "sichuan_xuezhandaodi_huansanzhang", "u3")
	require.NoError(t, err)
	require.Equal(t, first, roomID)
	require.EqualValues(t, 1, seat)

	roomID, seat, err = s.AutoMatch(ctx, "other", "u4")
	require.NoError(t, err)
	require.Equal(t, second, roomID)
	require.EqualValues(t, 1, seat)

	roomID, seat, err = s.AutoMatch(ctx, "new-rule", "u5")
	require.NoError(t, err)
	require.NotEmpty(t, roomID)
	require.EqualValues(t, 0, seat)
	require.NotEqual(t, first, roomID)
	require.NotEqual(t, second, roomID)
}

func TestCreateRoomWithMetaRetriesRoomIDCollision(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	calls := 0
	s.newRoomID = func() (string, error) {
		calls++
		if calls <= 2 {
			return "ABC123", nil
		}
		return "DEF456", nil
	}

	first, _, err := s.CreateRoomWithMeta(ctx, "", "", false, "u1")
	require.NoError(t, err)
	require.Equal(t, "ABC123", first)
	second, _, err := s.CreateRoomWithMeta(ctx, "", "", false, "u2")
	require.NoError(t, err)
	require.Equal(t, "DEF456", second)
	require.Equal(t, 3, calls)
}

func TestListRoomsPagination(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	s.newRoomID = fixedRoomIDs("ROOM01", "ROOM02", "ROOM03")
	for _, userID := range []string{"u1", "u2", "u3"} {
		_, _, err := s.CreateRoomWithMeta(ctx, "", "", false, userID)
		require.NoError(t, err)
	}

	firstPage, next, err := s.ListRooms(ctx, 2, "")
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	require.NotEmpty(t, next)
	secondPage, next, err := s.ListRooms(ctx, 2, next)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	require.Empty(t, next)
}

func fixedRoomIDs(ids ...string) func() (string, error) {
	i := 0
	return func() (string, error) {
		id := ids[i]
		i++
		return id, nil
	}
}

func TestNewWithRegistryPersistsCreateRoom(t *testing.T) {
	t.Parallel()
	reg := newMemRegistry()
	svc := NewWithRegistry(reg)
	ctx := context.Background()

	_, err := svc.CreateRoom(ctx, "r-persist")
	require.NoError(t, err)

	rec, ok := reg.get("r-persist")
	require.True(t, ok, "CreateRoom 应持久化房间记录")
	require.Equal(t, "r-persist", rec.RoomID)
	require.Equal(t, defaultNodeID, rec.NodeID)
}

func TestNewWithRegistryPersistsJoinAndLeave(t *testing.T) {
	t.Parallel()
	reg := newMemRegistry()
	svc := NewWithRegistry(reg)
	ctx := context.Background()

	_, err := svc.CreateRoom(ctx, "r-join")
	require.NoError(t, err)
	seat, err := svc.JoinRoom(ctx, "r-join", "u1")
	require.NoError(t, err)
	require.EqualValues(t, 0, seat)

	rec, ok := reg.get("r-join")
	require.True(t, ok)
	_, hasSeat := rec.Seats["u1"]
	require.True(t, hasSeat, "JoinRoom 后注册表应含座位记录")

	require.NoError(t, svc.LeaveRoom(ctx, "r-join", "u1"))
	rec, _ = reg.get("r-join")
	_, hasSeat = rec.Seats["u1"]
	require.False(t, hasSeat, "LeaveRoom 后注册表应移除座位")
}

func TestRecoverFromRegistryRestoresState(t *testing.T) {
	t.Parallel()
	reg := newMemRegistry()

	// 第一个实例：创建并加入房间
	svc1 := NewWithRegistry(reg)
	ctx := context.Background()
	_, err := svc1.CreateRoom(ctx, "r-recover")
	require.NoError(t, err)
	_, err = svc1.JoinRoom(ctx, "r-recover", "u1")
	require.NoError(t, err)

	// 写操作同步完成，直接验证注册表状态
	rec, ok := reg.get("r-recover")
	require.True(t, ok && len(rec.Seats) == 1, "注册表应含 u1 座位记录")

	// 第二个实例：模拟重启后恢复
	svc2 := NewWithRegistry(reg)
	require.NoError(t, svc2.RecoverFromRegistry(ctx))

	// 房间应已恢复：GetRoom 正常返回
	nodeID, err := svc2.GetRoom(ctx, "r-recover")
	require.NoError(t, err)
	require.Equal(t, defaultNodeID, nodeID)

	// u1 再次加入应返回同一座位（不新分配）
	seat, err := svc2.JoinRoom(ctx, "r-recover", "u1")
	require.NoError(t, err)
	require.EqualValues(t, 0, seat)

	// 房间只有 1 个已知座位（u1），再加 2 人后满
	_, err = svc2.JoinRoom(ctx, "r-recover", "u2")
	require.NoError(t, err)
	_, err = svc2.JoinRoom(ctx, "r-recover", "u3")
	require.NoError(t, err)
	_, err = svc2.JoinRoom(ctx, "r-recover", "u4")
	require.NoError(t, err)
	_, err = svc2.JoinRoom(ctx, "r-recover", "u5")
	require.ErrorIs(t, err, ErrRoomFull)
}
