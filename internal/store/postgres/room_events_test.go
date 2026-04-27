package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/store/postgres"
)

func TestNewRoomEventStoreNilPool(t *testing.T) {
	t.Parallel()
	require.Nil(t, postgres.NewRoomEventStore(nil))
}

func TestRoomEventStoreAppendEventValidation(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	s := postgres.NewRoomEventStore(mock)
	_, _, err = s.AppendEvent(context.Background(), "", "kind", []byte("x"))
	require.Error(t, err)
	_, _, err = s.AppendEvent(context.Background(), "r", "", []byte("x"))
	require.Error(t, err)
}

func TestRoomEventStoreAppendEventSuccess(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBeginTx(pgx.TxOptions{})
	mock.ExpectQuery("SELECT seq FROM room_events").
		WithArgs("room-a").
		WillReturnRows(pgxmock.NewRows([]string{"seq"}).AddRow(int64(0)))
	mock.ExpectExec("INSERT INTO room_events").
		WithArgs("room-a", int64(1), "deal", []byte("{}")).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	s := postgres.NewRoomEventStore(mock)
	seq, cur, err := s.AppendEvent(context.Background(), "room-a", "deal", []byte("{}"))
	require.NoError(t, err)
	require.Equal(t, int64(1), seq)
	require.Equal(t, "room-a:1", cur)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoomEventStoreAppendEventInsertFails(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBeginTx(pgx.TxOptions{})
	mock.ExpectQuery("SELECT seq FROM room_events").
		WithArgs("room-a").
		WillReturnRows(pgxmock.NewRows([]string{"seq"}).AddRow(int64(0)))
	mock.ExpectExec("INSERT INTO room_events").
		WithArgs("room-a", int64(1), "deal", []byte("{}")).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	s := postgres.NewRoomEventStore(mock)
	_, _, err = s.AppendEvent(context.Background(), "room-a", "deal", []byte("{}"))
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoomEventStoreAppendEventsSuccess(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBeginTx(pgx.TxOptions{})
	mock.ExpectQuery("SELECT seq FROM room_events").
		WithArgs("room-b").
		WillReturnRows(pgxmock.NewRows([]string{"seq"}).AddRow(int64(4)))
	mock.ExpectExec("INSERT INTO room_events").
		WithArgs("room-b", int64(5), "draw", []byte("a")).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO room_events").
		WithArgs("room-b", int64(6), "action", []byte("b")).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	s := postgres.NewRoomEventStore(mock)
	rows, err := s.AppendEvents(context.Background(), "room-b", []postgres.RoomEventRow{
		{Kind: "draw", Payload: []byte("a")},
		{Kind: "action", Payload: []byte("b")},
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(5), rows[0].Seq)
	require.Equal(t, int64(6), rows[1].Seq)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoomEventStoreAppendEventsRollbackOnFailure(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBeginTx(pgx.TxOptions{})
	mock.ExpectQuery("SELECT seq FROM room_events").
		WithArgs("room-c").
		WillReturnRows(pgxmock.NewRows([]string{"seq"}).AddRow(int64(1)))
	mock.ExpectExec("INSERT INTO room_events").
		WithArgs("room-c", int64(2), "draw", []byte("a")).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO room_events").
		WithArgs("room-c", int64(3), "action", []byte("b")).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	s := postgres.NewRoomEventStore(mock)
	_, err = s.AppendEvents(context.Background(), "room-c", []postgres.RoomEventRow{
		{Kind: "draw", Payload: []byte("a")},
		{Kind: "action", Payload: []byte("b")},
	})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoomEventStoreListEventsAfter(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	rows := pgxmock.NewRows([]string{"room_id", "seq", "kind", "payload"}).
		AddRow("r1", int64(1), "k1", []byte("a")).
		AddRow("r1", int64(2), "k2", []byte("b"))
	mock.ExpectQuery("SELECT room_id, seq, kind, payload FROM room_events").
		WithArgs("r1", int64(0)).
		WillReturnRows(rows)

	s := postgres.NewRoomEventStore(mock)
	out, err := s.ListEventsAfter(context.Background(), "r1", 0)
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, int64(2), out[1].Seq)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoomEventStoreMaxSeq(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	mock.ExpectQuery("SELECT COALESCE").
		WithArgs("r9").
		WillReturnRows(pgxmock.NewRows([]string{"m"}).AddRow(int64(7)))

	s := postgres.NewRoomEventStore(mock)
	m, err := s.MaxSeq(context.Background(), "r9")
	require.NoError(t, err)
	require.Equal(t, int64(7), m)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNewSettlementStoreNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, postgres.NewSettlementStore(nil))
}

func TestSettlementStoreAppendAndHas(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	settlement := &clientv1.SettlementNotify{
		RoomId:        "r1",
		WinnerUserIds: []string{"u1"},
		TotalFan:      8,
		DetailText:    "detail",
	}
	payload, err := proto.Marshal(settlement)
	require.NoError(t, err)
	mock.ExpectExec("INSERT INTO settlements").
		WithArgs("r1", []string{"u1"}, int32(8), "detail", payload).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectQuery("SELECT COUNT").
		WithArgs("r1").
		WillReturnRows(pgxmock.NewRows([]string{"c"}).AddRow(1))

	s := postgres.NewSettlementStore(mock)
	require.NoError(t, s.AppendSettlement(context.Background(), settlement))
	ok, err := s.HasSettlement(context.Background(), "r1")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettlementStoreNilReceiver(t *testing.T) {
	t.Parallel()
	var s *postgres.SettlementStore
	require.Error(t, s.AppendSettlement(context.Background(), &clientv1.SettlementNotify{RoomId: "r"}))
	_, err := s.HasSettlement(context.Background(), "r")
	require.Error(t, err)
}

func TestSettlementStoreGetLatestSettlement(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	want := &clientv1.SettlementNotify{
		RoomId:        "r2",
		WinnerUserIds: []string{"u2"},
		TotalFan:      4,
		DetailText:    "latest",
	}
	payload, err := proto.Marshal(want)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT payload").
		WithArgs("r2").
		WillReturnRows(pgxmock.NewRows([]string{"payload"}).AddRow(payload))

	s := postgres.NewSettlementStore(mock)
	got, err := s.GetLatestSettlement(context.Background(), "r2")
	require.NoError(t, err)
	require.True(t, proto.Equal(want, got))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGameSummaryStoreCRUD(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	endedAt := time.Now().UTC().Round(time.Second)
	createdAt := endedAt.Add(-time.Minute)
	mock.ExpectExec("INSERT INTO game_summaries").
		WithArgs("room-g", "sichuan_xzdd", []string{"u1", "u2"}).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE game_summaries").
		WithArgs("room-g", endedAt).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT room_id, rule_id, player_ids, created_at, ended_at").
		WithArgs("room-g").
		WillReturnRows(pgxmock.NewRows([]string{"room_id", "rule_id", "player_ids", "created_at", "ended_at"}).
			AddRow("room-g", "sichuan_xzdd", []string{"u1", "u2"}, createdAt, &endedAt))

	s := postgres.NewGameSummaryStore(mock)
	require.NoError(t, s.CreateGameSummary(context.Background(), "room-g", "sichuan_xzdd", []string{"u1", "u2"}))
	require.NoError(t, s.EndGameSummary(context.Background(), "room-g", endedAt))
	got, err := s.GetGameSummary(context.Background(), "room-g")
	require.NoError(t, err)
	require.Equal(t, "room-g", got.RoomID)
	require.NotNil(t, got.EndedAt)
	require.Equal(t, endedAt, got.EndedAt.UTC())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoomEventStoreNilReceiver(t *testing.T) {
	t.Parallel()
	var s *postgres.RoomEventStore
	_, _, err := s.AppendEvent(context.Background(), "r", "k", nil)
	require.Error(t, err)
	_, err = s.AppendEvents(context.Background(), "r", []postgres.RoomEventRow{{Kind: "k"}})
	require.Error(t, err)
	_, err = s.ListEventsAfter(context.Background(), "r", 0)
	require.Error(t, err)
	_, err = s.MaxSeq(context.Background(), "r")
	require.Error(t, err)
}
