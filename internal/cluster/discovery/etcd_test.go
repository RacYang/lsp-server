package discovery

import (
	"context"
	"net"
	"net/url"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/cluster/nodeid"
)

func startEmbeddedEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.Logger = "zap"
	cfg.LogLevel = "error"

	clientURL := mustURL(t)
	peerURL := mustURL(t)
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.InitialCluster = cfg.InitialClusterFromName(cfg.Name)

	e, err := embed.StartEtcd(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { e.Close() })

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		t.Fatal("etcd not ready")
	}

	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{clientURL.String()}, DialTimeout: 5 * time.Second})
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	return e, cli
}

func mustURL(t *testing.T) url.URL {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	u, err := url.Parse("http://" + addr)
	require.NoError(t, err)
	return *u
}

func TestEtcdRegisterWatchAndRevoke(t *testing.T) {
	t.Parallel()
	_, cli := startEmbeddedEtcd(t)
	d := NewEtcd(cli, "/lsp-test", 5)

	watchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := d.WatchNodes(watchCtx, nodeid.KindGate)
	require.NoError(t, err)

	initial := <-ch
	require.Empty(t, initial)

	leaseID, err := d.Register(context.Background(), nodeid.KindGate, "gate-a", NodeMeta{AdvertiseAddr: "127.0.0.1:18080", Version: "v1"})
	require.NoError(t, err)
	require.Positive(t, leaseID)

	require.Eventually(t, func() bool {
		select {
		case nodes := <-ch:
			return len(nodes) == 1 && nodes[0].NodeID == "gate-a"
		default:
			return false
		}
	}, 5*time.Second, 20*time.Millisecond)

	require.NoError(t, d.KeepAlive(context.Background(), leaseID))
	require.NoError(t, d.Revoke(context.Background(), leaseID))

	require.Eventually(t, func() bool {
		select {
		case nodes := <-ch:
			return len(nodes) == 0
		default:
			return false
		}
	}, 5*time.Second, 20*time.Millisecond)
}

func TestEtcdResolveNode(t *testing.T) {
	t.Parallel()
	_, cli := startEmbeddedEtcd(t)
	d := NewEtcd(cli, "/lsp-test", 5)

	_, err := d.Register(context.Background(), nodeid.KindRoom, "room-local", NodeMeta{AdvertiseAddr: "127.0.0.1:19090", Version: "v1"})
	require.NoError(t, err)

	node, ok, err := d.ResolveNode(context.Background(), nodeid.KindRoom, "room-local")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "127.0.0.1:19090", node.Meta.AdvertiseAddr)
}

func TestEtcdRegisterAndKeepAlive(t *testing.T) {
	t.Parallel()
	_, cli := startEmbeddedEtcd(t)
	d := NewEtcd(cli, "/lsp-test", 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg, err := d.RegisterAndKeepAlive(ctx, nodeid.KindLobby, "lobby-a", NodeMeta{AdvertiseAddr: "127.0.0.1:19091"}, 10*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, "lobby-a", reg.NodeID)
	require.Positive(t, reg.LeaseID)

	node, ok, err := d.ResolveNode(context.Background(), nodeid.KindLobby, "lobby-a")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "127.0.0.1:19091", node.Meta.AdvertiseAddr)
	require.NoError(t, reg.Stop(context.Background()))
}
