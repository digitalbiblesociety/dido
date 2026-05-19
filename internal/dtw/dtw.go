// Package dtw implements Dynamic Time Warping for aligning MFCC sequences.
// This is a port of aeneas/cdtw/cdtw_func.c and the pure-Python fallbacks in aeneas/dtw.py.
package dtw

import (
	"math"
	"runtime"
	"sync"
)

// CostMatrixParallelThreshold is the minimum row count (n) at which
// ComputeCostMatrix fans out across CPU cores. Below this, the serial
// path is faster (goroutine setup dominates).
var CostMatrixParallelThreshold = 64

// CostMatrixMaxWorkers caps the number of goroutines used. 0 = runtime.NumCPU().
var CostMatrixMaxWorkers = 0

// PathCell is one (i, j) cell on the optimal warp path.
type PathCell struct {
	I, J int
}

const inf = math.MaxFloat64

// ComputeNorm2 computes the L2 norm of each column of mfcc (shape [l][n], column-major).
func ComputeNorm2(mfcc [][]float64) []float64 {
	if len(mfcc) == 0 {
		return nil
	}
	l := len(mfcc)
	n := len(mfcc[0])
	norm2 := make([]float64, n)
	for i := 0; i < n; i++ {
		sum := 0.0
		for k := 0; k < l; k++ {
			v := mfcc[k][i]
			sum += v * v
		}
		norm2[i] = math.Sqrt(sum)
	}
	return norm2
}

// nonnegDiff returns max(0, a-b).
func nonnegDiff(a, b int) int {
	if b > a {
		return 0
	}
	return a - b
}

func threeWayMin(a, b, c float64) float64 {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

func threeWayArgmin(a, b, c float64) int {
	if a <= b && a <= c {
		return 0
	}
	if b <= c {
		return 1
	}
	return 2
}

// cosineDistance returns 1 - cos(θ) between two MFCC columns whose dot
// product is `sum` and whose L2 norms are n1 and n2. When either column is
// the zero vector (norm == 0) the cosine similarity is mathematically
// undefined; we return 1.0 (maximum dissimilarity for unit-bounded cosine
// distance), which is the standard convention in IR/NLP and matches the
// expectation that "no information available here, don't reward the path
// for routing through this cell". The Python/C implementation produces
// NaN here and silently poisons the accumulated cost — we deliberately
// diverge to avoid that failure mode.
func cosineDistance(sum, n1, n2 float64) float64 {
	if n1 == 0 || n2 == 0 {
		return 1.0
	}
	return 1.0 - sum/(n1*n2)
}

// ComputeCostMatrix computes the stripe cost matrix for MFCC sequence alignment.
// mfcc1 is [l][n], mfcc2 is [l][m] (column-major, rows = MFCC dimensions).
// The first MFCC component (row 0) is discarded to match the Python/C behaviour.
// Returns cost matrix [n][delta] and centers [n] as per cdtw_func.c.
// Returns (nil, nil) if either input has fewer than 2 coefficient rows
// (after discarding row 0 there's nothing to align) or zero frames.
func ComputeCostMatrix(mfcc1, mfcc2 [][]float64, delta int) ([][]float64, []int) {
	if len(mfcc1) < 2 || len(mfcc2) < 2 {
		return nil, nil
	}
	// Discard first MFCC component (row 0).
	m1 := mfcc1[1:]
	m2 := mfcc2[1:]

	l := len(m1) // MFCC dimensions after discarding first
	if len(m1[0]) == 0 || len(m2[0]) == 0 {
		return nil, nil
	}
	n := len(m1[0])
	m := len(m2[0])

	if delta > m {
		delta = m
	}

	norm2_1 := ComputeNorm2(m1)
	norm2_2 := ComputeNorm2(m2)

	costMatrix := make([][]float64, n)
	for i := range costMatrix {
		costMatrix[i] = make([]float64, delta)
	}
	centers := make([]int, n)

	fillRow := func(i int) {
		centerJ := m * i / n
		rangeStart := nonnegDiff(centerJ, delta/2)
		rangeEnd := rangeStart + delta
		if rangeEnd > m {
			rangeEnd = m
			rangeStart = rangeEnd - delta
		}
		centers[i] = rangeStart
		for j := rangeStart; j < rangeEnd; j++ {
			sum := 0.0
			for k := 0; k < l; k++ {
				sum += m1[k][i] * m2[k][j]
			}
			costMatrix[i][j-rangeStart] = cosineDistance(sum, norm2_1[i], norm2_2[j])
		}
	}

	// Each row is computed independently — fan out when the row count
	// makes the goroutine overhead worth it.
	workers := CostMatrixMaxWorkers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if n < CostMatrixParallelThreshold || workers <= 1 {
		for i := 0; i < n; i++ {
			fillRow(i)
		}
		return costMatrix, centers
	}
	if workers > n {
		workers = n
	}
	chunk := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		start := w * chunk
		if start >= n {
			break
		}
		end := start + chunk
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for i := start; i < end; i++ {
				fillRow(i)
			}
		}(start, end)
	}
	wg.Wait()
	return costMatrix, centers
}

// AccumulateCostMatrixInPlace overwrites costMatrix with the accumulated cost.
// Matches _compute_accumulated_cost_matrix_in_place in cdtw_func.c.
func AccumulateCostMatrixInPlace(costMatrix [][]float64, centers []int) {
	n := len(costMatrix)
	if n == 0 {
		return
	}
	delta := len(costMatrix[0])

	// First row: running sum.
	currentRow := make([]float64, delta)
	copy(currentRow, costMatrix[0])
	for j := 1; j < delta; j++ {
		costMatrix[0][j] = currentRow[j] + costMatrix[0][j-1]
	}

	for i := 1; i < n; i++ {
		copy(currentRow, costMatrix[i])
		offset := centers[i] - centers[i-1]
		for j := 0; j < delta; j++ {
			cost0 := inf
			if j+offset < delta {
				cost0 = costMatrix[i-1][j+offset]
			}
			cost1 := inf
			if j > 0 {
				cost1 = costMatrix[i][j-1]
			}
			cost2 := inf
			if (j+offset-1 < delta) && (j+offset >= 1) {
				cost2 = costMatrix[i-1][j+offset-1]
			}
			costMatrix[i][j] = currentRow[j] + threeWayMin(cost0, cost1, cost2)
		}
	}
}

// ComputeBestPath performs the backtrace on the accumulated cost matrix.
// Returns the optimal path as a slice of PathCells from (0,0) to (n-1, m-1).
// Matches _compute_best_path in cdtw_func.c.
func ComputeBestPath(accMatrix [][]float64, centers []int) []PathCell {
	n := len(accMatrix)
	if n == 0 {
		return nil
	}
	delta := len(accMatrix[0])

	maxPathLen := n + centers[n-1] + delta
	path := make([]PathCell, 0, maxPathLen)

	i := n - 1
	j := centers[i] + delta - 1
	path = append(path, PathCell{i, j})

	for i > 0 || j > 0 {
		if i == 0 {
			j--
			path = append(path, PathCell{0, j})
		} else if j == 0 {
			i--
			path = append(path, PathCell{i, 0})
		} else {
			offset := centers[i] - centers[i-1]
			rj := j - centers[i]

			cost0 := inf
			if rj+offset < delta {
				cost0 = accMatrix[i-1][rj+offset]
			}
			cost1 := inf
			if rj > 0 {
				cost1 = accMatrix[i][rj-1]
			}
			cost2 := inf
			if (rj > 0) && (rj+offset-1 < delta) && (rj+offset >= 1) {
				cost2 = accMatrix[i-1][rj+offset-1]
			}

			switch threeWayArgmin(cost0, cost1, cost2) {
			case 0: // up
				i--
				path = append(path, PathCell{i, j})
			case 1: // left
				j--
				path = append(path, PathCell{i, j})
			case 2: // diagonal
				i--
				j--
				path = append(path, PathCell{i, j})
			}
		}
	}

	// Reverse path.
	for a, b := 0, len(path)-1; a < b; a, b = a+1, b-1 {
		path[a], path[b] = path[b], path[a]
	}
	return path
}

// ComputePathStripe is the full stripe DTW: cost matrix → accumulation → backtrace.
func ComputePathStripe(mfcc1, mfcc2 [][]float64, delta int) []PathCell {
	cost, centers := ComputeCostMatrix(mfcc1, mfcc2, delta)
	if cost == nil {
		return nil
	}
	AccumulateCostMatrixInPlace(cost, centers)
	return ComputeBestPath(cost, centers)
}

// ComputeCostMatrixExact computes the full (n×m) cost matrix (exact DTW, no stripe).
// mfcc1 is [l][n], mfcc2 is [l][m]. Discards row 0. Returns nil if either
// input has fewer than 2 coefficient rows or zero frames.
func ComputeCostMatrixExact(mfcc1, mfcc2 [][]float64) [][]float64 {
	if len(mfcc1) < 2 || len(mfcc2) < 2 {
		return nil
	}
	m1 := mfcc1[1:]
	m2 := mfcc2[1:]

	l := len(m1)
	if len(m1[0]) == 0 || len(m2[0]) == 0 {
		return nil
	}
	n := len(m1[0])
	m := len(m2[0])

	norm2_1 := ComputeNorm2(m1)
	norm2_2 := ComputeNorm2(m2)

	cost := make([][]float64, n)
	for i := range cost {
		cost[i] = make([]float64, m)
	}

	fillRow := func(i int) {
		for j := 0; j < m; j++ {
			sum := 0.0
			for k := 0; k < l; k++ {
				sum += m1[k][i] * m2[k][j]
			}
			cost[i][j] = cosineDistance(sum, norm2_1[i], norm2_2[j])
		}
	}

	workers := CostMatrixMaxWorkers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if n < CostMatrixParallelThreshold || workers <= 1 {
		for i := 0; i < n; i++ {
			fillRow(i)
		}
		return cost
	}
	if workers > n {
		workers = n
	}
	chunk := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		start := w * chunk
		if start >= n {
			break
		}
		end := start + chunk
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for i := start; i < end; i++ {
				fillRow(i)
			}
		}(start, end)
	}
	wg.Wait()
	return cost
}

// AccumulateCostMatrixExactInPlace accumulates the exact (full) cost matrix.
func AccumulateCostMatrixExactInPlace(cost [][]float64) {
	n := len(cost)
	if n == 0 {
		return
	}
	m := len(cost[0])

	// First row.
	currentRow := make([]float64, m)
	copy(currentRow, cost[0])
	for j := 1; j < m; j++ {
		cost[0][j] = currentRow[j] + cost[0][j-1]
	}

	for i := 1; i < n; i++ {
		copy(currentRow, cost[i])
		cost[i][0] = cost[i-1][0] + currentRow[0]
		for j := 1; j < m; j++ {
			cost[i][j] = currentRow[j] + threeWayMin(
				cost[i-1][j],
				cost[i][j-1],
				cost[i-1][j-1],
			)
		}
	}
}

// ComputeBestPathExact backtracks through the full accumulated cost matrix.
func ComputeBestPathExact(acc [][]float64) []PathCell {
	n := len(acc)
	if n == 0 {
		return nil
	}
	m := len(acc[0])

	path := make([]PathCell, 0, n+m)
	i := n - 1
	j := m - 1
	path = append(path, PathCell{i, j})

	for i > 0 || j > 0 {
		if i == 0 {
			j--
			path = append(path, PathCell{0, j})
		} else if j == 0 {
			i--
			path = append(path, PathCell{i, 0})
		} else {
			switch threeWayArgmin(acc[i-1][j], acc[i][j-1], acc[i-1][j-1]) {
			case 0:
				i--
				path = append(path, PathCell{i, j})
			case 1:
				j--
				path = append(path, PathCell{i, j})
			case 2:
				i--
				j--
				path = append(path, PathCell{i, j})
			}
		}
	}

	for a, b := 0, len(path)-1; a < b; a, b = a+1, b-1 {
		path[a], path[b] = path[b], path[a]
	}
	return path
}

// ComputePathExact is the full exact DTW: cost matrix → accumulation → backtrace.
func ComputePathExact(mfcc1, mfcc2 [][]float64) []PathCell {
	cost := ComputeCostMatrixExact(mfcc1, mfcc2)
	if cost == nil {
		return nil
	}
	AccumulateCostMatrixExactInPlace(cost)
	return ComputeBestPathExact(cost)
}
