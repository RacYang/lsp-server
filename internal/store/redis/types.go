package redis

// SessionRecord 为会话在线状态；Phase 3 起补充房间、游标与令牌摘要，供断线重连校验。
type SessionRecord struct {
	GateNodeID    string `json:"gate_node_id"`
	AdvertiseAddr string `json:"advertise_addr,omitempty"`
	RoomID        string `json:"room_id,omitempty"`
	LastCursor    string `json:"last_cursor,omitempty"`
	TokenHash     string `json:"token_hash,omitempty"`
	SessionVer    int64  `json:"session_ver,omitempty"`
}

// RoomSnapMeta 为房间快照元数据摘要，与 PG 事件 seq 对齐，供重连与节点恢复。
type RoomSnapMeta struct {
	Seq       int64    `json:"seq"`
	PlayerIDs []string `json:"player_ids,omitempty"`
	QueSuits  []int32  `json:"que_suits,omitempty"`
	State     string   `json:"state,omitempty"`
	RoundJSON string   `json:"round_json,omitempty"`
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
