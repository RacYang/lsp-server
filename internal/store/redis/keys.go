package redis

import "fmt"

// SessionKey 返回 lsp:session:{userID} 形式的键名，用于记录用户与 gate 节点绑定关系。
func SessionKey(userID string) string {
	return fmt.Sprintf("lsp:session:%s", userID)
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
