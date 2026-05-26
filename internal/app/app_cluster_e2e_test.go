package app_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/protocol"
)

func TestClusterProcessesFourPlayersReceiveSettlement(t *testing.T) {
	// 地基二：gate 通过 Redis BLPOP 订阅事件，集群测试须配置 miniredis。
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	redisAddr := mr.Addr()

	repoRoot := mustRepoRoot(t)
	gateAddr := reserveTCPAddr(t)
	lobbyAddr := reserveTCPAddr(t)
	roomAddr := reserveTCPAddr(t)

	tempDir := t.TempDir()
	lobbyCfg := writeConfig(t, tempDir, "lobby.yaml", fmt.Sprintf("server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\n", lobbyAddr, "sichuan_xuezhandaodi_huansanzhang"))
	roomCfg := writeConfig(t, tempDir, "room.yaml", fmt.Sprintf("server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\nredis:\n  addr: %q\n", roomAddr, "sichuan_xuezhandaodi_huansanzhang", redisAddr))
	gateCfg := writeConfig(t, tempDir, "gate.yaml", fmt.Sprintf("server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: %q\n  room_addr: %q\nredis:\n  addr: %q\n", gateAddr, "sichuan_xuezhandaodi_huansanzhang", lobbyAddr, roomAddr, redisAddr))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	startProc(t, ctx, repoRoot, "./cmd/lobby", lobbyCfg)
	startProc(t, ctx, repoRoot, "./cmd/room", roomCfg)
	startProc(t, ctx, repoRoot, "./cmd/gate", gateCfg)

	waitForTCP(t, lobbyAddr, 20*time.Second)
	waitForTCP(t, roomAddr, 20*time.Second)
	waitForTCP(t, gateAddr, 20*time.Second)

	roomID := "cluster-room-smoke-1"
	conns := make([]*websocket.Conn, 4)
	for i := range conns {
		conns[i] = dialWS(t, gateAddr)
		t.Cleanup(func() { _ = conns[i].Close() })
	}
	for _, c := range conns {
		loginJoin(t, c, roomID)
	}
	for i := range conns {
		sendReadyAndReadResp(t, conns[i])
	}

	lastSn := drivePlayersUntilSettlement(t, conns)
	if lastSn == nil || lastSn.GetRoomId() != roomID {
		t.Fatalf("跨进程结算房间号不一致: %+v", lastSn)
	}
}

func TestClusterReconnectLoginWithSessionToken(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	redisAddr := mr.Addr()

	repoRoot := mustRepoRoot(t)
	gateAddr := reserveTCPAddr(t)
	lobbyAddr := reserveTCPAddr(t)
	roomAddr := reserveTCPAddr(t)

	tempDir := t.TempDir()
	lobbyCfg := writeConfig(t, tempDir, "lobby.yaml", fmt.Sprintf("server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\n", lobbyAddr, "sichuan_xuezhandaodi_huansanzhang"))
	roomCfg := writeConfig(t, tempDir, "room.yaml", fmt.Sprintf("server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: \"\"\n  room_addr: \"\"\nredis:\n  addr: %q\n", roomAddr, "sichuan_xuezhandaodi_huansanzhang", redisAddr))
	gateCfg := writeConfig(t, tempDir, "gate.yaml", fmt.Sprintf(
		"server:\n  addr: %q\nrule:\n  default_id: %q\ncluster:\n  lobby_addr: %q\n  room_addr: %q\nredis:\n  addr: %q\npostgres:\n  dsn: \"\"\nobs:\n  addr: \"\"\netcd:\n  endpoints: \"\"\n",
		gateAddr, "sichuan_xuezhandaodi_huansanzhang", lobbyAddr, roomAddr, redisAddr))

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	startProc(t, ctx, repoRoot, "./cmd/lobby", lobbyCfg)
	startProc(t, ctx, repoRoot, "./cmd/room", roomCfg)
	startProc(t, ctx, repoRoot, "./cmd/gate", gateCfg)

	waitForTCP(t, lobbyAddr, 20*time.Second)
	waitForTCP(t, roomAddr, 20*time.Second)
	waitForTCP(t, gateAddr, 20*time.Second)

	roomID := "cluster-reconnect-room-1"
	conns := make([]*websocket.Conn, 4)
	for i := range conns {
		conns[i] = dialWS(t, gateAddr)
		t.Cleanup(func() { _ = conns[i].Close() })
	}

	tok0 := loginJoinReturnSessionToken(t, conns[0], roomID)
	for i := 1; i < 4; i++ {
		loginJoin(t, conns[i], roomID)
	}
	// 至少一次 Ready 才会在 room 进程内 materialize 房间，SnapshotRoom 才能命中。
	sendReadyAndReadResp(t, conns[0])

	if err := conns[0].Close(); err != nil {
		t.Fatalf("关闭首路连接失败: %v", err)
	}

	reconn := dialWS(t, gateAddr)
	t.Cleanup(func() { _ = reconn.Close() })

	resume := &clientv1.Envelope{ReqId: "re", Body: &clientv1.Envelope_LoginReq{
		LoginReq: &clientv1.LoginRequest{Nickname: "重连", SessionToken: tok0},
	}}
	pb, err := proto.Marshal(resume)
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := protocol.Encode(protocol.LoginReq, pb)
	if err := reconn.WriteMessage(websocket.BinaryMessage, enc); err != nil {
		t.Fatal(err)
	}
	_ = reconn.SetReadDeadline(time.Now().Add(8 * time.Second))
	_, data, err := reconn.ReadMessage()
	if err != nil {
		t.Fatalf("读登录响应失败: %v", err)
	}
	h, err := protocol.ReadFrame(bytes.NewReader(data))
	if err != nil || h.MsgID != protocol.LoginResp {
		t.Fatal("重连登录响应异常")
	}
	var envLogin clientv1.Envelope
	if err := proto.Unmarshal(h.Payload, &envLogin); err != nil {
		t.Fatal(err)
	}
	if !envLogin.GetLoginResp().GetResumed() {
		t.Fatalf("期望 resumed=true，实际 %+v", envLogin.GetLoginResp())
	}

	_, snapData, err := reconn.ReadMessage()
	if err != nil {
		t.Fatalf("读快照帧失败: %v", err)
	}
	hSnap, err := protocol.ReadFrame(bytes.NewReader(snapData))
	if err != nil {
		t.Fatal(err)
	}
	if hSnap.MsgID != protocol.SnapshotNotify {
		t.Fatalf("期望 SnapshotNotify，实际 msg_id=%d", hSnap.MsgID)
	}
	var snapEnv clientv1.Envelope
	if err := proto.Unmarshal(hSnap.Payload, &snapEnv); err != nil {
		t.Fatal(err)
	}
	if snapEnv.GetSnapshot().GetRoomId() != roomID {
		t.Fatalf("快照房间号不一致: %+v", snapEnv.GetSnapshot())
	}

	conns[0] = reconn
	for i := range conns {
		sendReadyAndReadResp(t, conns[i])
	}
	sn := drivePlayersUntilSettlement(t, conns)
	if sn == nil || sn.GetRoomId() != roomID {
		t.Fatalf("结算房间号不一致: %+v", sn)
	}
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位当前测试文件")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func reserveTCPAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("申请临时端口失败: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("关闭临时监听失败: %v", err)
	}
	return addr
}

func writeConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写配置失败: %v", err)
	}
	return path
}

func startProc(t *testing.T, ctx context.Context, repoRoot, target, cfgPath string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "go", "run", target)

	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "LSP_CONFIG="+cfgPath)
	logFile, _ := os.CreateTemp("", "proc-*.log")
	// Stdout 与 Stderr 分别独占 logFile；不共享 bytes.Buffer，避免两个 exec goroutine 并发写同一缓冲产生数据竞争。
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动进程 %s 失败: %v", target, err)
	}
	t.Logf("进程 %s 日志文件: %s", target, logFile.Name())
	t.Cleanup(func() {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			t.Logf("%s 退出 (err=%v), 日志见: %s", target, err, logFile.Name())
		case <-cancelCtx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			t.Logf("%s 超时强杀，日志见: %s", target, logFile.Name())
		}
		_ = logFile.Close()
	})
}

func waitForTCP(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	dialer := &net.Dialer{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		conn, err := dialer.DialContext(t.Context(), "tcp", addr)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("等待端口可用超时: %s", addr)
}
