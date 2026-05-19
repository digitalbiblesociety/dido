// Package vad provides a simple voice activity detector
// based on the energy of the 0th MFCC coefficient.
// Port of aeneas/vad.py.
package vad

// Params holds VAD parameters expressed in frames (not seconds).
// Use ParamsFromSeconds to build from the RuntimeConfig second-based values.
type Params struct {
	LogEnergyThreshold float64 // min log-energy above the minimum to be speech
	MinNonspeechLength int     // min consecutive frames for a nonspeech region
	ExtendBefore       int     // extend speech by this many frames before each nonspeech region (from the right)
	ExtendAfter        int     // extend speech by this many frames after each nonspeech region (from the left)
}

// ParamsFromSeconds converts second-based config values to frame-based Params.
func ParamsFromSeconds(logEnergyThreshold, minNonspeechLengthSec, extendBeforeSec, extendAfterSec, windowShift float64) Params {
	toFrames := func(s float64) int {
		if windowShift <= 0 {
			return 0
		}
		return int(s / windowShift)
	}
	return Params{
		LogEnergyThreshold: logEnergyThreshold,
		MinNonspeechLength: toFrames(minNonspeechLengthSec),
		ExtendBefore:       toFrames(extendBeforeSec),
		ExtendAfter:        toFrames(extendAfterSec),
	}
}

// RunVAD returns a bool mask of length len(energy) where true = speech.
// energy is the 0th MFCC coefficient vector (one value per frame).
func RunVAD(energy []float64, p Params) []bool {
	n := len(energy)
	if n == 0 {
		return nil
	}

	// Energy threshold = min(energy) + logEnergyThreshold
	minE := energy[0]
	for _, e := range energy[1:] {
		if e < minE {
			minE = e
		}
	}
	threshold := minE + p.LogEnergyThreshold

	// Initial per-frame speech decision
	initialSpeech := make([]bool, n)
	for i, e := range energy {
		initialSpeech[i] = e >= threshold
	}

	w := p.MinNonspeechLength
	if w <= 0 {
		w = 1
	}

	// Prefix sum of nonspeech frames (false → nonspeech)
	nsPrefix := make([]int, n+1)
	for i, s := range initialSpeech {
		nsPrefix[i+1] = nsPrefix[i]
		if !s {
			nsPrefix[i+1]++
		}
	}

	// Find window-start indices where all w frames are nonspeech
	maxW := n - w + 1
	if maxW <= 0 {
		// Window wider than vector: declare everything speech (no nonspeech run long enough)
		mask := make([]bool, n)
		for i := range mask {
			mask[i] = true
		}
		return mask
	}

	// Collect runs of consecutive all-nonspeech window starts
	type run struct{ start, end int }
	var runs []run
	inRun := false
	runStart := 0
	for i := 0; i < maxW; i++ {
		allNS := (nsPrefix[i+w] - nsPrefix[i]) == w
		if allNS {
			if !inRun {
				runStart = i
				inRun = true
			}
		} else if inRun {
			runs = append(runs, run{runStart, i - 1})
			inRun = false
		}
	}
	if inRun {
		runs = append(runs, run{runStart, maxW - 1})
	}

	// Start with all speech; carve out nonspeech runs
	mask := make([]bool, n)
	for i := range mask {
		mask[i] = true
	}
	for _, r := range runs {
		start := r.start
		if p.ExtendAfter > 0 && start > 0 {
			start += p.ExtendAfter
		}
		stop := r.end + w // exclusive end of nonspeech region
		if p.ExtendBefore > 0 && stop < n-1 {
			stop -= p.ExtendBefore
		}
		if start < 0 {
			start = 0
		}
		if stop > n {
			stop = n
		}
		for i := start; i < stop; i++ {
			mask[i] = false
		}
	}
	return mask
}

// ComputeIntervals converts a bool mask into sorted [begin, end] inclusive frame pairs.
// If speech is true, intervals where mask is true are returned; otherwise where mask is false.
func ComputeIntervals(mask []bool, speech bool) [][2]int {
	n := len(mask)
	var intervals [][2]int
	inInterval := false
	start := 0
	for i := 0; i < n; i++ {
		matches := mask[i] == speech
		if matches && !inInterval {
			start = i
			inInterval = true
		} else if !matches && inInterval {
			intervals = append(intervals, [2]int{start, i - 1})
			inInterval = false
		}
	}
	if inInterval {
		intervals = append(intervals, [2]int{start, n - 1})
	}
	return intervals
}
