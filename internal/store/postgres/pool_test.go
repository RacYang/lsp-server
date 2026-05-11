package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/store/postgres"
)

func TestOpenPoolEmptyDSN(t *testing.T) {
	t.Parallel()
	_, err := postgres.OpenPool(context.Background(), "")
	require.Error(t, err)
}

func TestParseConfigAppliesPoolOptions(t *testing.T) {
	t.Parallel()

	cfg, err := postgres.ParseConfig("postgres://user:pass@localhost:5432/db?sslmode=disable&pool_max_conns=4", postgres.PoolOptions{
		MaxConns:          17,
		MinConns:          2,
		MaxConnLifetime:   45 * time.Minute,
		MaxConnIdleTime:   5 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
	})
	require.NoError(t, err)
	require.EqualValues(t, 17, cfg.MaxConns)
	require.EqualValues(t, 2, cfg.MinConns)
	require.Equal(t, 45*time.Minute, cfg.MaxConnLifetime)
	require.Equal(t, 5*time.Minute, cfg.MaxConnIdleTime)
	require.Equal(t, 30*time.Second, cfg.HealthCheckPeriod)
}
