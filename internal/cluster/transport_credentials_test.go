package cluster

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// TestTransportCredentialsSemantics 断言凭据构造点的三态语义：
// 全空→显式明文、半配置→拒绝、文件无效→拒绝。
func TestTransportCredentialsSemantics(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.pem")
	tests := []struct {
		name                    string
		cert, key, ca           string
		wantErr                 bool
		wantInsecureWhenNoError bool
	}{
		{name: "全空时显式明文", wantInsecureWhenNoError: true},
		{name: "仅有证书属半配置", cert: missing, wantErr: true},
		{name: "缺少CA属半配置", cert: missing, key: missing, wantErr: true},
		{name: "文件不存在时拒绝", cert: missing, key: missing, ca: missing, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			serverCreds, serverErr := NewServerTransportCredentials(tc.cert, tc.key, tc.ca)
			clientCreds, clientErr := NewClientTransportCredentials(tc.cert, tc.key, tc.ca, "")
			if tc.wantErr {
				require.Error(t, serverErr)
				require.Error(t, clientErr)
				return
			}
			require.NoError(t, serverErr)
			require.NoError(t, clientErr)
			require.Equal(t, tc.wantInsecureWhenNoError, serverCreds.Info().SecurityProtocol == "insecure")
			require.Equal(t, tc.wantInsecureWhenNoError, clientCreds.Info().SecurityProtocol == "insecure")
		})
	}
}

// TestClusterMTLSHandshake 断言 mTLS 边界：持合法客户端证书的调用方握手成功，
// 不出示客户端证书的调用方被服务端拒绝——这是集群内认证边界的回归锚点。
func TestClusterMTLSHandshake(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ca, caFile := writeTestCA(t, dir)
	serverCertFile, serverKeyFile := writeLeafCert(t, dir, ca, "server", x509.ExtKeyUsageServerAuth)
	clientCertFile, clientKeyFile := writeLeafCert(t, dir, ca, "client", x509.ExtKeyUsageClientAuth)

	serverCreds, err := NewServerTransportCredentials(serverCertFile, serverKeyFile, caFile)
	require.NoError(t, err)
	srv := grpc.NewServer(grpc.Creds(serverCreds))
	healthpb.RegisterHealthServer(srv, health.NewServer())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientCreds, err := NewClientTransportCredentials(clientCertFile, clientKeyFile, caFile, "")
	require.NoError(t, err)
	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(clientCreds))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err, "持合法客户端证书的调用方应通过 mTLS 握手")

	// 仅验证服务端、不出示客户端证书的调用方必须被拒绝。
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(caPEMBytes(t, caFile)))
	noCert := credentials.NewTLS(&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
	conn2, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(noCert))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn2.Close() })
	shortCtx, cancelShort := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelShort()
	_, err = healthpb.NewHealthClient(conn2).Check(shortCtx, &healthpb.HealthCheckRequest{})
	require.Error(t, err, "无客户端证书的调用方必须被 mTLS 边界拒绝")
}

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

// writeTestCA 生成自签 CA 并写入目录，返回 CA 材料与证书文件路径。
func writeTestCA(t *testing.T, dir string) (testCA, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "lsp-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	caFile := filepath.Join(dir, "ca.pem")
	writePEM(t, caFile, "CERTIFICATE", der)
	return testCA{cert: cert, key: key}, caFile
}

// writeLeafCert 由测试 CA 签发叶子证书（含 127.0.0.1 SAN）并写入目录。
func writeLeafCert(t *testing.T, dir string, ca testCA, name string, usage x509.ExtKeyUsage) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "lsp-test-" + name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)
	certFile = filepath.Join(dir, name+".pem")
	keyFile = filepath.Join(dir, name+".key")
	writePEM(t, certFile, "CERTIFICATE", der)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	writePEM(t, keyFile, "EC PRIVATE KEY", keyDER)
	return certFile, keyFile
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}))
	require.NoError(t, f.Close())
}

func caPEMBytes(t *testing.T, caFile string) []byte {
	t.Helper()
	b, err := os.ReadFile(caFile)
	require.NoError(t, err)
	return b
}
