package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"racoo.cn/lsp/internal/metrics"
	storex "racoo.cn/lsp/internal/store"
)

// roomEventPool 约束事件存储所需连接能力；*pgxpool.Pool 与 pgxmock 均可注入。
type roomEventPool interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// RoomEventRow 为单条持久化房间事件。
type RoomEventRow struct {
	RoomID     string
	Seq        int64
	Kind       string
	Payload    []byte
	TargetSeat int32
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
	rows, err := s.AppendEvents(ctx, roomID, []RoomEventRow{{Kind: kind, Payload: payload, TargetSeat: -1}})
	if err != nil {
		return 0, "", err
	}
	if len(rows) == 0 {
		return 0, "", fmt.Errorf("append event returned no rows")
	}
	row := rows[0]
	return row.Seq, fmt.Sprintf("%s:%d", row.RoomID, row.Seq), nil
}

// AppendEvents 在单事务内为一批事件连续分配 seq 并写入；要么全部成功，要么全部回滚。
func (s *RoomEventStore) AppendEvents(ctx context.Context, roomID string, events []RoomEventRow) ([]RoomEventRow, error) {
	started := time.Now()
	var opErr error
	defer func() { metrics.ObserveStorage("postgres", "append_events", started, opErr) }()
	if s == nil || s.pool == nil {
		opErr = fmt.Errorf("nil room event store")
		return nil, opErr
	}
	if roomID == "" {
		opErr = fmt.Errorf("empty room_id")
		return nil, opErr
	}
	if len(events) == 0 {
		return nil, nil
	}
	for _, event := range events {
		if event.Kind == "" {
			opErr = fmt.Errorf("empty kind")
			return nil, opErr
		}
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		opErr = err
		return nil, err
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
		opErr = fmt.Errorf("alloc seq: %w", err)
		return nil, opErr
	}

	out := make([]RoomEventRow, 0, len(events))
	for _, event := range events {
		if event.TargetSeat < -1 || event.TargetSeat > 3 {
			opErr = fmt.Errorf("invalid target seat: %d", event.TargetSeat)
			return nil, opErr
		}
		if _, err := tx.Exec(ctx, `INSERT INTO room_events (room_id, seq, kind, payload, target_seat) VALUES ($1, $2, $3, $4, $5)`, roomID, next, event.Kind, event.Payload, event.TargetSeat); err != nil {
			opErr = fmt.Errorf("insert event: %w", err)
			return nil, opErr
		}
		out = append(out, RoomEventRow{
			RoomID:     roomID,
			Seq:        next,
			Kind:       event.Kind,
			Payload:    append([]byte(nil), event.Payload...),
			TargetSeat: event.TargetSeat,
		})
		next++
	}
	if err := tx.Commit(ctx); err != nil {
		opErr = err
		return nil, err
	}
	return out, nil
}

// ListEventsAfter 返回 seq 严格大于 afterSeq 的事件，按 seq 升序。
func (s *RoomEventStore) ListEventsAfter(ctx context.Context, roomID string, afterSeq int64) ([]RoomEventRow, error) {
	started := time.Now()
	var opErr error
	defer func() { metrics.ObserveStorage("postgres", "list_events_after", started, opErr) }()
	if s == nil || s.pool == nil {
		opErr = fmt.Errorf("nil room event store")
		return nil, opErr
	}
	var rows pgx.Rows
	err := storex.Retry(ctx, "postgres", "list_events_after", 2, func(opCtx context.Context) error {
		var err error
		rows, err = s.pool.Query(opCtx, `SELECT room_id, seq, kind, payload, target_seat FROM room_events WHERE room_id = $1 AND seq > $2 ORDER BY seq ASC`, roomID, afterSeq)
		return err
	})
	if err != nil {
		opErr = err
		return nil, err
	}
	defer rows.Close()
	var out []RoomEventRow
	for rows.Next() {
		var r RoomEventRow
		if err := rows.Scan(&r.RoomID, &r.Seq, &r.Kind, &r.Payload, &r.TargetSeat); err != nil {
			opErr = err
			return nil, err
		}
		out = append(out, r)
	}
	opErr = rows.Err()
	return out, opErr
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
