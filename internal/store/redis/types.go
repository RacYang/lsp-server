package redis

// SessionRecord 为会话在线状态；当前只保存 gate 节点与可选接入地址，后续可追加版本号。
type SessionRecord struct {
	GateNodeID    string `json:"gate_node_id"`
	AdvertiseAddr string `json:"advertise_addr,omitempty"`
}

// RouteRecord 为房间路由缓存值；权威归属仍在 etcd，仅缓存 room 节点与可选版本。
type RouteRecord struct {
	RoomNodeID string `json:"room_node_id"`
	Version    int64  `json:"version,omitempty"`
}

// IdempotencyRecord 为幂等键缓存内容；可保存业务响应或去重摘要。
type IdempotencyRecord struct {
	Result string `json:"result"`
}
