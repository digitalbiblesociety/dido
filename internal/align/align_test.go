package align

import (
	"strings"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/audiomfcc"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

func mkAudioMFCC(rows, frames int) *audiomfcc.AudioMFCC {
	m := make([][]float64, rows)
	for i := range m {
		m[i] = make([]float64, frames)
		for j := range m[i] {
			m[i][j] = float64((i+1)*(j+1)) * 0.1
		}
	}
	a := audiomfcc.New(m, 0.040)
	a.SetMiddle(0, frames)
	return a
}

// TestComputeBoundariesEmptyMatricesArtificialFallback ensures empty inputs
// fall back to evenly-spaced boundaries (no error). This is the defined
// degenerate case.
func TestComputeBoundariesEmptyMatricesArtificialFallback(t *testing.T) {
	real := mkAudioMFCC(13, 100)
	empty := audiomfcc.New(nil, 0.040)
	intervals := []timing.TimeInterval{
		timing.NewTimeInterval(timing.FromFloat64(0.0), timing.FromFloat64(1.0)),
		timing.NewTimeInterval(timing.FromFloat64(1.0), timing.FromFloat64(2.0)),
	}
	b, err := ComputeBoundaries(real, empty, intervals, 0.040, 100)
	if err != nil {
		t.Fatalf("expected nil error for empty synth, got %v", err)
	}
	if len(b) != len(intervals)+1 {
		t.Errorf("len(b)=%d, want %d", len(b), len(intervals)+1)
	}
}

// TestComputeBoundariesDTWFailureSurfaced verifies that a degenerate MFCC
// with only the energy row (which DTW discards) produces an error instead
// of silently substituting evenly-spaced fake timestamps.
func TestComputeBoundariesDTWFailureSurfaced(t *testing.T) {
	// 1 row of MFCC: after DTW discards row 0, m1[1:] is empty → no path.
	real := mkAudioMFCC(1, 50)
	synth := mkAudioMFCC(1, 30)
	intervals := []timing.TimeInterval{
		timing.NewTimeInterval(timing.FromFloat64(0.0), timing.FromFloat64(0.5)),
		timing.NewTimeInterval(timing.FromFloat64(0.5), timing.FromFloat64(1.0)),
	}
	_, err := ComputeBoundaries(real, synth, intervals, 0.040, 100)
	if err == nil {
		t.Fatal("expected error when DTW produces no path, got nil")
	}
	if !strings.Contains(err.Error(), "DTW produced no path") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestComputeBoundariesNoIntervals verifies the n=0 edge case: the function
// returns just the tail-begin sentinel without panicking. Previously this
// path indexed pathPositions[0] on a zero-length slice.
func TestComputeBoundariesNoIntervals(t *testing.T) {
	real := mkAudioMFCC(13, 100)
	synth := mkAudioMFCC(13, 50)
	b, err := ComputeBoundaries(real, synth, nil, 0.040, 100)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(b) != 1 {
		t.Fatalf("len(b)=%d, want 1", len(b))
	}
	if b[0] != real.TailBegin() {
		t.Errorf("b[0]=%d, want TailBegin=%d", b[0], real.TailBegin())
	}
}

// TestComputeBoundariesNonPositiveDeltaUsesExactDTW verifies that a
// non-positive deltaFrames is treated as "no stripe limit" and routed
// through exact DTW rather than producing a malformed cost matrix.
func TestComputeBoundariesNonPositiveDeltaUsesExactDTW(t *testing.T) {
	real := mkAudioMFCC(13, 100)
	synth := mkAudioMFCC(13, 50)
	intervals := []timing.TimeInterval{
		timing.NewTimeInterval(timing.FromFloat64(0.0), timing.FromFloat64(1.0)),
		timing.NewTimeInterval(timing.FromFloat64(1.0), timing.FromFloat64(2.0)),
	}
	b, err := ComputeBoundaries(real, synth, intervals, 0.040, 0)
	if err != nil {
		t.Fatalf("expected nil error with delta=0, got %v", err)
	}
	if len(b) != 3 {
		t.Errorf("len(b)=%d, want 3", len(b))
	}
}

// TestComputeBoundariesFirstFragmentAnchorsAtMiddleBegin verifies the
// documented override: synthIntervals[0].Begin is ignored and the first
// fragment always starts at realMFCC.MiddleBegin().
func TestComputeBoundariesFirstFragmentAnchorsAtMiddleBegin(t *testing.T) {
	real := mkAudioMFCC(13, 100)
	real.SetMiddle(10, 90)
	synth := mkAudioMFCC(13, 50)
	intervals := []timing.TimeInterval{
		timing.NewTimeInterval(timing.FromFloat64(0.5), timing.FromFloat64(1.0)),
		timing.NewTimeInterval(timing.FromFloat64(1.0), timing.FromFloat64(2.0)),
	}
	b, err := ComputeBoundaries(real, synth, intervals, 0.040, 100)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if b[0] != 10 {
		t.Errorf("b[0]=%d, want middleBegin=10", b[0])
	}
	if b[len(b)-1] != 90 {
		t.Errorf("last boundary=%d, want tailBegin=90", b[len(b)-1])
	}
}

// TestComputeBoundariesOrFallbackSurfacesErrorAndReturnsBoundaries verifies the
// loose-fit wrapper: on DTW failure the callback fires AND artificial
// boundaries are returned (so the caller still gets a usable slice).
func TestComputeBoundariesOrFallbackSurfacesErrorAndReturnsBoundaries(t *testing.T) {
	real := mkAudioMFCC(1, 50)
	synth := mkAudioMFCC(1, 30)
	intervals := []timing.TimeInterval{
		timing.NewTimeInterval(timing.FromFloat64(0.0), timing.FromFloat64(0.5)),
		timing.NewTimeInterval(timing.FromFloat64(0.5), timing.FromFloat64(1.0)),
	}
	var seen error
	b := ComputeBoundariesOrFallback(real, synth, intervals, 0.040, 100, func(err error) {
		seen = err
	})
	if seen == nil {
		t.Fatal("expected onFallback callback to fire")
	}
	if len(b) != len(intervals)+1 {
		t.Errorf("len(b)=%d, want %d", len(b), len(intervals)+1)
	}
}
