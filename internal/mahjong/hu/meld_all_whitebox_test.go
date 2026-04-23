package hu

import "testing"

func TestMeldAllPurePongs(t *testing.T) {
	// 去掉将牌后应能被三组刻子完全吃尽
	var rest Counts
	rest[9] = 3
	rest[10] = 3
	rest[11] = 3
	if !meldAll(rest) {
		t.Fatal("expected meldAll true")
	}
}

func TestMeldAllChowLeftMiddleRight(t *testing.T) {
	// 仅顺子：123 + 456 + 789（万子）
	var rest Counts
	for i := 0; i < 9; i++ {
		rest[i] = 1
	}
	if !meldAll(rest) {
		t.Fatal("expected meldAll true for pure sequences")
	}
}

func TestMeldAllFalseWhenStuck(t *testing.T) {
	// 无法组成面子的残留
	var rest Counts
	rest[0] = 1
	rest[1] = 1
	if meldAll(rest) {
		t.Fatal("expected false")
	}
}
