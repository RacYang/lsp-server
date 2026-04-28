package room

import "racoo.cn/lsp/internal/mahjong/sichuanxzdd"

// migratePersistToCurrent 把任意旧 schema 的 roundPersist 升级到当前版本，让上层
// 反序列化与构造逻辑只与最新版本对话；每个分支只前进一个版本，避免分叉。
//
// schema 历史：
//   - v0 / v1：单赢家（WinnerSeat）+ TotalFanBySeat 总分。
//   - v2：多赢家（WinnerSeats）+ Ledger 流水（ADR-0020）。
//   - v3：补 Dealer/OpeningDraw/DealerFirstDiscard 庄家与首巡窗口（ADR-0021）。
func migratePersistToCurrent(rp *roundPersist) {
	if rp == nil {
		return
	}
	if rp.SchemaVersion <= 1 {
		migrateV01ToV2(rp)
	}
	if rp.SchemaVersion == 2 {
		migrateV2ToV3(rp)
	}
}

// migrateV01ToV2 把单赢家与按座位总分流水升级为多赢家与 score ledger 表达。
func migrateV01ToV2(rp *roundPersist) {
	if len(rp.WinnerSeats) == 0 && rp.WinnerSeat >= 0 {
		rp.WinnerSeats = []int{rp.WinnerSeat}
	}
	if len(rp.Ledger) == 0 && len(rp.TotalFanBySeat) > 0 {
		rp.Ledger = legacyLedgerFromTotals(rp.TotalFanBySeat)
	}
	rp.SchemaVersion = 2
}

// migrateV2ToV3 在缺少庄家/首巡上下文的旧快照上按"开局窗口已关闭"恢复，避免错误补发天胡或地胡。
func migrateV2ToV3(rp *roundPersist) {
	rp.DealerSeat = 0
	rp.OpeningDrawSeat = -1
	rp.DealerFirstDiscardOpen = false
	rp.SchemaVersion = 3
}

// legacyLedgerFromTotals 将 v0/v1 的座位总分翻译为单条 legacy_total 流水，保留可审计性。
func legacyLedgerFromTotals(totals []int32) []sichuanxzdd.ScoreEntry {
	out := make([]sichuanxzdd.ScoreEntry, 0, len(totals))
	for seat, total := range totals {
		if total == 0 {
			continue
		}
		from, to, amount := -1, seat, total
		if total < 0 {
			from, to, amount = seat, -1, -total
		}
		out = append(out, sichuanxzdd.ScoreEntry{
			Reason:     "legacy_total",
			FromSeat:   from,
			ToSeat:     to,
			Amount:     amount,
			Step:       0,
			WinnerSeat: -1,
		})
	}
	return out
}
