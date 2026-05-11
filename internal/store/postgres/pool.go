package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Pinger 约束连接池的轻量健康检查能力。
type Pinger interface {
	Ping(context.Context) error
}

// PoolOptions 定义 PostgreSQL 连接池可显式治理的运行时参数。
type PoolOptions struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// OpenPool 打开连接池并执行内嵌迁移（幂等 CREATE）。
func OpenPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return OpenPoolWithOptions(ctx, dsn, PoolOptions{})
}

// OpenPoolWithOptions 打开连接池、应用显式池配置并执行内嵌迁移（幂等 CREATE）。
func OpenPoolWithOptions(ctx context.Context, dsn string, opts PoolOptions) (*pgxpool.Pool, error) {
	cfg, err := ParseConfig(dsn, opts)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := applyMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// ParseConfig 解析 DSN 并叠加显式连接池选项；零值选项保留 DSN/pgx 默认值。
func ParseConfig(dsn string, opts PoolOptions) (*pgxpool.Config, error) {
	if dsn == "" {
		return nil, fmt.Errorf("empty postgres dsn")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	applyPoolOptions(cfg, opts)
	return cfg, nil
}

func applyPoolOptions(cfg *pgxpool.Config, opts PoolOptions) {
	if opts.MaxConns > 0 {
		cfg.MaxConns = opts.MaxConns
	}
	if opts.MinConns > 0 {
		cfg.MinConns = opts.MinConns
	}
	if opts.MaxConnLifetime > 0 {
		cfg.MaxConnLifetime = opts.MaxConnLifetime
	}
	if opts.MaxConnIdleTime > 0 {
		cfg.MaxConnIdleTime = opts.MaxConnIdleTime
	}
	if opts.HealthCheckPeriod > 0 {
		cfg.HealthCheckPeriod = opts.HealthCheckPeriod
	}
}

func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migration dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		sqlBytes, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}
