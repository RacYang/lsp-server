package cluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLeastRoomsSelectorNilGuards 验证 nil 接收者与 nil disco 安全返回 error。
func TestLeastRoomsSelectorNilGuards(t *testing.T) {
	t.Parallel()
	var s *LeastRoomsSelector
	_, err := s.Select(context.Background())
	require.Error(t, err)

	s2 := &LeastRoomsSelector{disco: nil}
	_, err = s2.Select(context.Background())
	require.Error(t, err)
}

// TestLeastRoomsSelectorPicksLeast 使用嵌入式 etcd 验证选择器返回活跃房间数最少的节点。
func TestLeastRoomsSelectorPicksLeast(t *testing.T) {
	t.Parallel()
	_, cli := startEmbeddedEtcd(t)
	disco := NewEtcdDiscovery(cli, "/lsp-sel", 10)
	ctx := context.Background()

	// 注册两个 room 节点并上报不同的活跃房间数。
	reg1, err := disco.RegisterAndKeepAlive(ctx, KindRoom, "room-a", NodeMeta{AdvertiseAddr: "a:1", ActiveRooms: 5}, 5)
	require.NoError(t, err)
	defer func() { _ = reg1.Stop(context.Background()) }()

	reg2, err := disco.RegisterAndKeepAlive(ctx, KindRoom, "room-b", NodeMeta{AdvertiseAddr: "b:1", ActiveRooms: 2}, 5)
	require.NoError(t, err)
	defer func() { _ = reg2.Stop(context.Background()) }()

	sel := NewLeastRoomsSelector(disco)
	id, err := sel.Select(ctx)
	require.NoError(t, err)
	require.Equal(t, "room-b", id, "应选择活跃房间数更少的节点 room-b")
}

// TestLeastRoomsSelectorNoNodes 验证无节点时返回 error。
func TestLeastRoomsSelectorNoNodes(t *testing.T) {
	t.Parallel()
	_, cli := startEmbeddedEtcd(t)
	disco := NewEtcdDiscovery(cli, "/lsp-empty", 10)
	sel := NewLeastRoomsSelector(disco)
	_, err := sel.Select(context.Background())
	require.Error(t, err, "无节点时应返回 error")
}
