package rank

import (
	"math"
	"testing"
)

func TestMeasure(t *testing.T) {
	grades := map[int64]int{1: 3, 2: 2, 3: 0, 4: 3}
	yes := func(int64) bool { return true }
	no := func(int64) bool { return false }
	// Ideal order is 1 and 4 (grade 3), then 2. This list puts a grade 0 first, has an unjudged
	// 99, and misses job 4.
	m := Measure("entry", []int64{3, 1, 99, 2}, grades, func(id int64) bool { return id != 99 },
		func(id int64) bool { return id == 3 })
	ideal := 7 + 7/math.Log2(3) + 3/math.Log2(4)
	want := (0 + 7/math.Log2(3) + 3/math.Log2(4)) / ideal // 99 is skipped, not counted as irrelevant
	if math.Abs(m.NDCG10-want) > 1e-9 {
		t.Fatalf("nDCG %v, want %v", m.NDCG10, want)
	}
	if m.RR != 0.5 || m.Recall != 2.0/3 || m.GeoViolate != 1 || m.LevelViolate != 1 || m.Judged10 != 0.75 {
		t.Fatalf("%+v", m)
	}
	if n := Measure("entry", nil, map[int64]int{1: 1}, yes, no); n.RR != -1 || n.Recall != -1 || n.NDCG10 != 0 {
		t.Fatalf("%+v", n)
	}
	if s := Summarise([]Metrics{m, {RR: -1, Recall: -1, NDCG10: 1}}, 8, 4); s.MRR != 0.5 || s.Recall != 2.0/3 ||
		s.GeoViolate != 1.0/8 || s.LevelViolation != 0.25 {
		t.Fatalf("%+v", s)
	}
}
