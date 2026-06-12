// 远程网关凭据装配测试：覆盖构造期凭据注入与 fail-closed 语义。
package remote

import (
	"testing"

	"github.com/stretchr/testify/require"

	svcv1 "racoo.cn/lsp/api/gen/go/v1"

	"racoo.cn/lsp/internal/config"
)

// TestNewRejectsPartialClusterTLS 断言凭据半配置在构造期 fail-fast，
// 不得静默降级为明文出站。
func TestNewRejectsPartialClusterTLS(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		ClusterLobbyAddr: "127.0.0.1:1",
		ClusterRoomAddr:  "127.0.0.1:2",
		ClusterTLS:       config.ClusterTLS{CertFile: "/nonexistent/node.pem"},
	}
	gw, cleanup, err := New(cfg, nil, nil, nil, nil)
	require.Error(t, err)
	require.Nil(t, gw)
	require.Nil(t, cleanup)
}

// TestNewPlaintextAssemblyAndCleanup 断言全空凭据时网关以显式明文装配成功
// （gRPC 拨号为惰性建立，不要求对端在线），cleanup 可安全执行。
func TestNewPlaintextAssemblyAndCleanup(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		ClusterLobbyAddr: "127.0.0.1:1",
		ClusterRoomAddr:  "127.0.0.1:2",
	}
	gw, cleanup, err := New(cfg, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, gw)
	require.NotNil(t, cleanup)
	cleanup()
}

// TestRoomClientForAddrFailClosedWithoutCreds 断言未经构造期装配的实例
// 禁止以明文新建出站连接（凭据决策归属 New，不在调用点兜底）。
func TestRoomClientForAddrFailClosedWithoutCreds(t *testing.T) {
	t.Parallel()

	g := &remoteRoomGateway{
		roomClients: map[string]svcv1.RoomServiceClient{},
	}
	client, err := g.roomClientForAddr("127.0.0.1:3")
	require.Error(t, err)
	require.Nil(t, client)
	require.ErrorContains(t, err, "凭据未注入")
}
