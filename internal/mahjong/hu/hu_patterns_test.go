package hu

import "testing"

func TestManyStandardWins(t *testing.T) {
	// Case A：四刻子 + 一对（筒子清一色对对胡形）
	var a Counts
	for idx := 9; idx <= 12; idx++ {
		a[idx] = 3
	}
	a[13] = 2
	if a.Total() != 14 || !IsWinning(a) {
		t.Fatalf("case A lose counts=%v", a)
	}
	// Case B：万子三面顺 + 筒子刻子 + 筒子对（与既有标准形等价）
	var b Counts
	for i := 0; i < 9; i++ {
		b[i] = 1
	}
	b[9] = 3
	b[10] = 2
	if b.Total() != 14 || !IsWinning(b) {
		t.Fatalf("case B lose counts=%v", b)
	}
}
