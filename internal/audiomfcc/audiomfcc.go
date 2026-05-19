// Package audiomfcc provides an MFCC matrix wrapper with lazy VAD and interval extraction.
// Port of aeneas/audiofilemfcc.py.
package audiomfcc

import (
	"fmt"

	"github.com/digitalbiblesociety/dido/internal/mfcc"
	"github.com/digitalbiblesociety/dido/internal/timing"
	"github.com/digitalbiblesociety/dido/internal/vad"
)

// AudioMFCC wraps a 2D MFCC matrix and provides VAD-based interval extraction.
// Matrix layout: matrix[coeff][frame] (coeff = 0 is energy).
type AudioMFCC struct {
	matrix      [][]float64
	windowShift float64 // seconds per frame (mfcc window shift)

	// Middle section [middleBegin, middleEnd) in frame indices.
	// After construction these span the whole matrix.
	middleBegin int
	middleEnd   int

	// VAD results; nil until RunVAD is called.
	mask               []bool
	speechIntervals    [][2]int // [begin, end] inclusive
	nonspeechIntervals [][2]int
}

// New creates an AudioMFCC from an already-computed MFCC matrix.
// windowShift is the MFCC window shift in seconds (seconds per frame).
func New(matrix [][]float64, windowShift float64) *AudioMFCC {
	var n int
	if len(matrix) > 0 {
		n = len(matrix[0])
	}
	return &AudioMFCC{
		matrix:      matrix,
		windowShift: windowShift,
		middleBegin: 0,
		middleEnd:   n,
	}
}

// FromSamples computes MFCCs from raw audio samples and wraps them.
func FromSamples(samples []float64, sampleRate uint32, mp mfcc.Params) (*AudioMFCC, error) {
	matrix, err := mfcc.ComputeFromData(samples, sampleRate, mp)
	if err != nil {
		return nil, fmt.Errorf("audiomfcc: %w", err)
	}
	return New(matrix, mp.WindowShift), nil
}

// FromSamplesCompiled is FromSamples but uses a pre-compiled MFCC table set.
// Useful for batch / repeated calls (e.g. the SD detector synthesising and
// MFCC'ing many short audio slices) where the static-tables cost dominates.
//
// The caller is responsible for ensuring the Compiled was built with the
// same sampleRate as the samples.
func FromSamplesCompiled(samples []float64, c *mfcc.Compiled) (*AudioMFCC, error) {
	matrix, err := c.Compute(samples)
	if err != nil {
		return nil, fmt.Errorf("audiomfcc: %w", err)
	}
	return New(matrix, c.Params().WindowShift), nil
}

// WindowShift returns the MFCC window shift in seconds (seconds per frame).
func (a *AudioMFCC) WindowShift() float64 { return a.windowShift }

// NumFrames returns the total number of frames in the full matrix.
func (a *AudioMFCC) NumFrames() int {
	if len(a.matrix) == 0 {
		return 0
	}
	return len(a.matrix[0])
}

// Matrix returns the underlying MFCC matrix (read-only; do not modify).
func (a *AudioMFCC) Matrix() [][]float64 { return a.matrix }

// AudioLength returns the duration of the full matrix in seconds.
func (a *AudioMFCC) AudioLength() timing.TimeValue {
	return timing.FromFloat64(float64(a.NumFrames()) * a.windowShift)
}

// MiddleBegin returns the first frame of the middle (non-head/tail) section.
func (a *AudioMFCC) MiddleBegin() int { return a.middleBegin }

// MiddleEnd returns the one-past-last frame of the middle section.
func (a *AudioMFCC) MiddleEnd() int { return a.middleEnd }

// MiddleLength returns the number of frames in the middle section.
func (a *AudioMFCC) MiddleLength() int { return a.middleEnd - a.middleBegin }

// TailBegin returns the frame index at which the tail section begins
// (= middleEnd, the first frame after the middle).
func (a *AudioMFCC) TailBegin() int { return a.middleEnd }

// SetMiddle sets the middle section frame boundaries.
// Both begin and end are clamped to [0, NumFrames()].
func (a *AudioMFCC) SetMiddle(begin, end int) {
	n := a.NumFrames()
	if begin < 0 {
		begin = 0
	}
	if begin > n {
		begin = n
	}
	if end < begin {
		end = begin
	}
	if end > n {
		end = n
	}
	a.middleBegin = begin
	a.middleEnd = end
}

// SetHeadMiddleTail sets head/tail lengths (in seconds), updating middleBegin/End.
// Nil means "keep current". headLength and tailLength must not exceed AudioLength.
func (a *AudioMFCC) SetHeadMiddleTail(headLength, middleLength, tailLength *timing.TimeValue) {
	mws := a.windowShift
	if mws <= 0 {
		return
	}
	if headLength != nil {
		a.middleBegin = int(headLength.Float64() / mws)
	}
	if middleLength != nil {
		a.middleEnd = a.middleBegin + int(middleLength.Float64()/mws)
	} else if tailLength != nil {
		a.middleEnd = a.NumFrames() - int(tailLength.Float64()/mws)
	}
}

// RunVAD runs voice activity detection on the energy vector (row 0 of the matrix)
// using the given parameters, and caches the resulting mask and intervals.
// Safe to call multiple times; subsequent calls update the cached results.
func (a *AudioMFCC) RunVAD(p vad.Params) {
	if len(a.matrix) == 0 {
		return
	}
	a.mask = vad.RunVAD(a.matrix[0], p)
	a.speechIntervals = vad.ComputeIntervals(a.mask, true)
	a.nonspeechIntervals = vad.ComputeIntervals(a.mask, false)
}

// Mask returns the current VAD bool mask (nil if RunVAD has not been called).
func (a *AudioMFCC) Mask() []bool { return a.mask }

// FrameIntervals returns the cached frame intervals.
// If speech is true, returns speech intervals; otherwise nonspeech.
// Returns nil if RunVAD has not been called.
func (a *AudioMFCC) FrameIntervals(speech bool) [][2]int {
	if speech {
		return a.speechIntervals
	}
	return a.nonspeechIntervals
}

// Intervals returns speech or nonspeech intervals as TimingInterval slices.
// The frame pair [b, e] (inclusive) maps to the time interval [b*ws, (e+1)*ws).
// Returns nil if RunVAD has not been called.
func (a *AudioMFCC) Intervals(speech bool) []timing.TimeInterval {
	var fi [][2]int
	if speech {
		fi = a.speechIntervals
	} else {
		fi = a.nonspeechIntervals
	}
	if fi == nil {
		return nil
	}
	mws := a.windowShift
	out := make([]timing.TimeInterval, len(fi))
	for i, pair := range fi {
		b := timing.FromFloat64(float64(pair[0]) * mws)
		e := timing.FromFloat64(float64(pair[1]+1) * mws)
		out[i] = timing.NewTimeInterval(b, e)
	}
	return out
}

// Slice returns a new AudioMFCC that is a column slice of the matrix [begin, end).
// The new instance has middleBegin=0, middleEnd=end-begin, and no VAD results.
func (a *AudioMFCC) Slice(begin, end int) *AudioMFCC {
	if begin < 0 {
		begin = 0
	}
	if len(a.matrix) == 0 {
		return New(nil, a.windowShift)
	}
	n := len(a.matrix[0])
	if end > n {
		end = n
	}
	sliced := make([][]float64, len(a.matrix))
	for c, row := range a.matrix {
		sliced[c] = row[begin:end]
	}
	return New(sliced, a.windowShift)
}

// Reverse reverses the MFCC matrix in place (time axis) and updates middleBegin/End
// and any cached VAD intervals to match, mirroring audiofilemfcc.py's reverse().
func (a *AudioMFCC) Reverse() {
	n := a.NumFrames()
	// Reverse each coefficient row
	for c := range a.matrix {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			a.matrix[c][i], a.matrix[c][j] = a.matrix[c][j], a.matrix[c][i]
		}
	}
	// Update middle section
	newEnd := n - a.middleBegin
	newBegin := n - a.middleEnd
	a.middleBegin = newBegin
	a.middleEnd = newEnd

	// Update VAD results if present
	if a.mask != nil {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			a.mask[i], a.mask[j] = a.mask[j], a.mask[i]
		}
		// Reverse and remap intervals: [b, e] → [n-1-e, n-1-b]
		rev := func(intervals [][2]int) [][2]int {
			r := make([][2]int, len(intervals))
			for i, iv := range intervals {
				r[len(intervals)-1-i] = [2]int{n - 1 - iv[1], n - 1 - iv[0]}
			}
			return r
		}
		a.speechIntervals = rev(a.speechIntervals)
		a.nonspeechIntervals = rev(a.nonspeechIntervals)
	}
}
