package room

import (
	"testing"

	"racoo.cn/lsp/internal/mahjong/sichuanxzdd"
)

func TestMigratePersist_NilSafe(t *testing.T) {
	migratePersistToCurrent(nil)
}

func TestMigratePersist_V0PromotesWinnerAndLedger(t *testing.T) {
	rp := &roundPersist{
		SchemaVersion:  0,
		WinnerSeat:     2,
		TotalFanBySeat: []int32{1, -2, 3, 0},
	}
	migratePersistToCurrent(rp)

	if rp.SchemaVersion != roundPersistSchemaVersion {
		t.Fatalf("schema version not advanced to %d, got %d", roundPersistSchemaVersion, rp.SchemaVersion)
	}
	if got := rp.WinnerSeats; len(got) != 1 || got[0] != 2 {
		t.Fatalf("expected winner seats [2], got %v", got)
	}
	if rp.OpeningDrawSeat != -1 {
		t.Fatalf("v3 promotion should set opening draw seat to -1, got %d", rp.OpeningDrawSeat)
	}
	if rp.DealerFirstDiscardOpen {
		t.Fatalf("v3 promotion should close dealer first discard window")
	}
	if got := rp.Ledger; len(got) != 3 {
		t.Fatalf("legacy ledger should contain 3 non-zero entries, got %v", got)
	}
}

func TestMigratePersist_V1PreservesExistingWinnerSeats(t *testing.T) {
	rp := &roundPersist{
		SchemaVersion: 1,
		WinnerSeat:    1,
		WinnerSeats:   []int{0, 3},
		Ledger:        []sichuanxzdd.ScoreEntry{{Reason: "kept"}},
	}
	migratePersistToCurrent(rp)

	if len(rp.WinnerSeats) != 2 || rp.WinnerSeats[0] != 0 || rp.WinnerSeats[1] != 3 {
		t.Fatalf("v1 with WinnerSeats should be preserved, got %v", rp.WinnerSeats)
	}
	if len(rp.Ledger) != 1 || rp.Ledger[0].Reason != "kept" {
		t.Fatalf("existing ledger should not be overwritten, got %v", rp.Ledger)
	}
}

func TestMigratePersist_V2InitializesDealerWindow(t *testing.T) {
	rp := &roundPersist{
		SchemaVersion:          2,
		WinnerSeats:            []int{0},
		DealerSeat:             3,
		OpeningDrawSeat:        2,
		DealerFirstDiscardOpen: true,
	}
	migratePersistToCurrent(rp)

	if rp.SchemaVersion != roundPersistSchemaVersion {
		t.Fatalf("schema version not advanced, got %d", rp.SchemaVersion)
	}
	if rp.DealerSeat != 0 || rp.OpeningDrawSeat != -1 || rp.DealerFirstDiscardOpen {
		t.Fatalf("v2->v3 should reset dealer window, got dealer=%d opening=%d open=%v",
			rp.DealerSeat, rp.OpeningDrawSeat, rp.DealerFirstDiscardOpen)
	}
}

func TestMigratePersist_V3IsIdempotent(t *testing.T) {
	rp := &roundPersist{
		SchemaVersion:          3,
		WinnerSeats:            []int{1},
		DealerSeat:             2,
		OpeningDrawSeat:        2,
		DealerFirstDiscardOpen: true,
	}
	migratePersistToCurrent(rp)

	if rp.SchemaVersion != 3 {
		t.Fatalf("v3 should remain at 3, got %d", rp.SchemaVersion)
	}
	if rp.DealerSeat != 2 || rp.OpeningDrawSeat != 2 || !rp.DealerFirstDiscardOpen {
		t.Fatalf("v3 fields must not be mutated, got dealer=%d opening=%d open=%v",
			rp.DealerSeat, rp.OpeningDrawSeat, rp.DealerFirstDiscardOpen)
	}
}
