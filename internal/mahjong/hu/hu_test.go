// 本文件为和牌判定单元测试。
// 覆盖标准形（四面子一对将）、七对子、总张数不为 14 的拒绝路径，以及七对中含四张相同作两对的川麻常见处理。
package hu

import "testing"

func TestSevenPairsAllTwos(t *testing.T) {
	var c Counts
	for _, idx := range []int{0, 2, 4, 6, 8, 10, 12} {
		c[idx] = 2
	}
	if c.Total() != 14 {
		t.Fatalf("total=%d", c.Total())
	}
	if !IsWinning(c) {
		t.Fatal("expected seven pairs")
	}
}

func TestSevenPairsWithQuad(t *testing.T) {
	var c Counts
	c[0] = 4
	for _, idx := range []int{2, 4, 6, 8, 10} {
		c[idx] = 2
	}
	if c.Total() != 14 {
		t.Fatalf("total=%d", c.Total())
	}
	if !IsWinning(c) {
		t.Fatal("expected seven pairs with quad as two pairs")
	}
}

func TestStandardFormSequenceAndTriple(t *testing.T) {
	// 123m 456m 789m + 111p + 22p
	var c Counts
	for i := 0; i < 9; i++ {
		c[i] = 1
	}
	c[9] = 3
	c[10] = 2
	if c.Total() != 14 {
		t.Fatalf("total=%d", c.Total())
	}
	if !IsWinning(c) {
		t.Fatal("expected standard win")
	}
}

func TestNonWinningWrongTotal(t *testing.T) {
	var c Counts
	c[0] = 2
	if IsWinning(c) {
		t.Fatal("expected false")
	}
}
