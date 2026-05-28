package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// hashSessionToken 对明文会话令牌做 SHA-256 十六进制摘要，仅用于存证与反查。
func hashSessionToken(plain string) string {
	if plain == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// formatSessionToken 将版本与随机载荷编码为不透明令牌。
func formatSessionToken(version int64, entropy string) string {
	return fmt.Sprintf("v%d.%s", version, strings.TrimSpace(entropy))
}

// parseSessionTokenVersion 提取会话令牌版本；失败返回 ok=false。
func parseSessionTokenVersion(plain string) (version int64, ok bool) {
	if !strings.HasPrefix(plain, "v") {
		return 0, false
	}
	head, _, found := strings.Cut(strings.TrimSpace(plain), ".")
	if !found || len(head) <= 1 {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.TrimPrefix(head, "v"), 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}
