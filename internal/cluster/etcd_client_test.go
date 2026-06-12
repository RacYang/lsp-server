package cluster

import (
	"crypto/x509"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewEtcdClientCredentialSemantics 断言 etcd 客户端构造点继承凭据三态语义：
// 全空明文可构造、半配置拒绝、TLS 材料齐备可构造。
func TestNewEtcdClientCredentialSemantics(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ca, caFile := writeTestCA(t, dir)
	certFile, keyFile := writeLeafCert(t, dir, ca, "etcd-client", x509.ExtKeyUsageClientAuth)
	missing := filepath.Join(dir, "missing.pem")

	tests := []struct {
		name          string
		cert, key, ca string
		wantErr       bool
	}{
		{name: "全空时以明文构造"},
		{name: "半配置时拒绝构造", cert: missing, wantErr: true},
		{name: "材料齐备时以 TLS 构造", cert: certFile, key: keyFile, ca: caFile},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli, err := NewEtcdClient("127.0.0.1:2379", tc.cert, tc.key, tc.ca, "")
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cli)
			require.NoError(t, cli.Close())
		})
	}
}
