package cluster

import (
	"context"
	"fmt"
)

// RoomNodeSelector 为新建房间选择 room 节点。
type RoomNodeSelector interface {
	Select(ctx context.Context) (nodeID string, err error)
}

// LeastRoomsSelector 从 etcd 读取各 room 节点的活跃房间数，选择负载最少的节点。
// 多个节点负载相同时选 etcd 返回顺序中的第一个（字典序最小键）。
type LeastRoomsSelector struct {
	disco *EtcdDiscovery
}

// NewLeastRoomsSelector 创建基于 etcd 负载数据的节点选择器。
func NewLeastRoomsSelector(disco *EtcdDiscovery) *LeastRoomsSelector {
	return &LeastRoomsSelector{disco: disco}
}

// Select 返回当前活跃房间数最少的 room 节点 ID。
// 若无可用节点则返回 error；etcd 查询失败同样返回 error。
func (s *LeastRoomsSelector) Select(ctx context.Context) (string, error) {
	if s == nil || s.disco == nil {
		return "", fmt.Errorf("nil selector")
	}
	nodes, err := s.disco.ListNodes(ctx, KindRoom)
	if err != nil {
		return "", fmt.Errorf("查询 room 节点列表失败: %w", err)
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("无可用 room 节点")
	}
	best := nodes[0]
	for _, n := range nodes[1:] {
		if n.Meta.ActiveRooms < best.Meta.ActiveRooms {
			best = n
		}
	}
	return best.NodeID, nil
}
