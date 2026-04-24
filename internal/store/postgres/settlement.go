package postgres

import (
	"context"
	"errors"
	"fmt"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/proto"
)

// settlementPool 约束结算写入所需连接能力。
type settlementPool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
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
func (s *SettlementStore) AppendSettlement(ctx context.Context, settlement *clientv1.SettlementNotify) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("nil settlement store")
	}
	if settlement == nil || settlement.GetRoomId() == "" {
		return fmt.Errorf("nil settlement or empty room_id")
	}
	payload, err := proto.Marshal(settlement)
	if err != nil {
		return fmt.Errorf("marshal settlement payload: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO settlements (room_id, winner_user_ids, total_fan, detail_text, payload)
		VALUES ($1, $2, $3, $4, $5)
	`, settlement.GetRoomId(), settlement.GetWinnerUserIds(), settlement.GetTotalFan(), settlement.GetDetailText(), payload)
	return err
}

// HasSettlement 判断房间是否已有结算记录。
func (s *SettlementStore) HasSettlement(ctx context.Context, roomID string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, fmt.Errorf("nil settlement store")
	}
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(1) FROM settlements WHERE room_id = $1`, roomID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// GetLatestSettlement 读取房间最近一次结算详情，供断线重连 fallback。
func (s *SettlementStore) GetLatestSettlement(ctx context.Context, roomID string) (*clientv1.SettlementNotify, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("nil settlement store")
	}
	var payload []byte
	err := s.pool.QueryRow(ctx, `
		SELECT payload
		FROM settlements
		WHERE room_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, roomID).Scan(&payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSettlementNotFound
		}
		return nil, err
	}
	var settlement clientv1.SettlementNotify
	if err := proto.Unmarshal(payload, &settlement); err != nil {
		return nil, fmt.Errorf("unmarshal settlement payload: %w", err)
	}
	return &settlement, nil
}
