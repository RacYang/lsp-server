package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func TestEvaluateLoginEnvelopeAccept(t *testing.T) {
	env := &clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		UserId:       "u1",
		SessionToken: "tok-new",
	}}}
	v := evaluateLoginEnvelope(env, true)
	require.Equal(t, loginDecisionAccept, v.decision)
	require.Equal(t, "u1", v.userID)
	require.Equal(t, "tok-new", v.sessionToken)
}

func TestEvaluateLoginEnvelopeUnauthorizedWithTokenTriggersRetry(t *testing.T) {
	env := &clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
		ErrorMessage: "token expired",
	}}}
	v := evaluateLoginEnvelope(env, true)
	require.Equal(t, loginDecisionClearTokenAndRetry, v.decision)
	require.Equal(t, "token expired", v.errorMessage)
}

func TestEvaluateLoginEnvelopeUnauthorizedWithoutTokenIsFatal(t *testing.T) {
	env := &clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
		ErrorMessage: "bad nick",
	}}}
	v := evaluateLoginEnvelope(env, false)
	require.Equal(t, loginDecisionFatal, v.decision)
}

func TestEvaluateLoginEnvelopeRouteRedirect(t *testing.T) {
	env := &clientv1.Envelope{Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{
		WsUrl: "wss://other.example/ws",
	}}}
	v := evaluateLoginEnvelope(env, false)
	require.Equal(t, loginDecisionRedirect, v.decision)
	require.Equal(t, "wss://other.example/ws", v.redirectURL)
}

func TestEvaluateLoginEnvelopeRouteRedirectLoginRespAwaitsNotify(t *testing.T) {
	env := &clientv1.Envelope{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
		ErrorCode:    clientv1.ErrorCode_ERROR_CODE_ROUTE_REDIRECT,
		ErrorMessage: "moved",
	}}}
	v := evaluateLoginEnvelope(env, true)
	require.Equal(t, loginDecisionAwaitRedirect, v.decision)
	require.Equal(t, "moved", v.errorMessage)
}

func TestEvaluateLoginEnvelopeIgnoresUnrelated(t *testing.T) {
	env := &clientv1.Envelope{Body: &clientv1.Envelope_HeartbeatResp{HeartbeatResp: &clientv1.HeartbeatResponse{}}}
	v := evaluateLoginEnvelope(env, false)
	require.Equal(t, loginDecisionPending, v.decision)
}

// fakeRunner 是 silentLoginRunner 的可控制实现，用于在不联网的情况下驱动 SilentLogin。
type fakeRunner struct {
	mu        sync.Mutex
	scripts   [][]*clientv1.Envelope // 每次 runSession 投递一份脚本
	returnErr []error
	calls     int
}

func (f *fakeRunner) runSession(ctx context.Context, sink func(*clientv1.Envelope)) error {
	f.mu.Lock()
	idx := f.calls
	f.calls++
	var script []*clientv1.Envelope
	if idx < len(f.scripts) {
		script = f.scripts[idx]
	}
	var err error
	if idx < len(f.returnErr) {
		err = f.returnErr[idx]
	}
	f.mu.Unlock()
	for _, env := range script {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			sink(env)
		}
	}
	if err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestSilentLoginAcceptsOnFirstAttempt(t *testing.T) {
	runner := &fakeRunner{
		scripts: [][]*clientv1.Envelope{
			{
				{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
					UserId:       "u1",
					SessionToken: "tok-fresh",
				}}},
			},
		},
	}
	res, err := silentLoginCore(context.Background(), silentLoginOptions{
		runner:       runner,
		hasTokenNow:  func() bool { return true },
		clearToken:   func() {},
		updateServer: func(string) {},
	})
	require.NoError(t, err)
	require.Equal(t, "u1", res.UserID)
	require.Equal(t, "tok-fresh", res.SessionToken)
	require.Equal(t, 1, runner.calls)
}

func TestSilentLoginClearsTokenAndRetriesOnUnauthorized(t *testing.T) {
	runner := &fakeRunner{
		scripts: [][]*clientv1.Envelope{
			{{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
				ErrorCode:    clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED,
				ErrorMessage: "token expired",
			}}}},
			{{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
				UserId:       "u2",
				SessionToken: "tok-new",
			}}}},
		},
	}
	hasToken := true
	cleared := false
	res, err := silentLoginCore(context.Background(), silentLoginOptions{
		runner: runner,
		hasTokenNow: func() bool {
			defer func() { hasToken = false }()
			return hasToken
		},
		clearToken:   func() { cleared = true },
		updateServer: func(string) {},
	})
	require.NoError(t, err)
	require.True(t, cleared)
	require.Equal(t, 2, runner.calls)
	require.Equal(t, "u2", res.UserID)
}

func TestSilentLoginFollowsRouteRedirect(t *testing.T) {
	runner := &fakeRunner{
		scripts: [][]*clientv1.Envelope{
			{{Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{
				WsUrl: "wss://b/ws",
			}}}},
			{{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
				UserId:       "u3",
				SessionToken: "tok-b",
			}}}},
		},
	}
	var serverURL string
	res, err := silentLoginCore(context.Background(), silentLoginOptions{
		runner:       runner,
		hasTokenNow:  func() bool { return false },
		clearToken:   func() {},
		updateServer: func(s string) { serverURL = s },
	})
	require.NoError(t, err)
	require.Equal(t, "wss://b/ws", serverURL)
	require.Equal(t, "u3", res.UserID)
}

func TestSilentLoginFollowsLoginRespThenRouteRedirect(t *testing.T) {
	runner := &fakeRunner{
		scripts: [][]*clientv1.Envelope{
			{
				{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
					ErrorCode:    clientv1.ErrorCode_ERROR_CODE_ROUTE_REDIRECT,
					ErrorMessage: "会话迁移",
				}}},
				{Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{
					WsUrl: "wss://b/ws",
				}}},
			},
			{{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
				UserId:       "u4",
				SessionToken: "tok-b",
			}}}},
		},
	}
	var serverURL string
	res, err := silentLoginCore(context.Background(), silentLoginOptions{
		runner:       runner,
		hasTokenNow:  func() bool { return true },
		clearToken:   func() {},
		updateServer: func(s string) { serverURL = s },
	})
	require.NoError(t, err)
	require.Equal(t, "wss://b/ws", serverURL)
	require.Equal(t, "u4", res.UserID)
}

func TestSilentLoginRouteRedirectWithoutURLFailsClearly(t *testing.T) {
	runner := &fakeRunner{
		scripts: [][]*clientv1.Envelope{
			{{Body: &clientv1.Envelope_LoginResp{LoginResp: &clientv1.LoginResponse{
				ErrorCode:    clientv1.ErrorCode_ERROR_CODE_ROUTE_REDIRECT,
				ErrorMessage: "会话迁移",
			}}}},
		},
	}
	_, err := silentLoginCore(context.Background(), silentLoginOptions{
		runner:       runner,
		maxAttempts:  1,
		loginTimeout: 50 * time.Millisecond,
		hasTokenNow:  func() bool { return true },
		clearToken:   func() {},
		updateServer: func(string) {},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "服务端要求重连但未给出地址")
}

func TestSilentLoginGivesUpAfterMaxAttempts(t *testing.T) {
	runner := &fakeRunner{
		scripts: [][]*clientv1.Envelope{
			{{Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{WsUrl: "wss://x/ws"}}}},
			{{Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{WsUrl: "wss://y/ws"}}}},
			{{Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{WsUrl: "wss://z/ws"}}}},
		},
	}
	_, err := silentLoginCore(context.Background(), silentLoginOptions{
		runner:       runner,
		maxAttempts:  3,
		hasTokenNow:  func() bool { return false },
		clearToken:   func() {},
		updateServer: func(string) {},
	})
	require.Error(t, err)
	require.Equal(t, 3, runner.calls)
}

func TestSilentLoginReturnsTimeoutWhenNoResponse(t *testing.T) {
	runner := &fakeRunner{}
	_, err := silentLoginCore(context.Background(), silentLoginOptions{
		runner:       runner,
		maxAttempts:  1,
		loginTimeout: 50 * time.Millisecond,
		hasTokenNow:  func() bool { return false },
		clearToken:   func() {},
		updateServer: func(string) {},
	})
	require.ErrorIs(t, err, errLoginTimeout)
}

func TestSilentLoginPropagatesRunnerError(t *testing.T) {
	want := errors.New("boom")
	runner := &fakeRunner{returnErr: []error{want}}
	_, err := silentLoginCore(context.Background(), silentLoginOptions{
		runner:       runner,
		maxAttempts:  1,
		hasTokenNow:  func() bool { return false },
		clearToken:   func() {},
		updateServer: func(string) {},
	})
	require.ErrorIs(t, err, want)
}
