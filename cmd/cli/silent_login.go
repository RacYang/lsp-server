package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

// loginDecision 描述 SilentLogin 收到响应后该执行的下一步动作。
type loginDecision int

const (
	loginDecisionPending loginDecision = iota
	loginDecisionAccept
	loginDecisionClearTokenAndRetry
	loginDecisionRedirect
	loginDecisionAwaitRedirect
	loginDecisionFatal
)

// loginVerdict 是 evaluateLoginEnvelope 的判定结果，连同必要的副作用上下文一起返回。
type loginVerdict struct {
	decision     loginDecision
	redirectURL  string
	errorMessage string
	resumed      bool
	sessionToken string
	userID       string
}

// evaluateLoginEnvelope 是纯策略函数：给定一个 envelope 与当前是否带 token，
// 决定 SilentLogin 应该接受/清 token 重试/跟随重定向/放弃。
//
// 拆成纯函数的目的是把"协议解析 + 重试策略"作为可独立单测的最小单元，
// 真实网络层与超时控制留在 SilentLogin。
func evaluateLoginEnvelope(env *clientv1.Envelope, hadToken bool) loginVerdict {
	if env == nil {
		return loginVerdict{decision: loginDecisionPending}
	}
	if route := env.GetRouteRedirect(); route != nil && route.GetWsUrl() != "" {
		return loginVerdict{decision: loginDecisionRedirect, redirectURL: route.GetWsUrl()}
	}
	resp := env.GetLoginResp()
	if resp == nil {
		return loginVerdict{decision: loginDecisionPending}
	}
	if resp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return loginVerdict{
			decision:     loginDecisionAccept,
			sessionToken: resp.GetSessionToken(),
			userID:       resp.GetUserId(),
			resumed:      resp.GetResumed(),
		}
	}
	// 仅当之前提交了 token 才有"清 token 再试"的语义；否则视为致命错误。
	if resp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED && hadToken {
		return loginVerdict{decision: loginDecisionClearTokenAndRetry, errorMessage: resp.GetErrorMessage()}
	}
	if resp.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_ROUTE_REDIRECT {
		return loginVerdict{decision: loginDecisionAwaitRedirect, errorMessage: resp.GetErrorMessage()}
	}
	return loginVerdict{decision: loginDecisionFatal, errorMessage: resp.GetErrorMessage()}
}

// silentLoginRunner 抽象一次"建立连接并消费 envelope"的能力，便于测试用 fake 替换。
type silentLoginRunner interface {
	// runSession 在连接生命周期内不断把 envelope 投递到 sink；
	// 调用方通过 cancel ctx 显式终止连接。
	runSession(ctx context.Context, sink func(*clientv1.Envelope)) error
}

// silentLoginResult 是 SilentLogin 成功完成的结果集，调用方据此持久化配置。
type silentLoginResult struct {
	UserID       string
	SessionToken string
	ServerURL    string
	Resumed      bool
}

// silentLoginOptions 收集 SilentLogin 所需的依赖与可调参数，避免函数签名爆炸。
type silentLoginOptions struct {
	maxAttempts  int
	loginTimeout time.Duration
	clearToken   func()
	updateServer func(url string)
	hasTokenNow  func() bool
	logf         func(format string, args ...any)
	runner       silentLoginRunner
}

func (o *silentLoginOptions) withDefaults() {
	if o.maxAttempts <= 0 {
		o.maxAttempts = 3
	}
	if o.loginTimeout <= 0 {
		o.loginTimeout = 15 * time.Second
	}
	if o.logf == nil {
		o.logf = func(string, ...any) {}
	}
}

// errLoginTimeout 表示一次连接成功但服务端长时间没回 LoginResp，玩家应感知为"网络异常"。
var errLoginTimeout = errors.New("登录超时")

// silentLoginCore 是 SilentLogin 的核心循环，从 runner 拿到的 envelope 流中
// 推导出登录结果或继续重试。该函数只依赖 silentLoginOptions 中的 hook，
// 因此可以在测试中用 fake runner 完全模拟服务端行为。
func silentLoginCore(ctx context.Context, opts silentLoginOptions) (*silentLoginResult, error) {
	opts.withDefaults()
	var lastErr error
	for attempt := 0; attempt < opts.maxAttempts; attempt++ {
		runCtx, cancelRun := context.WithCancel(ctx)
		envCh := make(chan *clientv1.Envelope, 16)
		runDone := make(chan error, 1)
		go func() {
			runDone <- opts.runner.runSession(runCtx, func(env *clientv1.Envelope) {
				select {
				case envCh <- env:
				case <-runCtx.Done():
				}
			})
		}()

		verdict, err := waitForLoginVerdict(runCtx, envCh, runDone, opts.hasTokenNow(), opts.loginTimeout)
		cancelRun()
		// runDone 容量为 1，runner goroutine 在 cancel 后会写入并自然退出，
		// 此处不显式 drain，避免与 waitForLoginVerdict 的内部 drain 冲突而死锁。

		if err != nil {
			lastErr = err
			opts.logf("登录尝试 %d 失败: %v", attempt+1, err)
			continue
		}
		switch verdict.decision {
		case loginDecisionAccept:
			return &silentLoginResult{
				UserID:       verdict.userID,
				SessionToken: verdict.sessionToken,
				Resumed:      verdict.resumed,
			}, nil
		case loginDecisionClearTokenAndRetry:
			opts.clearToken()
			opts.logf("会话失效，已清除本地令牌后重试 (%s)", verdict.errorMessage)
		case loginDecisionRedirect:
			opts.updateServer(verdict.redirectURL)
			opts.logf("收到路由重定向，切换到 %s 重试", verdict.redirectURL)
		case loginDecisionAwaitRedirect:
			lastErr = errors.New("服务端要求重连但未给出地址")
		case loginDecisionFatal:
			return nil, fmt.Errorf("登录失败: %s", verdict.errorMessage)
		default:
			lastErr = errors.New("未识别的登录结果")
		}
	}
	if lastErr == nil {
		lastErr = errors.New("登录达到最大重试次数")
	}
	return nil, lastErr
}

// wsClientLoginRunner 用真实 WSClient.connectOnce 跑一次完整连接生命周期，
// 同时把收到的 envelope 同步投递到 sink (供策略层判定) 和 AppState (维持本地视图)。
//
// 登录成功后 SilentLogin 会主动 cancel 这一连接，调用方再启动 client.Run 做正式连接，
// 这样 lobby 才能持续接管 client.Events()，不需要在登录与正式态之间切换通道所有权。
type wsClientLoginRunner struct {
	client *WSClient
}

// runSession 把 client.Events 的事件透传给登录策略并落到 state，
// 直到 ctx 取消或底层连接异常。
func (r *wsClientLoginRunner) runSession(ctx context.Context, sink func(*clientv1.Envelope)) error {
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		for {
			select {
			case <-ctx.Done():
				return
			case env := <-r.client.Events():
				if env == nil {
					continue
				}
				r.client.state.Apply(env)
				sink(env)
			}
		}
	}()
	err := r.client.connectOnce(ctx)
	<-pumpDone
	return err
}

// SilentLogin 完成首次登录验证；成功时返回 nil，配置写回并把 SessionToken 持久化。
// 失败时根据 LoginResponse 错误码自动清 token 或跟随路由重定向重试，最多 3 次。
//
// SilentLogin 只是验证连接，登录成功后会主动断开。调用方应继续启动 client.Run
// 做正式连接，以便 lobby/牌桌主循环接管 events。
func SilentLogin(ctx context.Context, client *WSClient, cfg *Config, configPath string) error {
	if cfg == nil {
		return errors.New("配置指针为空")
	}
	runner := &wsClientLoginRunner{client: client}
	res, err := silentLoginCore(ctx, silentLoginOptions{
		runner: runner,
		hasTokenNow: func() bool {
			return readToken(client.tokenFile) != ""
		},
		clearToken: func() {
			if client.tokenFile != "" {
				_ = os.Remove(client.tokenFile)
			}
			cfg.SessionToken = ""
		},
		updateServer: func(url string) {
			cfg.ServerURL = url
			client.SetConfig(url, "")
		},
		logf: func(format string, args ...any) {
			client.state.AddLog(fmt.Sprintf(format, args...))
		},
	})
	if err != nil {
		return err
	}
	if res != nil && res.SessionToken != "" {
		cfg.SessionToken = res.SessionToken
	}
	if configPath != "" {
		if saveErr := SaveConfig(configPath, *cfg); saveErr != nil {
			client.state.AddLog("保存配置失败: " + saveErr.Error())
		}
	}
	return nil
}

// waitForLoginVerdict 在单次连接内消费 envelope，直到拿到判定结果或超时/失败。
func waitForLoginVerdict(ctx context.Context, envCh <-chan *clientv1.Envelope, runDone <-chan error, hadToken bool, timeout time.Duration) (loginVerdict, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	awaitingRedirect := false
	for {
		select {
		case <-ctx.Done():
			return loginVerdict{}, ctx.Err()
		case <-timer.C:
			if awaitingRedirect {
				return loginVerdict{}, errors.New("服务端要求重连但未给出地址")
			}
			return loginVerdict{}, errLoginTimeout
		case err := <-runDone:
			if awaitingRedirect {
				return loginVerdict{}, errors.New("服务端要求重连但未给出地址")
			}
			if err == nil {
				err = errors.New("连接在登录前断开")
			}
			return loginVerdict{}, err
		case env := <-envCh:
			v := evaluateLoginEnvelope(env, hadToken)
			if v.decision == loginDecisionAwaitRedirect {
				awaitingRedirect = true
				continue
			}
			if v.decision == loginDecisionPending {
				continue
			}
			return v, nil
		}
	}
}
