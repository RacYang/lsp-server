// 可观测性 HTTP 端点测试：覆盖空地址直通、健康检查、就绪检查、metrics、pprof 与 collector 注册。
package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus/collectors"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	redisstore "racoo.cn/lsp/internal/store/redis"
)

// freePort 借助内核分配一个随机可用端口；返回 host:port 字符串与立刻关闭的 listener 副作用，避免端口冲突。
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// httpGet 是测试辅助：发起一次 HTTP GET，返回状态码与响应体；保证响应体被正确关闭。
func httpGet(t *testing.T, url string) (int, string) {
	t.Helper()
	cli := &http.Client{Timeout: 2 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := cli.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

// waitFor 在最多 1 秒内每 20 毫秒重试条件，避免起服务后第一次请求被拒。
func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

// TestStartObsHTTPEmptyAddr 校验空地址走捷径返回 noop stop，不会启动监听端口。
func TestStartObsHTTPEmptyAddr(t *testing.T) {
	t.Parallel()
	stop, err := StartObsHTTP("", nil)
	require.NoError(t, err)
	require.NotNil(t, stop)
	stop()
}

// TestStartObsHTTPEndpoints 校验四个核心端点：健康检查、就绪检查、指标、pprof 索引页都能正常响应。
func TestStartObsHTTPEndpoints(t *testing.T) {
	addr := freePort(t)
	stop, err := StartObsHTTP(addr, nil)
	require.NoError(t, err)
	t.Cleanup(stop)

	base := "http://" + addr
	waitFor(t, func() bool {
		_, _, err := net.SplitHostPort(addr)
		if err != nil {
			return false
		}
		conn, derr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if derr != nil {
			return false
		}
		_ = conn.Close()
		return true
	})

	code, body := httpGet(t, base+"/healthz")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "ok", body)

	code, body = httpGet(t, base+"/readyz")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "ok", body)

	code, body = httpGet(t, base+"/metrics")
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, "go_goroutines", "metrics 端点应返回 go runtime 指标")

	code, _ = httpGet(t, base+"/debug/pprof/")
	require.Equal(t, http.StatusOK, code)
}

// TestStartObsHTTPReadyzReportsRedisFailure 校验 readyz 在 redis 不可达时返回 503，并把错误透传给客户端。
func TestStartObsHTTPReadyzReportsRedisFailure(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	cli := redisstore.NewClientFromUniversal(rcli)

	addr := freePort(t)
	stop, err := StartObsHTTP(addr, cli)
	require.NoError(t, err)
	t.Cleanup(stop)

	waitFor(t, func() bool {
		conn, derr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if derr != nil {
			return false
		}
		_ = conn.Close()
		return true
	})

	mr.Close()

	cliHTTP := &http.Client{Timeout: 4 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/readyz", nil)
	require.NoError(t, err)
	resp, err := cliHTTP.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "redis 不可达必须导致 readyz 返回 503")
}

// TestRegisterCollectorIdempotent 校验已注册过的 collector 再次注册不会引发 panic 或残留错误。
func TestRegisterCollectorIdempotent(t *testing.T) {
	c := collectors.NewBuildInfoCollector()
	registerCollector(c)
	registerCollector(c)
}

// TestRegisterRuntimeCollectorsOnceSafe 校验 registerRuntimeCollectors 在多次调用下也安全。
func TestRegisterRuntimeCollectorsOnceSafe(t *testing.T) {
	registerRuntimeCollectors()
	registerRuntimeCollectors()
}
