// Package mfcc computes Mel-frequency cepstral coefficients.
// This is a port of aeneas/cmfcc/cmfcc_func.c which uses an SPTK-derived FFT.
package mfcc

import "math"

// precomputeSinTable builds the sin look-up table used by the SPTK FFT.
// Matches _precompute_sin_table in cmfcc_func.c exactly.
func precomputeSinTable(m int) []float64 {
	arg := math.Pi / float64(m) * 2
	size := m - m/4 + 1
	table := make([]float64, size)
	table[0] = 0
	for k := 1; k < size; k++ {
		table[k] = math.Sin(arg * float64(k))
	}
	table[m/2] = 0
	return table
}

// fftInPlace is a direct port of the SPTK fft() function in cmfcc_func.c.
// x and y hold the real and imaginary parts respectively; m must be a power of 2.
// sinTable must be precomputed via precomputeSinTable(m).
func fftInPlace(x, y []float64, m int, sinTable []float64) {
	lf := 1
	lmx := m

	for {
		lix := lmx
		lmx /= 2
		if lmx <= 1 {
			break
		}
		sinpIdx := 0
		cospIdx := m / 4
		for j := 0; j < lmx; j++ {
			xpIdx := j
			ypIdx := j
			for li := lix; li <= m; li += lix {
				t1 := x[xpIdx] - x[xpIdx+lmx]
				t2 := y[ypIdx] - y[ypIdx+lmx]
				x[xpIdx] += x[xpIdx+lmx]
				y[ypIdx] += y[ypIdx+lmx]
				x[xpIdx+lmx] = sinTable[cospIdx]*t1 + sinTable[sinpIdx]*t2
				y[ypIdx+lmx] = sinTable[cospIdx]*t2 - sinTable[sinpIdx]*t1
				xpIdx += lix
				ypIdx += lix
			}
			sinpIdx += lf
			cospIdx += lf
		}
		lf += lf
	}

	xpIdx := 0
	ypIdx := 0
	for li := m / 2; li > 0; li-- {
		t1 := x[xpIdx] - x[xpIdx+1]
		t2 := y[ypIdx] - y[ypIdx+1]
		x[xpIdx] += x[xpIdx+1]
		y[ypIdx] += y[ypIdx+1]
		x[xpIdx+1] = t1
		y[ypIdx+1] = t2
		xpIdx += 2
		ypIdx += 2
	}

	j := 0
	xpIdx = 0
	ypIdx = 0
	mv2 := m / 2
	mm1 := m - 1
	for lmx := 0; lmx < mm1; lmx++ {
		li := lmx - j
		if li < 0 {
			t1 := x[xpIdx]
			t2 := y[ypIdx]
			x[xpIdx] = x[xpIdx+li]
			y[ypIdx] = y[ypIdx+li]
			x[xpIdx+li] = t1
			y[ypIdx+li] = t2
		}
		li = mv2
		for li <= j {
			j -= li
			li /= 2
		}
		j += li
		xpIdx = j
		ypIdx = j
	}
}

// rfftInPlace is a direct port of the SPTK rfft() function in cmfcc_func.c.
// x is the real-valued input/output; y is a scratch buffer (len >= m+m/2+1).
// sinTableFull = precomputeSinTable(m), sinTableHalf = precomputeSinTable(m/2).
// After return, x[0..m-1] hold the real part and y[0..m-1] hold the imag part
// of the one-sided complex spectrum (positive frequencies only).
func rfftInPlace(x, y []float64, m int, sinTableFull, sinTableHalf []float64) {
	mv2 := m / 2

	// Pack alternating even/odd samples into x and y as a complex sequence.
	xpIdx := 0
	xqIdx := 0
	ypIdx := 0
	for i := mv2; i > 0; i-- {
		x[xpIdx] = x[xqIdx]
		xpIdx++
		xqIdx++
		y[ypIdx] = x[xqIdx]
		ypIdx++
		xqIdx++
	}

	fftInPlace(x, y, mv2, sinTableHalf)

	sinpIdx := 0
	cospIdx := m / 4
	xpIdx = 0
	ypIdx = 0
	xqIdx = m
	yqIdx := m

	x[mv2] = x[0] - y[0]
	x[0] = x[0] + y[0]
	y[mv2] = 0
	y[0] = 0

	for i, j := mv2, mv2-2; i > 1; i, j = i-1, j-2 {
		xpIdx++
		ypIdx++
		sinpIdx++
		cospIdx++

		yt := y[ypIdx] + y[ypIdx+j]
		xt := x[xpIdx] - x[xpIdx+j]
		xqIdx--
		yqIdx--
		x[xqIdx] = (x[xpIdx] + x[xpIdx+j] + sinTableFull[cospIdx]*yt - sinTableFull[sinpIdx]*xt) * 0.5
		y[yqIdx] = (y[ypIdx+j] - y[ypIdx] + sinTableFull[sinpIdx]*yt + sinTableFull[cospIdx]*xt) * 0.5
	}

	xpIdx = 1
	ypIdx = 1
	xqIdx = m
	yqIdx = m

	for i := mv2 - 1; i > 0; i-- {
		xqIdx--
		yqIdx--
		x[xpIdx] = x[xqIdx]
		y[ypIdx] = -y[yqIdx]
		xpIdx++
		ypIdx++
	}
}

