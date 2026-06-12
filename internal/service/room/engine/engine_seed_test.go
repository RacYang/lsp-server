package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStartRoundWallUnpredictableForSameRoomID 断言公平性不变量：真实对局的牌墙
// 不得由 roomID 推算。同一 roomID 开两局，种子与发牌后剩余牌序都必须不同
// （CSPRNG 下碰撞概率约 2^-63，可忽略）。该测试在种子取自 roomID 的旧实现下必然失败。
func TestStartRoundWallUnpredictableForSameRoomID(t *testing.T) {
	t.Parallel()

	const roomID = "room-seed-unpredictable"
	players := [4]string{"u0", "u1", "u2", "u3"}

	rs1, _, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").StartRound(context.Background(), roomID, players)
	require.NoError(t, err)
	rs2, _, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").StartRound(context.Background(), roomID, players)
	require.NoError(t, err)

	require.NotEqual(t, rs1.wallSeed, rs2.wallSeed, "同一 roomID 的两局不得复用种子")
	require.NotEqual(t, rs1.wall.Tiles(), rs2.wall.Tiles(), "同一 roomID 的两局牌墙剩余牌序不得一致")
}

// TestStartRoundInjectedSeedReproducesWall 断言测试缝可用：注入固定种子后，
// 同规则两局的牌墙与四家起手牌完全一致，且 RoundState 记录的审计种子等于注入值。
func TestStartRoundInjectedSeedReproducesWall(t *testing.T) {
	t.Parallel()

	const fixedSeed int64 = 7
	players := [4]string{"u0", "u1", "u2", "u3"}
	startWithSeed := func(roomID string) *RoundState {
		e := NewEngine("sichuan_xuezhandaodi_huansanzhang")
		e.SetWallSeedSource(func() int64 { return fixedSeed })
		rs, _, err := e.StartRound(context.Background(), roomID, players)
		require.NoError(t, err)
		return rs
	}

	rs1 := startWithSeed("room-seed-a")
	rs2 := startWithSeed("room-seed-b")

	require.Equal(t, fixedSeed, rs1.wallSeed)
	require.Equal(t, rs1.wall.Tiles(), rs2.wall.Tiles())
	for seat := 0; seat < 4; seat++ {
		require.Equal(t, rs1.hands[seat].Tiles(), rs2.hands[seat].Tiles(), "座位 %d 起手牌应一致", seat)
	}
}

// TestRoundPersistRoundTripKeepsWallSeed 断言审计种子随快照持久化并在恢复后保留。
func TestRoundPersistRoundTripKeepsWallSeed(t *testing.T) {
	t.Parallel()

	rs, _, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").StartRound(
		context.Background(), "room-seed-persist", [4]string{"u0", "u1", "u2", "u3"},
	)
	require.NoError(t, err)
	require.NotZero(t, rs.wallSeed)

	data, err := rs.MarshalRoundPersistJSON()
	require.NoError(t, err)
	restored, err := RestoreRoundFromPersistJSON("room-seed-persist", data)
	require.NoError(t, err)
	require.Equal(t, rs.wallSeed, restored.wallSeed)
}

// TestPlayAutoRoundReplayDeterministicForSameRoomID 守住回放路径的既有职责：
// 同一 roomID 的回放局必须逐字节复现，确保种子职责分离没有破坏可复现性。
func TestPlayAutoRoundReplayDeterministicForSameRoomID(t *testing.T) {
	t.Parallel()

	const roomID = "room-seed-replay"
	players := [4]string{"u0", "u1", "u2", "u3"}

	first, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").PlayAutoRound(context.Background(), roomID, players)
	require.NoError(t, err)
	second, err := NewEngine("sichuan_xuezhandaodi_huansanzhang").PlayAutoRound(context.Background(), roomID, players)
	require.NoError(t, err)

	require.Equal(t, len(first), len(second))
	for i := range first {
		require.Equal(t, first[i].Kind, second[i].Kind, "第 %d 条通知类型应一致", i)
		require.Equal(t, first[i].Payload, second[i].Payload, "第 %d 条通知载荷应逐字节一致", i)
	}
}
