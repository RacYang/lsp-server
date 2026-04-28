package handler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/msgid"
	roomsvc "racoo.cn/lsp/internal/service/room"
)

// TestActionErrorCodeMappings 校验动作错误码映射：限流错误（含包装链）必须返回限流码，其他错误一律映射到非法状态码。
func TestActionErrorCodeMappings(t *testing.T) {
	t.Parallel()

	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_RATE_LIMITED, actionErrorCode(roomsvc.ErrRateLimited))
	wrapped := fmt.Errorf("outer: %w", roomsvc.ErrRateLimited)
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_RATE_LIMITED, actionErrorCode(wrapped))
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, actionErrorCode(errors.New("非限流错误")))
}

// TestJoinRoomErrorCodeAllBranches 校验进房错误码映射的全部分支：房间已满、房间不存在、参数非法、未知错误以及空错误。
func TestJoinRoomErrorCodeAllBranches(t *testing.T) {
	t.Parallel()

	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED, joinRoomErrorCode(nil))
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_ROOM_FULL, joinRoomErrorCode(errors.New("Room full: 4/4")))
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_ROOM_NOT_FOUND, joinRoomErrorCode(errors.New("room not found: r1")))
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, joinRoomErrorCode(errors.New("invalid argument: empty")))
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED, joinRoomErrorCode(errors.New("connection refused")))
}

// TestOutboundMsgIDMappings 锁定业务通知种类到出站消息编号的映射，未识别种类必须显式返回布尔标记假，避免静默丢包。
func TestOutboundMsgIDMappings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind roomsvc.Kind
		want uint16
	}{
		{roomsvc.KindExchangeThreeDone, msgid.ExchangeThreeDone},
		{roomsvc.KindQueMenDone, msgid.QueMenDone},
		{roomsvc.KindStartGame, msgid.StartGame},
		{roomsvc.KindDrawTile, msgid.DrawTile},
		{roomsvc.KindAction, msgid.ActionNotify},
		{roomsvc.KindSettlement, msgid.Settlement},
	}
	for _, c := range cases {
		got, ok := outboundMsgID(c.kind)
		require.True(t, ok, "kind %v 应被映射", c.kind)
		require.Equal(t, c.want, got, "kind %v 映射目标错误", c.kind)
	}

	_, ok := outboundMsgID(roomsvc.Kind("__未注册__"))
	require.False(t, ok, "未注册 Kind 应返回 false")
}

// TestActionErrEnvelopeShapes 验证各动作响应封装的形状契约：
//   - 成功路径必须保留传入的回调闭包，便于上层在写完响应后再继续广播；
//   - 错误路径必须把回调置空，避免错误结果还触发后续广播；同时把错误码与错误消息写进响应体。
func TestActionErrEnvelopeShapes(t *testing.T) {
	t.Parallel()

	called := 0
	after := func() { called++ }

	envOK, gotAfter := discardErrEnvelope("r-ok", after, nil)
	require.NotNil(t, envOK.GetDiscardResp())
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED, envOK.GetDiscardResp().GetErrorCode())
	require.NotNil(t, gotAfter, "成功路径不应丢弃 after")
	gotAfter()
	require.Equal(t, 1, called)

	envErr, gotAfter := discardErrEnvelope("r-err", after, errors.New("bad tile"))
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, envErr.GetDiscardResp().GetErrorCode())
	require.Equal(t, "bad tile", envErr.GetDiscardResp().GetErrorMessage())
	require.Nil(t, gotAfter, "错误路径必须丢弃 after，避免错误后还广播")

	type envelopeFn func(string, func(), error) (*clientv1.Envelope, func())
	cases := []struct {
		name string
		fn   envelopeFn
		ok   func(*clientv1.Envelope) clientv1.ErrorCode
	}{
		{"pong", pongErrEnvelope, func(e *clientv1.Envelope) clientv1.ErrorCode { return e.GetPongResp().GetErrorCode() }},
		{"gang", gangErrEnvelope, func(e *clientv1.Envelope) clientv1.ErrorCode { return e.GetGangResp().GetErrorCode() }},
		{"hu", huErrEnvelope, func(e *clientv1.Envelope) clientv1.ErrorCode { return e.GetHuResp().GetErrorCode() }},
		{"exchangeThree", exchangeThreeErrEnvelope, func(e *clientv1.Envelope) clientv1.ErrorCode {
			return e.GetExchangeThreeResp().GetErrorCode()
		}},
		{"queMen", queMenErrEnvelope, func(e *clientv1.Envelope) clientv1.ErrorCode {
			return e.GetQueMenResp().GetErrorCode()
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name+"/ok", func(t *testing.T) {
			t.Parallel()
			env, after2 := c.fn("req", func() {}, nil)
			require.Equal(t, clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED, c.ok(env))
			require.NotNil(t, after2)
		})
		t.Run(c.name+"/err", func(t *testing.T) {
			t.Parallel()
			env, after2 := c.fn("req", func() {}, errors.New("e"))
			require.Equal(t, clientv1.ErrorCode_ERROR_CODE_INVALID_STATE, c.ok(env))
			require.Nil(t, after2)
		})
	}

	envRL, gotAfter := discardErrEnvelope("r-rl", after, fmt.Errorf("wrap: %w", roomsvc.ErrRateLimited))
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_RATE_LIMITED, envRL.GetDiscardResp().GetErrorCode())
	require.Nil(t, gotAfter)
}

// TestConfigureRuntime 校验运行时入口的限流与幂等缓存可被外部覆盖：传入非正值时应回退到默认参数，正值则按入参重建。
func TestConfigureRuntime(t *testing.T) {
	prevLimiter := defaultWSRateLimiter
	prevIdem := defaultWSIdemCache
	t.Cleanup(func() {
		defaultWSRateLimiter = prevLimiter
		defaultWSIdemCache = prevIdem
	})

	ConfigureRuntime(0, 0, 0)
	require.NotNil(t, defaultWSRateLimiter)
	require.InDelta(t, float64(20), defaultWSRateLimiter.rate, 1e-9)
	require.InDelta(t, float64(40), defaultWSRateLimiter.burst, 1e-9)
	require.Equal(t, 4096, defaultWSIdemCache.capacity)

	ConfigureRuntime(5, 10, 32)
	require.InDelta(t, float64(5), defaultWSRateLimiter.rate, 1e-9)
	require.InDelta(t, float64(10), defaultWSRateLimiter.burst, 1e-9)
	require.Equal(t, 32, defaultWSIdemCache.capacity)
}
