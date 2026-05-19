package audiomfcc

import (
	"math"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/vad"
)

// makeMatrix builds a synthetic [mfccSize × numFrames] matrix
// with the energy row set to the given values.
func makeMatrix(energy []float64, mfccSize int) [][]float64 {
	m := make([][]float64, mfccSize)
	m[0] = make([]float64, len(energy))
	copy(m[0], energy)
	for c := 1; c < mfccSize; c++ {
		m[c] = make([]float64, len(energy))
	}
	return m
}

func TestNewBasic(t *testing.T) {
	energy := []float64{1, 2, 3, 4, 5}
	a := New(makeMatrix(energy, 3), 0.040)
	if a.NumFrames() != 5 {
		t.Errorf("NumFrames = %d", a.NumFrames())
	}
	if a.MiddleBegin() != 0 || a.MiddleEnd() != 5 {
		t.Errorf("middle = [%d, %d)", a.MiddleBegin(), a.MiddleEnd())
	}
}

func TestAudioLength(t *testing.T) {
	a := New(makeMatrix(make([]float64, 10), 2), 0.040)
	got := a.AudioLength().Float64()
	want := 10 * 0.040
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("AudioLength = %.6f, want %.6f", got, want)
	}
}

func TestSetMiddle(t *testing.T) {
	a := New(makeMatrix(make([]float64, 20), 2), 0.040)
	a.SetMiddle(3, 17)
	if a.MiddleBegin() != 3 || a.MiddleEnd() != 17 {
		t.Errorf("got [%d, %d)", a.MiddleBegin(), a.MiddleEnd())
	}
	if a.MiddleLength() != 14 {
		t.Errorf("MiddleLength = %d", a.MiddleLength())
	}
}

func TestRunVADAndIntervals(t *testing.T) {
	// 10 frames: frames 0-3 speech, 4-6 silence, 7-9 speech
	energy := []float64{5, 5, 5, 5, 0, 0, 0, 5, 5, 5}
	a := New(makeMatrix(energy, 2), 0.040)
	p := vad.Params{LogEnergyThreshold: 2.0, MinNonspeechLength: 3}
	a.RunVAD(p)

	fi := a.FrameIntervals(true)
	if len(fi) < 2 {
		t.Fatalf("want at least 2 speech intervals, got %d: %v", len(fi), fi)
	}

	ti := a.Intervals(false)
	if len(ti) == 0 {
		t.Fatal("expected nonspeech time intervals after RunVAD")
	}
}

func TestSlice(t *testing.T) {
	energy := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	a := New(makeMatrix(energy, 2), 0.040)
	s := a.Slice(2, 6)
	if s.NumFrames() != 4 {
		t.Errorf("sliced NumFrames = %d, want 4", s.NumFrames())
	}
	if s.Matrix()[0][0] != 3 {
		t.Errorf("first frame of slice = %f, want 3", s.Matrix()[0][0])
	}
}

func TestReverse(t *testing.T) {
	energy := []float64{1, 2, 3, 4, 5}
	a := New(makeMatrix(energy, 2), 0.040)
	a.SetMiddle(1, 4)
	a.Reverse()
	// Matrix should be reversed
	if a.Matrix()[0][0] != 5 {
		t.Errorf("after reverse, first frame = %f, want 5", a.Matrix()[0][0])
	}
	// middleBegin/End should be mirrored: begin=5-4=1, end=5-1=4
	if a.MiddleBegin() != 1 || a.MiddleEnd() != 4 {
		t.Errorf("after reverse, middle = [%d, %d), want [1, 4)", a.MiddleBegin(), a.MiddleEnd())
	}
}

func TestReverseVADIntervals(t *testing.T) {
	// 8 frames: 0-2 speech, 3-5 silence, 6-7 speech
	energy := []float64{5, 5, 5, 0, 0, 0, 5, 5}
	a := New(makeMatrix(energy, 2), 0.040)
	p := vad.Params{LogEnergyThreshold: 2.0, MinNonspeechLength: 3}
	a.RunVAD(p)

	si := a.FrameIntervals(true)
	ni := a.FrameIntervals(false)

	a.Reverse()

	// After reversal, speech/nonspeech intervals must be consistent with the reversed mask
	siRev := a.FrameIntervals(true)
	niRev := a.FrameIntervals(false)

	if len(siRev) != len(si) {
		t.Errorf("speech interval count changed: %d → %d", len(si), len(siRev))
	}
	if len(niRev) != len(ni) {
		t.Errorf("nonspeech interval count changed: %d → %d", len(ni), len(niRev))
	}
}
