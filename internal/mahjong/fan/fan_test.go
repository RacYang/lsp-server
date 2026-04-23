package fan

import "testing"

func TestBreakdownAdd(t *testing.T) {
	var b Breakdown
	b.Add(KindPingHu, 1, "平胡")
	b.Add(KindQingYiSe, 4, "清一色")
	if b.Total != 5 || len(b.Items) != 2 {
		t.Fatalf("got %+v", b)
	}
}

func TestBreakdownSkipNonPositive(t *testing.T) {
	var b Breakdown
	b.Add(KindPingHu, 0, "无效")
	if b.Total != 0 || len(b.Items) != 0 {
		t.Fatalf("got %+v", b)
	}
}
