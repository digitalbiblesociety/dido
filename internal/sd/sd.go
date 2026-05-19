// Package sd detects the audio head and tail of an audio file.
// Port of aeneas/sd.py.
package sd

import (
	"fmt"
	"math"

	"github.com/digitalbiblesociety/dido/internal/audiomfcc"
	"github.com/digitalbiblesociety/dido/internal/dtw"
	goMFCC "github.com/digitalbiblesociety/dido/internal/mfcc"
	"github.com/digitalbiblesociety/dido/internal/timing"
	"github.com/digitalbiblesociety/dido/internal/tts"
	"github.com/digitalbiblesociety/dido/internal/vad"
)

const (
	// queryFactor multiplied by maxLength gives the minimum amount of text to synthesize.
	queryFactor = 1.0
	// audioFactor multiplied by maxLength gives the audio search window length.
	// Must be >= 1.0 + queryFactor*1.5.
	audioFactor = 2.5
)

// DefaultMaxLength is the maximum head/tail length searched if none is specified.
var DefaultMaxLength = timing.MustParseTimeValue("10.000")

// DefaultMinLength is the minimum head/tail length if none is specified.
var DefaultMinLength = timing.Zero

// Params holds detection boundaries.
type Params struct {
	MinHeadLength timing.TimeValue
	MaxHeadLength timing.TimeValue
	MinTailLength timing.TimeValue
	MaxTailLength timing.TimeValue
}

// DefaultParams returns zero min / 10 s max for both head and tail.
func DefaultParams() Params {
	return Params{
		MinHeadLength: DefaultMinLength,
		MaxHeadLength: DefaultMaxLength,
		MinTailLength: DefaultMinLength,
		MaxTailLength: DefaultMaxLength,
	}
}

// Config holds runtime parameters needed by the detector.
type Config struct {
	// MFCCWindowShift is the seconds-per-frame value used for both the real wave and synthesis.
	MFCCWindowShift float64
	// MFCCParams is used to compute MFCCs for the synthesized audio.
	MFCCParams goMFCC.Params
	// VADParams is used to detect speech regions in the real wave.
	VADParams vad.Params
	// EspeakPath overrides the espeak-ng binary path ("" = use PATH).
	EspeakPath string

	// CachedForwardMFCC, when non-nil, lets DetectHead skip synthesis and
	// MFCC computation by reusing this caller-supplied forward-direction
	// synthesis result. It must be the MFCC of TTS-synthesizing the
	// fragments in their natural order (same order, language, and MFCC
	// params as this detector's config). Tail detection always re-synthesises
	// — fragment-interior order matters there and the cached forward
	// MFCC can't be reversed in place to satisfy it.
	CachedForwardMFCC *audiomfcc.AudioMFCC
}

// Detector wraps the real-wave AudioMFCC and the text fragments.
type Detector struct {
	realMFCC  *audiomfcc.AudioMFCC
	fragments []tts.Fragment
	cfg       Config
}

// New creates a Detector.
// fragments is the ordered list of text fragments to synthesize as a query.
// cfg.VADParams must already be in frame units (use vad.ParamsFromSeconds).
func New(realMFCC *audiomfcc.AudioMFCC, fragments []tts.Fragment, cfg Config) *Detector {
	return &Detector{realMFCC: realMFCC, fragments: fragments, cfg: cfg}
}

// DetectInterval detects [begin, end] of the spoken content within the audio.
// It combines DetectHead and DetectTail and returns (begin, end) in seconds.
// Returns (Zero, Zero) if detection fails.
func (d *Detector) DetectInterval(p Params) (begin, end timing.TimeValue, err error) {
	head, err := d.DetectHead(p.MinHeadLength, p.MaxHeadLength)
	if err != nil {
		return timing.Zero, timing.Zero, err
	}
	tail, err := d.DetectTail(p.MinTailLength, p.MaxTailLength)
	if err != nil {
		return timing.Zero, timing.Zero, err
	}
	audioLen := d.realMFCC.AudioLength()
	begin = head
	end = audioLen.Sub(tail)
	if begin.Less(timing.Zero) || !begin.Less(end) {
		return timing.Zero, timing.Zero, nil
	}
	return begin, end, nil
}

// DetectHead returns the duration of the audio head in seconds.
func (d *Detector) DetectHead(minLen, maxLen timing.TimeValue) (timing.TimeValue, error) {
	return d.detect(minLen, maxLen, false)
}

// DetectTail returns the duration of the audio tail in seconds.
func (d *Detector) DetectTail(minLen, maxLen timing.TimeValue) (timing.TimeValue, error) {
	return d.detect(minLen, maxLen, true)
}

func (d *Detector) detect(minLen, maxLen timing.TimeValue, tail bool) (timing.TimeValue, error) {
	if minLen.IsZero() {
		minLen = DefaultMinLength
	}
	if maxLen.IsZero() {
		maxLen = DefaultMaxLength
	}

	mws := d.cfg.MFCCWindowShift
	if mws <= 0 {
		return timing.Zero, fmt.Errorf("sd: MFCCWindowShift must be positive")
	}
	minFrames := int(minLen.Float64() / mws)
	maxFrames := int(maxLen.Float64() / mws)

	// Head detection can reuse a caller-supplied forward synthesis MFCC
	// when available (typical: the main alignment pipeline already
	// computed it). Tail detection always synthesizes — the MFCC of a
	// time-reversed audio is not equivalent to the time-reversed MFCC of
	// the forward audio (fragment interior order matters).
	var queryAM *audiomfcc.AudioMFCC
	if !tail && d.cfg.CachedForwardMFCC != nil {
		// Clone so the in-place Reverse() below (when tail==true) never
		// mutates the cache. For tail==false we just borrow read-only.
		queryAM = d.cfg.CachedForwardMFCC
	} else {
		synthDuration := timing.FromFloat64(maxLen.Float64() * queryFactor)
		synthResult, err := d.synthesizeQuery(synthDuration, tail)
		if err != nil {
			return timing.Zero, fmt.Errorf("sd: synthesis failed: %w", err)
		}
		if len(synthResult.Samples) == 0 {
			return timing.Zero, nil
		}
		queryAM, err = audiomfcc.FromSamples(synthResult.Samples, synthResult.SampleRate, d.cfg.MFCCParams)
		if err != nil {
			return timing.Zero, fmt.Errorf("sd: query MFCC: %w", err)
		}
	}

	// Search window: first audioFactor * maxLen seconds of real audio
	searchWindowFrames := int(maxLen.Float64()*audioFactor/mws) + 1
	realN := d.realMFCC.NumFrames()
	if searchWindowFrames > realN {
		searchWindowFrames = realN
	}
	searchEnd := searchWindowFrames

	// Reverse real MFCC and query for tail detection (reduces to head problem)
	realAM := d.realMFCC
	if tail {
		realAM = cloneAudioMFCC(d.realMFCC)
		realAM.Reverse()
		queryAM.Reverse()
	}

	// Ensure VAD has been run on the real wave
	if realAM.Mask() == nil {
		realAM.RunVAD(d.cfg.VADParams)
	}
	speechFI := realAM.FrameIntervals(true)
	if len(speechFI) == 0 {
		return timing.Zero, nil
	}

	// Collect candidate begin frame indices (speech interval starts in [minFrames, maxFrames])
	type candidate struct {
		minVal     float64
		beginFrame int
	}
	var candidates []candidate

	for _, iv := range speechFI {
		begin := iv[0]
		if begin < minFrames || begin > maxFrames {
			continue
		}
		// DTW: align full query against real[begin:searchEnd]
		realSlice := realAM.Slice(begin, searchEnd)
		acm := dtw.ComputeCostMatrixExact(realSlice.Matrix(), queryAM.Matrix())
		if acm == nil {
			continue
		}
		dtw.AccumulateCostMatrixExactInPlace(acm)
		// Min over last column (= matching full query against any prefix of real slice)
		nReal := len(acm)
		if nReal == 0 {
			continue
		}
		nQuery := len(acm[0])
		if nQuery == 0 {
			continue
		}
		minVal := math.MaxFloat64
		for i := 0; i < nReal; i++ {
			if v := acm[i][nQuery-1]; v < minVal {
				minVal = v
			}
		}
		candidates = append(candidates, candidate{minVal, begin})
	}

	// Reverse back if we were doing tail detection
	if tail {
		realAM.Reverse()
	}

	if len(candidates) == 0 {
		return timing.Zero, nil
	}

	// Best candidate = minimum DTW cost
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.minVal < best.minVal {
			best = c
		}
	}
	return timing.FromFloat64(float64(best.beginFrame) * mws), nil
}

// synthesizeQuery synthesizes all fragments (optionally reversed for tail detection)
// and returns the combined audio. The caller limits exploration via the search window.
func (d *Detector) synthesizeQuery(synthDuration timing.TimeValue, tail bool) (*tts.Result, error) {
	_ = synthDuration // search window limits exploration; synthesize all fragments
	frags := d.fragments
	if tail {
		rev := make([]tts.Fragment, len(frags))
		for i, f := range frags {
			rev[len(frags)-1-i] = f
		}
		frags = rev
	}
	return tts.SynthesizeMultiple(frags, d.cfg.EspeakPath)
}

// cloneAudioMFCC deep-copies an AudioMFCC so Reverse() on the clone
// does not affect the original.
func cloneAudioMFCC(a *audiomfcc.AudioMFCC) *audiomfcc.AudioMFCC {
	src := a.Matrix()
	if len(src) == 0 {
		return audiomfcc.New(nil, a.WindowShift())
	}
	dst := make([][]float64, len(src))
	for c, row := range src {
		dst[c] = make([]float64, len(row))
		copy(dst[c], row)
	}
	clone := audiomfcc.New(dst, a.WindowShift())
	clone.SetMiddle(a.MiddleBegin(), a.MiddleEnd())
	return clone
}
