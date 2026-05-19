// Package dtw implements Dynamic Time Warping for aligning MFCC
// sequences. It is a port of aeneas/cdtw/cdtw_func.c and the pure-Python
// fallbacks in aeneas/dtw.py.
//
// Both algorithms (stripe — margin-constrained — and exact) discard the
// first MFCC coefficient (row 0, "energy") before computing the cost
// matrix, matching the Python pipeline. The metric is one minus the
// cosine similarity between MFCC columns; zero-norm columns return 1.0
// (max distance) rather than NaN, an intentional divergence from
// upstream that avoids silent NaN propagation through the accumulator.
//
// API
//
//   ComputeCostMatrix(m1, m2, delta)  → (cost[n][delta], centers[n])
//   AccumulateCostMatrixInPlace(c, ctr)
//   ComputeBestPath(acc, centers)     → []PathCell
//   ComputePathStripe(m1, m2, delta)  → composition of the above
//
//   ComputeCostMatrixExact(m1, m2)
//   AccumulateCostMatrixExactInPlace(cost)
//   ComputeBestPathExact(acc)
//   ComputePathExact(m1, m2)
//
// Parallelism
//
// Cost-matrix construction auto-fans-out across rows when n is at
// least CostMatrixParallelThreshold (default 64). The accumulation and
// backtrace stages are inherently serial (data dependencies) and run
// on a single goroutine.
//
// Output is bit-identical to the serial path, since each row writes to
// its own slice of cost[i][*] independently.
package dtw
