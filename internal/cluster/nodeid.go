package cluster

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Kind 表示集群中的角色类型（与运维侧配置一致）。
type Kind string

const (
	KindGate  Kind = "gate"
	KindLobby Kind = "lobby"
	KindRoom  Kind = "room"
)

// NewNodeID 生成唯一节点 ID（短 UUID，便于 etcd 键展示）。
func NewNodeID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

// FormatNodeID 将角色与节点 ID 组合为可读的注册键后缀（不含全局前缀）。
func FormatNodeID(k Kind, id string) string {
	return fmt.Sprintf("%s/%s", k, strings.TrimSpace(id))
}

// KindString 返回用于日志与指标的角色字符串（小写）。
func KindString(k Kind) string {
	return string(k)
}
