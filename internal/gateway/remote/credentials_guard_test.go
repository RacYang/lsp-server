package remote

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTransportCredentialDecisionNotInlined 断言凭据归属不变量：集群出站连接的
// 传输凭据由 config 持值、internal/cluster 构造、New 装配注入；本包任何生产源码
// 不得直接引用 insecure.NewCredentials 在调用点自行决定凭据形态。
func TestTransportCredentialDecisionNotInlined(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		require.NotContains(t, string(src), "insecure.NewCredentials",
			"%s 不得在调用点内联凭据决策，凭据须经 internal/cluster 构造注入", name)
	}
}
