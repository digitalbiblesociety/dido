package dtw

import (
	"math"
	"testing"
)

// syntheticMFCC returns a random-ish MFCC matrix [l][n] seeded with deterministic values.
func syntheticMFCC(l, n int, seed float64) [][]float64 {
	m := make([][]float64, l)
	for i := range m {
		m[i] = make([]float64, n)
		for j := range m[i] {
			m[i][j] = math.Sin(seed*float64(i+1)*float64(j+1)) * 5.0
		}
	}
	return m
}

func TestComputeNorm2(t *testing.T) {
	// Single-dimension case: norm2[i] = |mfcc[0][i]|
	mfcc := [][]float64{{3.0, 4.0, 0.0}}
	got := ComputeNorm2(mfcc)
	want := []float64{3.0, 4.0, 0.0}
	for i, g := range got {
		if math.Abs(g-want[i]) > 1e-12 {
			t.Errorf("norm2[%d]: got %f, want %f", i, g, want[i])
		}
	}

	// Two-dimension: norm2[0] = sqrt(3^2+4^2) = 5
	mfcc2 := [][]float64{{3.0}, {4.0}}
	got2 := ComputeNorm2(mfcc2)
	if math.Abs(got2[0]-5.0) > 1e-12 {
		t.Errorf("norm2[0]: got %f, want 5", got2[0])
	}
}

func TestComputePathStripeShape(t *testing.T) {
	l, n, m := 13, 100, 80
	mfcc1 := syntheticMFCC(l, n, 1.1)
	mfcc2 := syntheticMFCC(l, m, 2.3)
	delta := 40

	path := ComputePathStripe(mfcc1, mfcc2, delta)
	if len(path) == 0 {
		t.Fatal("path is empty")
	}
	// Path must start at (0, something) and end at (n-1, something).
	if path[0].I != 0 {
		t.Errorf("path starts at I=%d, want 0", path[0].I)
	}
	if path[len(path)-1].I != n-1 {
		t.Errorf("path ends at I=%d, want %d", path[len(path)-1].I, n-1)
	}
	// J values must be monotonically non-decreasing along the path.
	for k := 1; k < len(path); k++ {
		if path[k].J < path[k-1].J {
			t.Errorf("path[%d].J=%d < path[%d].J=%d (not monotone)", k, path[k].J, k-1, path[k-1].J)
			break
		}
	}
}

func TestComputePathExactShape(t *testing.T) {
	l, n, m := 13, 30, 25
	mfcc1 := syntheticMFCC(l, n, 0.7)
	mfcc2 := syntheticMFCC(l, m, 1.3)

	path := ComputePathExact(mfcc1, mfcc2)
	if len(path) == 0 {
		t.Fatal("path is empty")
	}
	if path[0].I != 0 || path[0].J != 0 {
		t.Errorf("path should start at (0,0), got (%d,%d)", path[0].I, path[0].J)
	}
	if path[len(path)-1].I != n-1 || path[len(path)-1].J != m-1 {
		t.Errorf("path should end at (%d,%d), got (%d,%d)", n-1, m-1,
			path[len(path)-1].I, path[len(path)-1].J)
	}
}

func TestIdenticalSequencesLowCost(t *testing.T) {
	// Aligning a sequence with itself via exact DTW should produce a diagonal path
	// and very low total cost.
	l, n := 13, 20
	mfcc := syntheticMFCC(l, n, 1.5)
	// Clone it.
	mfcc2 := make([][]float64, l)
	for i := range mfcc {
		mfcc2[i] = append([]float64(nil), mfcc[i]...)
	}

	path := ComputePathExact(mfcc, mfcc2)
	// Cost matrix of identical sequences: all diagonal costs ≈ 0 (cosine distance → 0).
	// The path should follow the diagonal.
	for _, p := range path {
		if p.I != p.J {
			// Allow some off-diagonal steps near boundaries.
			if math.Abs(float64(p.I-p.J)) > 1 {
				t.Errorf("identical-sequence path deviates: (%d,%d)", p.I, p.J)
				break
			}
		}
	}
}

// TestCosineDistanceZeroNorm verifies that a zero-norm MFCC column doesn't
// produce NaN or Inf in the cost matrix (it returns 1.0 — maximum cosine
// distance). This protects against silent NaN propagation through the
// accumulated cost matrix when the audio contains true silence on a frame.
func TestCosineDistanceZeroNorm(t *testing.T) {
	if got := cosineDistance(0, 0, 5); got != 1.0 {
		t.Errorf("cosineDistance(0,0,5) = %v, want 1.0", got)
	}
	if got := cosineDistance(0, 5, 0); got != 1.0 {
		t.Errorf("cosineDistance(0,5,0) = %v, want 1.0", got)
	}
	if got := cosineDistance(0, 0, 0); got != 1.0 {
		t.Errorf("cosineDistance(0,0,0) = %v, want 1.0", got)
	}
	// Sanity: positive norms still produce the normal value (parallel vectors
	// → cosine similarity 1 → distance 0).
	if got := cosineDistance(12, 3, 4); math.Abs(got) > 1e-12 {
		t.Errorf("cosineDistance(12,3,4) = %v, want 0", got)
	}
}

// TestCostMatrixZeroColumn checks that ComputeCostMatrix handles a fully-zero
// MFCC column (e.g. degenerate synthetic input or a silence frame after the
// energy row is discarded) without producing NaN.
func TestCostMatrixZeroColumn(t *testing.T) {
	l, m := 3, 4
	m1 := make([][]float64, l)
	m2 := make([][]float64, l)
	for i := range m1 {
		m1[i] = []float64{1, 2, 3, 4, 5}
		m2[i] = []float64{1, 0, 1, 1} // column 1 is all-zero across all rows
	}
	// We need an extra "row 0" because ComputeCostMatrix discards it.
	m1 = append([][]float64{{0, 0, 0, 0, 0}}, m1...)
	m2 = append([][]float64{{0, 0, 0, 0}}, m2...)

	cm, _ := ComputeCostMatrix(m1, m2, m)
	if cm == nil {
		t.Fatal("nil cost matrix")
	}
	for i, row := range cm {
		for j, v := range row {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Errorf("cost[%d][%d] = %v (NaN/Inf with zero norm column)", i, j, v)
			}
		}
	}
}

func TestThreeWayMin(t *testing.T) {
	if v := threeWayMin(3, 1, 2); v != 1 { t.Errorf("got %f", v) }
	if v := threeWayMin(1, 2, 3); v != 1 { t.Errorf("got %f", v) }
	if v := threeWayMin(3, 2, 1); v != 1 { t.Errorf("got %f", v) }
	if v := threeWayMin(1, 1, 1); v != 1 { t.Errorf("got %f", v) }
}

func TestThreeWayArgmin(t *testing.T) {
	if v := threeWayArgmin(3, 1, 2); v != 1 { t.Errorf("got %d", v) }
	if v := threeWayArgmin(1, 2, 3); v != 0 { t.Errorf("got %d", v) }
	if v := threeWayArgmin(3, 2, 1); v != 2 { t.Errorf("got %d", v) }
}
