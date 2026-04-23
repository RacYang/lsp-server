package router

import (
	"context"
	"net"
	"net/url"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/stretchr/testify/require"
)

func TestEtcdClaimAndResolveRoomOwner(t *testing.T) {
	t.Parallel()
	_, cli := startEmbeddedEtcd(t)
	r := NewEtcd(cli, "/lsp-test")

	err := r.ClaimRoom(context.Background(), " room-1 ", "room-node-a", 0)
	require.NoError(t, err)

	nodeID, ok, err := r.ResolveRoomOwner(context.Background(), "room-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "room-node-a", nodeID)

	err = r.ClaimRoom(context.Background(), "room-1", "room-node-b", 0)
	require.Error(t, err)
}

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
