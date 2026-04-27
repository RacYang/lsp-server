//go:build integration

package app_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/testcontainers/testcontainers-go"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/net/msgid"
)

func TestRoomProcessRestartReplay(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("docker unavailable")
	}
	ensureDockerImage(t, "postgres:16-alpine")

	repoRoot := mustRepoRootIntegration(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)

	etcdURL := startEmbeddedEtcdForIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	pgC, err := pgmodule.Run(ctx, "postgres:16-alpine",
		pgmodule.WithDatabase("lsp"),
		pgmodule.WithUsername("lsp"),
		pgmodule.WithPassword("lsp"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pgC.Terminate(context.Background())
	})
	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	gateAddr := reserveTCPAddr(t)
	lobbyAddr := reserveTCPAddr(t)
	roomAddr := reserveTCPAddr(t)
	tempDir := t.TempDir()

	lobbyCfg := writeConfig(t, tempDir, "lobby.yaml", fmt.Sprintf(
		"server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\nobs:\n  addr: \"\"\netcd:\n  endpoints: %q\n",
		lobbyAddr, "sichuan_xzdd", etcdURL,
	))
	roomCfg := writeConfig(t, tempDir, "room.yaml", fmt.Sprintf(
		"server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\nredis:\n  addr: %q\npostgres:\n  dsn: %q\nobs:\n  addr: \"\"\netcd:\n  endpoints: %q\n",
		roomAddr, "sichuan_xzdd", mr.Addr(), dsn, etcdURL,
	))
	gateCfg := writeConfig(t, tempDir, "gate.yaml", fmt.Sprintf(
		"server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: %q\n  room_addr: %q\nredis:\n  addr: %q\npostgres:\n  dsn: \"\"\nobs:\n  addr: \"\"\netcd:\n  endpoints: %q\n",
		gateAddr, "sichuan_xzdd", lobbyAddr, roomAddr, mr.Addr(), etcdURL,
	))

	lobbyProc := startManagedProc(t, ctx, repoRoot, "./cmd/lobby", lobbyCfg)
	defer lobbyProc.Stop(t)
	roomProc := startManagedProc(t, ctx, repoRoot, "./cmd/room", roomCfg)
	defer roomProc.Stop(t)
	gateProc := startManagedProc(t, ctx, repoRoot, "./cmd/gate", gateCfg)
	defer gateProc.Stop(t)

	waitForTCP(t, lobbyAddr, 20*time.Second)
	waitForTCP(t, roomAddr, 20*time.Second)
	waitForTCP(t, gateAddr, 20*time.Second)

	roomID := "cluster-room-restart-1"
	conns := make([]*websocket.Conn, 4)
	tokens := make([]string, 4)
	for i := range conns {
		conns[i] = dialWS(t, gateAddr)
		t.Cleanup(func() { _ = conns[i].Close() })
	}

	for i := range conns {
		tokens[i] = loginJoinReturnSessionToken(t, conns[i], roomID)
	}
	sendReadyAndReadResp(t, conns[0])

	roomProc.Stop(t)
	roomProc = startManagedProc(t, ctx, repoRoot, "./cmd/room", roomCfg)
	defer roomProc.Stop(t)
	waitForTCP(t, roomAddr, 20*time.Second)

	for i := range conns {
		requireReconnectSnapshot(t, gateAddr, tokens[i], roomID, &conns[i])
	}

	for i := range conns {
		sendReadyAndReadResp(t, conns[i])
	}
	sn := drivePlayersUntilSettlement(t, conns)
	if sn == nil || sn.GetRoomId() != roomID {
		t.Fatalf("重启后结算房间号不一致: %+v", sn)
	}
}

func TestRoomProcessRestartReconnectNoDocker(t *testing.T) {
	repoRoot := mustRepoRootIntegration(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)

	etcdURL := startEmbeddedEtcdForIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	gateAddr := reserveTCPAddr(t)
	lobbyAddr := reserveTCPAddr(t)
	roomAddr := reserveTCPAddr(t)
	tempDir := t.TempDir()

	lobbyCfg := writeConfig(t, tempDir, "lobby.yaml", fmt.Sprintf(
		"server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\nobs:\n  addr: \"\"\netcd:\n  endpoints: %q\n",
		lobbyAddr, "sichuan_xzdd", etcdURL,
	))
	roomCfg := writeConfig(t, tempDir, "room.yaml", fmt.Sprintf(
		"server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\nredis:\n  addr: %q\npostgres:\n  dsn: \"\"\nobs:\n  addr: \"\"\netcd:\n  endpoints: %q\n",
		roomAddr, "sichuan_xzdd", mr.Addr(), etcdURL,
	))
	gateCfg := writeConfig(t, tempDir, "gate.yaml", fmt.Sprintf(
		"server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: %q\n  room_addr: %q\nredis:\n  addr: %q\npostgres:\n  dsn: \"\"\nobs:\n  addr: \"\"\netcd:\n  endpoints: %q\n",
		gateAddr, "sichuan_xzdd", lobbyAddr, roomAddr, mr.Addr(), etcdURL,
	))

	lobbyProc := startManagedProc(t, ctx, repoRoot, "./cmd/lobby", lobbyCfg)
	defer lobbyProc.Stop(t)
	roomProc := startManagedProc(t, ctx, repoRoot, "./cmd/room", roomCfg)
	defer roomProc.Stop(t)
	gateProc := startManagedProc(t, ctx, repoRoot, "./cmd/gate", gateCfg)
	defer gateProc.Stop(t)

	waitForTCP(t, lobbyAddr, 20*time.Second)
	waitForTCP(t, roomAddr, 20*time.Second)
	waitForTCP(t, gateAddr, 20*time.Second)

	roomID := "cluster-room-restart-nodocker"
	conns := make([]*websocket.Conn, 4)
	tokens := make([]string, 4)
	for i := range conns {
		conns[i] = dialWS(t, gateAddr)
		tokens[i] = loginJoinReturnSessionToken(t, conns[i], roomID)
	}
	sendReadyAndReadResp(t, conns[0])
	for _, conn := range conns {
		_ = conn.Close()
	}

	roomProc.Stop(t)
	roomProc = startManagedProc(t, ctx, repoRoot, "./cmd/room", roomCfg)
	defer roomProc.Stop(t)
	waitForTCP(t, roomAddr, 20*time.Second)

	for i := range conns {
		requireReconnectSnapshot(t, gateAddr, tokens[i], roomID, &conns[i])
		_ = conns[i].Close()
	}
}

type managedProc struct {
	target string
	cmd    *exec.Cmd
	out    *bytes.Buffer
}

func startManagedProc(t *testing.T, ctx context.Context, repoRoot, target, cfgPath string) *managedProc {
	t.Helper()
	cmd := exec.CommandContext(ctx, "go", "run", target)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "LSP_CONFIG="+cfgPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动进程 %s 失败: %v", target, err)
	}
	return &managedProc{target: target, cmd: cmd, out: &out}
}

func (p *managedProc) Stop(t *testing.T) {
	t.Helper()
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil && !isExited(err) {
			t.Logf("%s 退出日志:\n%s", p.target, p.out.String())
		}
	case <-time.After(10 * time.Second):
		_ = p.cmd.Process.Kill()
		t.Logf("%s 超时被强制终止，日志:\n%s", p.target, p.out.String())
	}
	p.cmd = nil
}

func requireReconnectSnapshot(t *testing.T, gateAddr, token, roomID string, connRef **websocket.Conn) {
	t.Helper()
	if connRef != nil && *connRef != nil {
		_ = (*connRef).Close()
	}
	reconn := dialWS(t, gateAddr)
	if connRef != nil {
		*connRef = reconn
	}
	req := &clientv1.Envelope{ReqId: "re", Body: &clientv1.Envelope_LoginReq{
		LoginReq: &clientv1.LoginRequest{Nickname: "重连", SessionToken: token},
	}}
	pb, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := reconn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.LoginReq, pb)); err != nil {
		t.Fatal(err)
	}
	env := readWSMessage(t, reconn, msgid.LoginResp)
	if !env.GetLoginResp().GetResumed() {
		t.Fatalf("期望 resumed=true，实际 %+v", env.GetLoginResp())
	}
	snap := readWSMessage(t, reconn, msgid.SnapshotNotify)
	if snap.GetSnapshot().GetRoomId() != roomID {
		t.Fatalf("重连快照房间号不一致: %+v", snap.GetSnapshot())
	}
}

func readWSMessage(t *testing.T, conn *websocket.Conn, wantMsg uint16) *clientv1.Envelope {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(8 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	h, err := frame.ReadFrame(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if h.MsgID != wantMsg {
		t.Fatalf("msg_id want %d got %d", wantMsg, h.MsgID)
	}
	var env clientv1.Envelope
	if err := proto.Unmarshal(h.Payload, &env); err != nil {
		t.Fatal(err)
	}
	return &env
}

func sendReadyOnly(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	rd := &clientv1.Envelope{ReqId: "r", Body: &clientv1.Envelope_ReadyReq{ReadyReq: &clientv1.ReadyRequest{}}}
	pb, err := proto.Marshal(rd)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame.Encode(msgid.ReadyReq, pb)); err != nil {
		t.Fatal(err)
	}
}

func dockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

func ensureDockerImage(t *testing.T, image string) {
	t.Helper()
	inspect := exec.Command("docker", "image", "inspect", image)
	if inspect.Run() == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "pull", image)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("预拉取镜像 %s 失败: %v\n%s", image, err, out.String())
	}
}

func startEmbeddedEtcdForIntegration(t *testing.T) string {
	t.Helper()
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.Logger = "zap"
	cfg.LogLevel = "error"

	clientURL := mustEtcdURL(t)
	peerURL := mustEtcdURL(t)
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.InitialCluster = cfg.InitialClusterFromName(cfg.Name)

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		t.Fatal("etcd not ready")
	}

	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{clientURL.String()}, DialTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return clientURL.String()
}

func mustEtcdURL(t *testing.T) url.URL {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	u, err := url.Parse("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	return *u
}

func mustRepoRootIntegration(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位当前测试文件")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func isExited(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}
