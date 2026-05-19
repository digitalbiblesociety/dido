// Package align maps synthesized fragment anchor times to real-wave frame
// boundaries using the DTW path produced by the dtw package.
// Port of aeneas/dtw.py DTWAligner.compute_boundaries.
package align

import (
	"fmt"
	"sort"

	"github.com/digitalbiblesociety/dido/internal/audiomfcc"
	"github.com/digitalbiblesociety/dido/internal/dtw"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

// ComputeBoundaries aligns realMFCC's middle section against synthMFCC using
// DTW and returns per-fragment boundary frame indices in real-wave space.
//
// synthIntervals holds one entry per text fragment: its Begin is the anchor
// time (seconds) of that fragment in the synthesized audio. The first entry's
// Begin is ignored — fragment 0 always anchors at realMFCC.MiddleBegin()
// because head detection has already trimmed leading silence.
//
// deltaFrames is the DTW stripe half-width in frames (2*dtw_margin/mws).
// Exact DTW is used when synthMFCC.NumFrames() ≤ deltaFrames or when
// deltaFrames ≤ 0 (no stripe limit requested).
//
// Returns len(synthIntervals)+1 indices: one per fragment begin, followed by
// realMFCC.TailBegin() as the end of the last fragment.
//
// Failure modes:
//   - If either MFCC has zero frames, evenly-spaced artificial boundaries are
//     returned (no error). This is a defined degenerate case.
//   - If DTW returns no path despite non-empty inputs, an error is returned —
//     silently substituting evenly-spaced timestamps would hide real bugs.
//     Callers that want a fallback should call ComputeBoundariesOrFallback.
func ComputeBoundaries(
	realMFCC, synthMFCC *audiomfcc.AudioMFCC,
	synthIntervals []timing.TimeInterval,
	mws float64,
	deltaFrames int,
) ([]int, error) {
	middleBegin := realMFCC.MiddleBegin()
	tailBegin := realMFCC.TailBegin()
	n := len(synthIntervals)

	realSlice := realMFCC.Slice(middleBegin, realMFCC.MiddleEnd())
	if realSlice.NumFrames() == 0 || synthMFCC.NumFrames() == 0 {
		return artificialBoundaries(middleBegin, tailBegin, n), nil
	}

	var path []dtw.PathCell
	if deltaFrames <= 0 || synthMFCC.NumFrames() <= deltaFrames {
		path = dtw.ComputePathExact(realSlice.Matrix(), synthMFCC.Matrix())
	} else {
		path = dtw.ComputePathStripe(realSlice.Matrix(), synthMFCC.Matrix(), deltaFrames)
	}
	if len(path) == 0 {
		// Both matrices had frames but DTW couldn't produce a path. The
		// usual cause is fewer than 2 MFCC coefficient rows (the cost
		// matrix discards row 0, so an l-row matrix needs l ≥ 2). Surface
		// this rather than silently emitting fake timestamps.
		return nil, fmt.Errorf(
			"align: DTW produced no path "+
				"(real frames=%d, synth frames=%d, real rows=%d, synth rows=%d, delta=%d)",
			realSlice.NumFrames(), synthMFCC.NumFrames(),
			len(realSlice.Matrix()), len(synthMFCC.Matrix()), deltaFrames)
	}

	boundaries := make([]int, n+1)
	boundaries[n] = tailBegin
	if n == 0 {
		return boundaries, nil
	}

	// Fragment 0 always anchors at the start of the alignment region.
	boundaries[0] = middleBegin

	// For each subsequent fragment, find the first path position whose synth
	// frame index exceeds the anchor (searchsorted side="right"), then map
	// that position back to the corresponding real-wave frame.
	last := len(path) - 1
	for i := 1; i < n; i++ {
		af := int(synthIntervals[i].Begin.Float64() / mws)
		pos := sort.Search(last+1, func(k int) bool {
			return path[k].J > af
		})
		if pos > last {
			pos = last
		}
		boundaries[i] = path[pos].I + middleBegin
	}
	return boundaries, nil
}

// ComputeBoundariesOrFallback runs ComputeBoundaries and, when it fails,
// substitutes evenly-spaced artificial boundaries while passing the error
// to onFallback (e.g. a logger). The returned slice is always usable. This
// is the loose-fit wrapper for callers that prefer "best effort" output
// over a hard failure — most production callers should use ComputeBoundaries
// directly and decide for themselves.
func ComputeBoundariesOrFallback(
	realMFCC, synthMFCC *audiomfcc.AudioMFCC,
	synthIntervals []timing.TimeInterval,
	mws float64,
	deltaFrames int,
	onFallback func(err error),
) []int {
	b, err := ComputeBoundaries(realMFCC, synthMFCC, synthIntervals, mws, deltaFrames)
	if err == nil {
		return b
	}
	if onFallback != nil {
		onFallback(err)
	}
	return artificialBoundaries(realMFCC.MiddleBegin(), realMFCC.TailBegin(), len(synthIntervals))
}

func artificialBoundaries(middleBegin, tailBegin, n int) []int {
	b := make([]int, n+1)
	if n > 0 {
		step := float64(tailBegin-middleBegin) / float64(n)
		for i := 0; i < n; i++ {
			b[i] = middleBegin + int(float64(i)*step)
		}
	}
	b[n] = tailBegin
	return b
}
