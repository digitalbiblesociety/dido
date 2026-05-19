package mfcc

import (
	"fmt"
	"math"
	"runtime"
	"sync"
)

// ParallelThreshold is the minimum number of frames at which Compute will
// automatically fan out across CPU cores. Below this, the serial path is
// faster (goroutine setup dominates). Exposed for benchmarking.
var ParallelThreshold = 256

// MaxWorkers is the upper bound on goroutines Compute will spawn. 0 (default)
// means runtime.NumCPU().
var MaxWorkers = 0

const (
	mel10  = 2595.0
	cutoff = 0.00001
	pi     = math.Pi
	pi2    = 2 * math.Pi
)

// Params holds the MFCC computation parameters.
// Default values match RuntimeConfiguration in runtimeconfiguration.py.
type Params struct {
	FilterBankSize  int     // number of Mel filter bank filters (default 40)
	MFCCSize        int     // number of MFCC coefficients (default 13)
	FFTOrder        int     // FFT order, must be power of 2 (default 512)
	LowerFrequency  float64 // lower cut-off frequency in Hz (default 133.3333)
	UpperFrequency  float64 // upper cut-off frequency in Hz (default 6855.4976)
	EmphasisFactor  float64 // pre-emphasis factor (default 0.97)
	WindowLength    float64 // analysis window length in seconds (default 0.025)
	WindowShift     float64 // analysis window shift in seconds (default 0.010)
}

// DefaultParams returns the same defaults as aeneas RuntimeConfiguration.
func DefaultParams() Params {
	return Params{
		FilterBankSize: 40,
		MFCCSize:       13,
		FFTOrder:       512,
		LowerFrequency: 133.3333,
		UpperFrequency: 6855.4976,
		EmphasisFactor: 0.97,
		WindowLength:   0.025,
		WindowShift:    0.010,
	}
}

// Validate returns nil if p describes a usable MFCC parameter set, or a
// descriptive error otherwise. Catches the cases where the SPTK FFT or the
// Mel filter bank would silently produce garbage:
//   - FFTOrder must be a positive power of 2 (FFT is radix-2)
//   - FilterBankSize, MFCCSize must be > 0
//   - LowerFrequency must be ≥ 0 and strictly less than UpperFrequency
//   - WindowLength, WindowShift must be > 0
//   - EmphasisFactor must be in [0, 1] (1 is allowed but extreme)
//
// Nyquist clamping (UpperFrequency ≤ sampleRate/2) is checked at compute time
// since sampleRate isn't on Params.
func (p Params) Validate() error {
	if p.FFTOrder <= 0 {
		return fmt.Errorf("mfcc: FFTOrder must be > 0, got %d", p.FFTOrder)
	}
	if p.FFTOrder&(p.FFTOrder-1) != 0 {
		return fmt.Errorf("mfcc: FFTOrder must be a power of 2, got %d", p.FFTOrder)
	}
	if p.FilterBankSize <= 0 {
		return fmt.Errorf("mfcc: FilterBankSize must be > 0, got %d", p.FilterBankSize)
	}
	if p.MFCCSize <= 0 {
		return fmt.Errorf("mfcc: MFCCSize must be > 0, got %d", p.MFCCSize)
	}
	if p.MFCCSize > p.FilterBankSize {
		return fmt.Errorf("mfcc: MFCCSize (%d) cannot exceed FilterBankSize (%d)",
			p.MFCCSize, p.FilterBankSize)
	}
	if p.LowerFrequency < 0 {
		return fmt.Errorf("mfcc: LowerFrequency must be ≥ 0, got %g", p.LowerFrequency)
	}
	if p.UpperFrequency <= p.LowerFrequency {
		return fmt.Errorf("mfcc: UpperFrequency (%g) must be > LowerFrequency (%g)",
			p.UpperFrequency, p.LowerFrequency)
	}
	if p.WindowLength <= 0 {
		return fmt.Errorf("mfcc: WindowLength must be > 0, got %g", p.WindowLength)
	}
	if p.WindowShift <= 0 {
		return fmt.Errorf("mfcc: WindowShift must be > 0, got %g", p.WindowShift)
	}
	if p.EmphasisFactor < 0 || p.EmphasisFactor > 1 {
		return fmt.Errorf("mfcc: EmphasisFactor must be in [0, 1], got %g", p.EmphasisFactor)
	}
	return nil
}

// Compiled holds the static lookup tables (sin tables, Hamming window, Mel
// filter bank, DCT matrix) and per-call scratch buffers for a fixed
// (Params, sampleRate) pair. Repeated Compute calls reuse the tables and
// the working buffers, avoiding the ~5–10% overhead of recomputing them
// on every short-clip call (e.g. the SD detector's per-fragment slices).
//
// A Compiled instance is NOT safe for concurrent use. Callers running
// MFCC concurrently should hold one Compiled per goroutine, or call
// Compile inside the goroutine.
type Compiled struct {
	params     Params
	sampleRate uint32

	// Static lookup tables.
	sinFull []float64
	sinHalf []float64
	hamming []float64
	filters []float64
	s2dct   []float64

	// Derived sizes.
	frameLen       int
	frameLenPadded int
	frameShift     int
	filtersN       int

	// Per-frame scratch buffers (reused across Compute calls).
	frame    []float64
	power    []float64
	logsp    []float64
	tmp      []float64
	origBuf  []float64 // pre-emphasis original frame copy
}

// Compile validates p and returns a reusable Compiled bound to the given
// sample rate. Callers that only do one Compute per (params, rate) can use
// ComputeFromData instead; Compile is intended for batch / repeated use.
func Compile(p Params, sampleRate uint32) (*Compiled, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if sampleRate == 0 {
		return nil, fmt.Errorf("mfcc: sampleRate must be > 0")
	}
	nyquist := float64(sampleRate) / 2.0
	if p.UpperFrequency > nyquist {
		return nil, fmt.Errorf("mfcc: upper frequency %f exceeds Nyquist %f", p.UpperFrequency, nyquist)
	}
	frameLen := int(math.Floor(p.WindowLength * float64(sampleRate)))
	frameShift := int(math.Floor(p.WindowShift * float64(sampleRate)))
	if frameShift == 0 {
		return nil, fmt.Errorf("mfcc: WindowShift %g at sampleRate %d yields zero-sample frame shift",
			p.WindowShift, sampleRate)
	}
	frameLenPadded := frameLen
	if p.FFTOrder > frameLenPadded {
		frameLenPadded = p.FFTOrder
	}
	filtersN := p.FFTOrder/2 + 1
	return &Compiled{
		params:         p,
		sampleRate:     sampleRate,
		sinFull:        precomputeSinTable(p.FFTOrder),
		sinHalf:        precomputeSinTable(p.FFTOrder / 2),
		hamming:        precomputeHamming(frameLen),
		filters:        createMelFilterBank(p.FFTOrder, p.FilterBankSize, int(sampleRate), p.UpperFrequency, p.LowerFrequency),
		s2dct:          createDCTMatrix(p.MFCCSize, p.FilterBankSize),
		frameLen:       frameLen,
		frameLenPadded: frameLenPadded,
		frameShift:     frameShift,
		filtersN:       filtersN,
		frame:          make([]float64, frameLenPadded),
		power:          make([]float64, filtersN),
		logsp:          make([]float64, p.FilterBankSize),
		tmp:            make([]float64, p.FFTOrder+p.FFTOrder/2+2),
		origBuf:        make([]float64, frameLen),
	}, nil
}

// Params returns the parameter set this Compiled was built for.
func (c *Compiled) Params() Params { return c.params }

// SampleRate returns the sample rate this Compiled was built for.
func (c *Compiled) SampleRate() uint32 { return c.sampleRate }

// Compute runs the MFCC pipeline on data and returns a freshly-allocated
// [mfcc_size × num_frames] matrix. Static tables are reused; per-call scratch
// is either the Compiled's own buffers (serial path) or per-worker buffers
// (parallel path).
//
// When numFrames ≥ ParallelThreshold and the runtime has > 1 CPU, Compute
// automatically fans out across goroutines. The output is bit-identical to
// the serial path: pre-emphasis state is the only cross-frame dependency,
// and we pre-compute it from raw data before splitting work.
func (c *Compiled) Compute(data []float64) ([][]float64, error) {
	n := len(data)
	if n == 0 {
		return nil, fmt.Errorf("mfcc: data is empty")
	}
	p := c.params
	numFrames := int(float64(n) / float64(c.frameShift))

	mfcc := make([][]float64, p.MFCCSize)
	for i := range mfcc {
		mfcc[i] = make([]float64, numFrames)
	}

	// Decide serial vs. parallel.
	workers := MaxWorkers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if numFrames < ParallelThreshold || workers <= 1 {
		c.computeFrameRange(data, 0, numFrames, 0, mfcc,
			c.frame, c.power, c.logsp, c.tmp, c.origBuf)
		return mfcc, nil
	}
	if workers > numFrames {
		workers = numFrames
	}

	// Pre-compute the prior carried into each frame. priors[0] = 0; for
	// frame i > 0, the prior is the value of data at the last sample
	// position of frame i-1 (or 0 if that position is outside the data,
	// matching the zero-padding in the serial path).
	priors := make([]float64, numFrames)
	for i := 1; i < numFrames; i++ {
		idx := (i-1)*c.frameShift + c.frameLen - 1
		if idx < n {
			priors[i] = data[idx]
		}
	}

	chunk := (numFrames + workers - 1) / workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		start := w * chunk
		if start >= numFrames {
			break
		}
		end := start + chunk
		if end > numFrames {
			end = numFrames
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			frame := make([]float64, c.frameLenPadded)
			power := make([]float64, c.filtersN)
			logsp := make([]float64, p.FilterBankSize)
			tmp := make([]float64, p.FFTOrder+p.FFTOrder/2+2)
			origBuf := make([]float64, c.frameLen)
			c.computeFrameRange(data, start, end, priors[start], mfcc,
				frame, power, logsp, tmp, origBuf)
		}(start, end)
	}
	wg.Wait()
	return mfcc, nil
}

// computeFrameRange computes MFCC frames in [start, end), writing into
// mfcc[*][fi]. The supplied scratch buffers are reused across frames within
// this range. prior is the carry-in pre-emphasis value for frame `start`.
func (c *Compiled) computeFrameRange(
	data []float64,
	start, end int,
	prior float64,
	mfcc [][]float64,
	frame, power, logsp, tmp, origBuf []float64,
) {
	p := c.params
	n := len(data)
	for fi := start; fi < end; fi++ {
		// Zero the working buffers.
		for i := range frame {
			frame[i] = 0
		}
		for i := range power {
			power[i] = 0
		}
		for i := range logsp {
			logsp[i] = 0
		}

		// Copy samples for this frame.
		fStart := fi * c.frameShift
		fEnd := fStart + c.frameLen
		if fEnd > n {
			fEnd = n
		}
		copy(frame[:fEnd-fStart], data[fStart:fEnd])

		// Pre-emphasis (stateful).
		prior = applyEmphasisCached(frame, c.frameLen, p.EmphasisFactor, prior, origBuf)

		// Hamming window.
		for k := 0; k < c.frameLen; k++ {
			frame[k] *= c.hamming[k]
		}

		// RFFT + power spectrum.
		for i := range tmp {
			tmp[i] = 0
		}
		rfftInPlace(frame, tmp, p.FFTOrder, c.sinFull, c.sinHalf)
		power[0] = frame[0] * frame[0]
		for k := 1; k < c.filtersN; k++ {
			power[k] = frame[k]*frame[k] + tmp[k]*tmp[k]
		}

		// Mel filter bank → log.
		for j := 0; j < p.FilterBankSize; j++ {
			acc := 0.0
			for i := 0; i < c.filtersN; i++ {
				acc += power[i] * c.filters[i*p.FilterBankSize+j]
			}
			if acc < cutoff {
				acc = cutoff
			}
			logsp[j] = math.Log(acc)
		}

		// DCT.
		for i := 0; i < p.MFCCSize; i++ {
			acc := 0.0
			for j := 0; j < p.FilterBankSize; j++ {
				acc += logsp[j] * c.s2dct[i*p.FilterBankSize+j]
			}
			mfcc[i][fi] = acc / float64(p.FilterBankSize)
		}
	}
}

// ComputeFromData computes the MFCC matrix from raw float64 audio samples.
// Returns a matrix of shape [mfcc_size × num_frames] (column-major, like the C extension).
// data must be mono float64 samples normalised to [-1, 1].
// sampleRate is the sample rate of data in Hz.
//
// This is a convenience wrapper that compiles a fresh Compiled on every call.
// For batch / repeated work (e.g. one MFCC per audio fragment in the SD
// detector), use Compile once and call (*Compiled).Compute per audio buffer.
func ComputeFromData(data []float64, sampleRate uint32, p Params) ([][]float64, error) {
	c, err := Compile(p, sampleRate)
	if err != nil {
		return nil, err
	}
	return c.Compute(data)
}

// applyEmphasisCached is applyEmphasis but borrows the scratch slice from
// the caller instead of allocating per-frame.
func applyEmphasisCached(frame []float64, length int, factor, prior float64, orig []float64) float64 {
	priorOrig := frame[length-1]
	copy(orig[:length], frame[:length])
	frame[0] = orig[0] - factor*prior
	for i := 1; i < length; i++ {
		frame[i] = orig[i] - orig[i-1]*factor
	}
	return priorOrig
}

// precomputeHamming builds the Hamming window coefficients.
// Matches _precompute_hamming in cmfcc_func.c.
func precomputeHamming(frameLen int) []float64 {
	arg := pi2 / float64(frameLen-1)
	c := make([]float64, frameLen)
	for k := 0; k < frameLen; k++ {
		c[k] = 0.54 - 0.46*math.Cos(float64(k)*arg)
	}
	return c
}

// hz2mel converts Hz to Mel.
func hz2mel(f float64) float64 {
	return mel10 * math.Log10(1.0+f/700.0)
}

// mel2hz converts Mel to Hz.
func mel2hz(m float64) float64 {
	return 700.0 * (math.Pow(10.0, m/mel10) - 1.0)
}

// roundNearest rounds x to the nearest non-negative integer.
func roundNearest(x float64) int {
	if x < 0 {
		return 0
	}
	return int(math.Floor(x + 0.5))
}

// createMelFilterBank builds a (filtersN × filterBankSize) filter matrix.
// filtersN = fftOrder/2 + 1.
// Matches _create_mel_filter_bank in cmfcc_func.c.
// The returned slice is row-major: filters[i*filterBankSize+j].
func createMelFilterBank(fftOrder, filterBankSize, sampleRate int, upperFreq, lowerFreq float64) []float64 {
	stepFreq := float64(sampleRate) / float64(fftOrder)
	melmax := hz2mel(upperFreq)
	melmin := hz2mel(lowerFreq)
	melstep := (melmax - melmin) / float64(filterBankSize+1)
	filterEdgeLen := filterBankSize + 2
	filtersN := fftOrder/2 + 1

	filters := make([]float64, filtersN*filterBankSize)

	filterEdges := make([]float64, filterEdgeLen)
	for k := 0; k < filterEdgeLen; k++ {
		filterEdges[k] = mel2hz(melmin + melstep*float64(k))
	}

	for k := 0; k < filterBankSize; k++ {
		leftFreq := roundNearest(filterEdges[k] / stepFreq)
		centerFreq := roundNearest(filterEdges[k+1] / stepFreq)
		rightFreq := roundNearest(filterEdges[k+2] / stepFreq)
		widthFreq := float64(rightFreq-leftFreq) * stepFreq
		heightFreq := 2.0 / widthFreq

		var leftSlope float64
		if centerFreq != leftFreq {
			leftSlope = heightFreq / float64(centerFreq-leftFreq)
		}
		cur := leftFreq + 1
		for cur < centerFreq {
			filters[cur*filterBankSize+k] = float64(cur-leftFreq) * leftSlope
			cur++
		}
		if cur == centerFreq {
			filters[cur*filterBankSize+k] = heightFreq
			cur++
		}
		var rightSlope float64
		if centerFreq != rightFreq {
			rightSlope = heightFreq / float64(centerFreq-rightFreq)
		}
		for cur < rightFreq {
			filters[cur*filterBankSize+k] = float64(cur-rightFreq) * rightSlope
			cur++
		}
	}
	return filters
}

// createDCTMatrix builds the (mfccSize × filterBankSize) DCT matrix.
// Matches _create_dct_matrix in cmfcc_func.c (the "Sphinx-style" not-quite-DCT).
// The returned slice is row-major: s2dct[i*filterBankSize+j].
func createDCTMatrix(mfccSize, filterBankSize int) []float64 {
	s2dct := make([]float64, mfccSize*filterBankSize)
	for i := 0; i < mfccSize; i++ {
		freq := pi * float64(i) / float64(filterBankSize)
		for j := 0; j < filterBankSize; j++ {
			v := math.Cos(freq * (0.5 + float64(j)))
			if j == 0 {
				v *= 0.5
			}
			s2dct[i*filterBankSize+j] = v
		}
	}
	return s2dct
}
