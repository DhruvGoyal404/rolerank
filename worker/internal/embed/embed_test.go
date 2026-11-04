package embed

import (
	"math"
	"slices"
	"testing"
)

func TestPoolTakesCLSAndNormalises(t *testing.T) {
	// batch 2, seq 2, dims 2: rows start with [3,4] and [0,2]
	got := pool([]float32{3, 4, 9, 9, 0, 2, 9, 9}, 2, 2, 2)
	if math.Abs(float64(got[0][0])-0.6) > 1e-6 || math.Abs(float64(got[0][1])-0.8) > 1e-6 {
		t.Fatalf("row 0 %v", got[0])
	}
	if got[1][0] != 0 || got[1][1] != 1 {
		t.Fatalf("row 1 %v", got[1])
	}
}

func TestBatchInputsPadsToTheLongest(t *testing.T) {
	ids, mask, types, seq := batchInputs([][]int64{{101, 7, 102}, {101, 102}}, 0)
	if seq != 3 || !slices.Equal(ids, []int64{101, 7, 102, 101, 102, 0}) || !slices.Equal(mask, []int64{1, 1, 1, 1, 1, 0}) ||
		len(types) != 6 {
		t.Fatalf("ids %v mask %v seq %d", ids, mask, seq)
	}
}

func TestLiteral(t *testing.T) {
	if got := Literal([]float32{0.5, -1, 0.25}); got != "[0.5,-1,0.25]" {
		t.Fatal(got)
	}
}
