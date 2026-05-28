package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"racoo.cn/lsp/internal/metrics"
	storex "racoo.cn/lsp/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// settlementPool 约束结算写入所需连接能力。
type settlementPool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// SettlementRecord 是结算存储的内部记录类型，与传输层协议解耦。
type SettlementRecord struct {
	RoomID        string
	WinnerUserIDs []string
	TotalFan      int32
	DetailText    string
	// Payload 是已序列化的 proto 结算摘要字节，供 GetLatestSettlement 原样返回给调用方。
	Payload    []byte
	RoundIndex *int32
	HandIndex  *int32
}

// SettlementStore 写入结算历史。
type SettlementStore struct {
	pool settlementPool
}

var ErrSettlementNotFound = errors.New("settlement not found")

// NewSettlementStore 创建结算存储。
func NewSettlementStore(pool settlementPool) *SettlementStore {
	if pool == nil {
		return nil
	}
	return &SettlementStore{pool: pool}
}

// AppendSettlement 记录一局结算摘要。
func (s *SettlementStore) AppendSettlement(ctx context.Context, rec SettlementRecord) error {
	started := time.Now()
	var opErr error
	defer func() { metrics.ObserveStorage("postgres", "append_settlement", started, opErr) }()
	if s == nil || s.pool == nil {
		opErr = fmt.Errorf("nil settlement store")
		return opErr
	}
	if rec.RoomID == "" {
		opErr = fmt.Errorf("empty room_id in settlement record")
		return opErr
	}
	opCtx, cancel := storex.WithOperationTimeout(ctx)
	defer cancel()
	_, opErr = s.pool.Exec(opCtx, `
		INSERT INTO settlements (room_id, winner_user_ids, total_fan, detail_text, payload, round_index, hand_index)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (room_id, round_index, hand_index)
		WHERE round_index IS NOT NULL AND hand_index IS NOT NULL
		DO UPDATE SET
		    winner_user_ids = EXCLUDED.winner_user_ids,
		    total_fan = EXCLUDED.total_fan,
		    detail_text = EXCLUDED.detail_text,
		    payload = EXCLUDED.payload
	`, rec.RoomID, rec.WinnerUserIDs, rec.TotalFan, rec.DetailText, rec.Payload, rec.RoundIndex, rec.HandIndex)
	return opErr
}

// HasSettlement 判断房间是否已有结算记录。
func (s *SettlementStore) HasSettlement(ctx context.Context, roomID string) (bool, error) {
	started := time.Now()
	var opErr error
	defer func() { metrics.ObserveStorage("postgres", "has_settlement", started, opErr) }()
	if s == nil || s.pool == nil {
		opErr = fmt.Errorf("nil settlement store")
		return false, opErr
	}
	var n int
	opCtx, cancel := storex.WithOperationTimeout(ctx)
	defer cancel()
	opErr = s.pool.QueryRow(opCtx, `SELECT COUNT(1) FROM settlements WHERE room_id = $1`, roomID).Scan(&n)
	if opErr != nil {
		return false, opErr
	}
	return n > 0, nil
}

// GetLatestSettlement 读取房间最近一次结算的序列化 payload（proto bytes），供断线重连 fallback。
// 调用方负责将 payload 解析为 proto 消息并推送给客户端。
func (s *SettlementStore) GetLatestSettlement(ctx context.Context, roomID string) ([]byte, error) {
	started := time.Now()
	var opErr error
	defer func() { metrics.ObserveStorage("postgres", "get_latest_settlement", started, opErr) }()
	if s == nil || s.pool == nil {
		opErr = fmt.Errorf("nil settlement store")
		return nil, opErr
	}
	var payload []byte
	opCtx, cancel := storex.WithOperationTimeout(ctx)
	defer cancel()
	err := s.pool.QueryRow(opCtx, `
		SELECT payload
		FROM settlements
		WHERE room_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, roomID).Scan(&payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			opErr = ErrSettlementNotFound
			return nil, ErrSettlementNotFound
		}
		opErr = err
		return nil, err
	}
	return payload, nil
}
