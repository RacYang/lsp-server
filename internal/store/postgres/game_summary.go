package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"racoo.cn/lsp/internal/metrics"
	storex "racoo.cn/lsp/internal/store"
)

// gameSummaryPool 约束对局摘要读写所需连接能力。
type gameSummaryPool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// GameSummary 表示一局对局的起止摘要。
type GameSummary struct {
	RoomID    string
	RuleID    string
	PlayerIDs []string
	CreatedAt time.Time
	EndedAt   *time.Time
}

// GameSummaryStore 维护对局摘要表。
type GameSummaryStore struct {
	pool gameSummaryPool
}

var ErrGameSummaryNotFound = errors.New("game summary not found")

// NewGameSummaryStore 创建摘要存储。
func NewGameSummaryStore(pool gameSummaryPool) *GameSummaryStore {
	if pool == nil {
		return nil
	}
	return &GameSummaryStore{pool: pool}
}

// CreateGameSummary 在首帧落地前确保存在对局摘要。
func (s *GameSummaryStore) CreateGameSummary(ctx context.Context, roomID, ruleID string, playerIDs []string) error {
	started := time.Now()
	var opErr error
	defer func() { metrics.ObserveStorage("postgres", "create_game_summary", started, opErr) }()
	if s == nil || s.pool == nil {
		opErr = fmt.Errorf("nil game summary store")
		return opErr
	}
	if roomID == "" || ruleID == "" {
		opErr = fmt.Errorf("empty room_id or rule_id")
		return opErr
	}
	opCtx, cancel := storex.WithOperationTimeout(ctx)
	defer cancel()
	_, opErr = s.pool.Exec(opCtx, `
		INSERT INTO game_summaries (room_id, rule_id, player_ids)
		VALUES ($1, $2, $3)
		ON CONFLICT (room_id) DO UPDATE
		SET rule_id = EXCLUDED.rule_id,
		    player_ids = EXCLUDED.player_ids
	`, roomID, ruleID, playerIDs)
	return opErr
}

// EndGameSummary 标记该局已结束。
func (s *GameSummaryStore) EndGameSummary(ctx context.Context, roomID string, endedAt time.Time) error {
	started := time.Now()
	var opErr error
	defer func() { metrics.ObserveStorage("postgres", "end_game_summary", started, opErr) }()
	if s == nil || s.pool == nil {
		opErr = fmt.Errorf("nil game summary store")
		return opErr
	}
	if roomID == "" {
		opErr = fmt.Errorf("empty room_id")
		return opErr
	}
	opCtx, cancel := storex.WithOperationTimeout(ctx)
	defer cancel()
	_, opErr = s.pool.Exec(opCtx, `UPDATE game_summaries SET ended_at = $2 WHERE room_id = $1`, roomID, endedAt.UTC())
	return opErr
}

// GetGameSummary 返回某房间最新摘要。
func (s *GameSummaryStore) GetGameSummary(ctx context.Context, roomID string) (GameSummary, error) {
	started := time.Now()
	var opErr error
	defer func() { metrics.ObserveStorage("postgres", "get_game_summary", started, opErr) }()
	if s == nil || s.pool == nil {
		opErr = fmt.Errorf("nil game summary store")
		return GameSummary{}, opErr
	}
	var summary GameSummary
	var endedAt *time.Time
	opCtx, cancel := storex.WithOperationTimeout(ctx)
	defer cancel()
	err := s.pool.QueryRow(opCtx, `
		SELECT room_id, rule_id, player_ids, created_at, ended_at
		FROM game_summaries
		WHERE room_id = $1
	`, roomID).Scan(&summary.RoomID, &summary.RuleID, &summary.PlayerIDs, &summary.CreatedAt, &endedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			opErr = ErrGameSummaryNotFound
			return GameSummary{}, ErrGameSummaryNotFound
		}
		opErr = err
		return GameSummary{}, err
	}
	summary.EndedAt = endedAt
	return summary, nil
}
