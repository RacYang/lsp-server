// Package store 提供 Redis 与 PostgreSQL 共享的存储弹性辅助函数。
package store

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"racoo.cn/lsp/internal/metrics"
)

const defaultOperationTimeout = 3 * time.Second

// WithOperationTimeout 在调用方未设置 deadline 时补充默认存储超时。
func WithOperationTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, defaultOperationTimeout)
}

// IsRetryable 判断错误是否适合在幂等存储操作上短重试。
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return strings.HasPrefix(pgErr.Code, "08") || pgErr.Code == "40001" || pgErr.Code == "40P01"
	}
	return false
}

// Retry 执行幂等存储操作的有限退避重试，并通过独立 counter 暴露重试事实。
func Retry(ctx context.Context, storeName, op string, attempts int, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if attempts <= 0 {
		attempts = 1
	}
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		opCtx, cancel := WithOperationTimeout(ctx)
		err = fn(opCtx)
		cancel()
		if err == nil || !IsRetryable(err) || attempt == attempts {
			break
		}
		metrics.StorageRetryTotal.WithLabelValues(storeName, op, "retry").Inc()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 25 * time.Millisecond):
		}
	}
	if err != nil && IsRetryable(err) {
		metrics.StorageRetryTotal.WithLabelValues(storeName, op, "exhausted").Inc()
	}
	return err
}
