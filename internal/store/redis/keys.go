package redis

import "fmt"

// SessionKey 返回 lsp:session:{userID} 形式的键名，用于记录用户与 gate 节点绑定关系。
func SessionKey(userID string) string {
	return fmt.Sprintf("lsp:session:%s", userID)
}

// SessionLookupKey 通过令牌摘要反查 user_id，键值仅存 user_id 字符串。
func SessionLookupKey(tokenHash string) string {
	return fmt.Sprintf("lsp:session:lookup:%s", tokenHash)
}

// IdempotencyKey 返回 lsp:idem:{scope}:{key} 形式的键名，用于缓存幂等响应与去重结果。
func IdempotencyKey(scope, idempotencyKey string) string {
	return fmt.Sprintf("lsp:idem:%s:%s", scope, idempotencyKey)
}

// RoomRouteCacheKey 返回 lsp:route:room:{roomID} 形式的缓存键；仅作 etcd 回源后的只读缓存。
func RoomRouteCacheKey(roomID string) string {
	return fmt.Sprintf("lsp:route:room:%s", roomID)
}

// RoomSnapshotMetaKey 返回 lsp:room:snapmeta:{roomID} 形式的房间快照摘要键。
func RoomSnapshotMetaKey(roomID string) string {
	return fmt.Sprintf("lsp:room:snapmeta:%s", roomID)
}

// UserProfileKey 返回 lsp:user:profile:{userID} 形式的用户资料键。
func UserProfileKey(userID string) string {
	return fmt.Sprintf("lsp:user:profile:%s", userID)
}

// RoomEventQueueKey 返回 lsp:room:{roomID}:events 形式的房间实时事件队列键。
// 由 room 节点 RPUSH，gate 节点通过 BLPOP 消费；TTL 5 分钟防止死房间积压。
func RoomEventQueueKey(roomID string) string {
	return fmt.Sprintf("lsp:room:%s:events", roomID)
}

// LobbyRoomsKey 返回 lsp:lobby:rooms Hash 键，field=roomID，value=JSON 房间快照。
// 由 lobby 节点在房间创建/加入/离开时写入，启动时从此处恢复内存状态。
func LobbyRoomsKey() string {
	return "lsp:lobby:rooms"
}
