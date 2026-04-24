package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// roomEventPool 约束事件存储所需连接能力；*pgxpool.Pool 与 pgxmock 均可注入。
type roomEventPool interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// RoomEventRow 为单条持久化房间事件。
type RoomEventRow struct {
	RoomID  string
	Seq     int64
	Kind    string
	Payload []byte
}

// RoomEventStore 追加并查询房间事件日志。
type RoomEventStore struct {
	pool roomEventPool
}

// NewRoomEventStore 创建事件存储；pool 不可为空。
func NewRoomEventStore(pool roomEventPool) *RoomEventStore {
	if pool == nil {
		return nil
	}
	return &RoomEventStore{pool: pool}
}

// AppendEvent 在事务内分配 seq 并写入；返回新 seq 与游标字符串。
func (s *RoomEventStore) AppendEvent(ctx context.Context, roomID, kind string, payload []byte) (seq int64, cursor string, err error) {
	if s == nil || s.pool == nil {
		return 0, "", fmt.Errorf("nil room event store")
	}
	if roomID == "" || kind == "" {
		return 0, "", fmt.Errorf("empty room_id or kind")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var next int64
	var lastSeq int64
	err = tx.QueryRow(ctx, `SELECT seq FROM room_events WHERE room_id = $1 ORDER BY seq DESC LIMIT 1 FOR UPDATE`, roomID).Scan(&lastSeq)
	switch {
	case err == nil:
		next = lastSeq + 1
	case errors.Is(err, pgx.ErrNoRows):
		next = 1
	default:
		return 0, "", fmt.Errorf("alloc seq: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO room_events (room_id, seq, kind, payload) VALUES ($1, $2, $3, $4)`, roomID, next, kind, payload); err != nil {
		return 0, "", fmt.Errorf("insert event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, "", err
	}
	return next, fmt.Sprintf("%s:%d", roomID, next), nil
}

// ListEventsAfter 返回 seq 严格大于 afterSeq 的事件，按 seq 升序。
func (s *RoomEventStore) ListEventsAfter(ctx context.Context, roomID string, afterSeq int64) ([]RoomEventRow, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("nil room event store")
	}
	rows, err := s.pool.Query(ctx, `SELECT room_id, seq, kind, payload FROM room_events WHERE room_id = $1 AND seq > $2 ORDER BY seq ASC`, roomID, afterSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoomEventRow
	for rows.Next() {
		var r RoomEventRow
		if err := rows.Scan(&r.RoomID, &r.Seq, &r.Kind, &r.Payload); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MaxSeq 返回房间当前最大 seq；无记录时为 0。
func (s *RoomEventStore) MaxSeq(ctx context.Context, roomID string) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("nil room event store")
	}
	var m int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(seq), 0) FROM room_events WHERE room_id = $1`, roomID).Scan(&m)
	return m, err
}
