package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/frame"
	"racoo.cn/lsp/internal/session"
)

func allowWebSocketOrigin(r *http.Request, allowedOrigins []string) bool {
	if r == nil {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if len(allowedOrigins) > 0 {
		for _, allowed := range allowedOrigins {
			if strings.EqualFold(strings.TrimSpace(allowed), origin) {
				return true
			}
		}
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func shouldDropRequest(env *clientv1.Envelope, msgID uint16, userID string) bool {
	if env == nil {
		return false
	}
	if limiter := defaultWSRateLimiter.Load(); limiter != nil && !limiter.Allow(userID) {
		rateLimitedTotal.WithLabelValues("gate").Inc()
		return true
	}
	key := strings.TrimSpace(env.GetIdempotencyKey())
	if key == "" {
		return false
	}
	if cache := defaultWSIdemCache.Load(); cache != nil && cache.SeenOrStore("ws", msgID, userID, key) {
		idempotentReplayTotal.Inc()
		return true
	}
	return false
}

func respondAction(conn *websocket.Conn, reqID string, responseMsgID uint16, env *clientv1.Envelope, after func()) {
	b, _ := proto.Marshal(env)
	_ = session.WriteBinary(conn, frame.Encode(responseMsgID, b))
	if after != nil {
		after()
	}
}
