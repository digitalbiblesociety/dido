// Package aba implements the adjust-boundary algorithm (ABA) for forced
// alignment.
//
// A SyncMap is a chain of HEAD + REGULAR + TAIL fragments laid end-to-end
// across [0, audioLen]. The ABA shifts the boundary points between adjacent
// fragments according to one of several strategies (OFFSET, PERCENT,
// AFTERCURRENT, BEFORENEXT, RATE, RATEAGGRESSIVE), with optional pre- and
// post-processing:
//
//   - NoZero: enforce a minimum duration on every fragment by redistributing
//     time from neighbours.
//   - Nonspeech injection: insert dedicated NONSPEECH fragments wherever a
//     boundary falls inside a long enough silent stretch.
//   - Smoothing: snap HEAD.begin = 0 and TAIL.end = audioLen.
//
// Port of aeneas/adjustboundaryalgorithm.py + aeneas/syncmap/fragmentlist.py.
package aba

import (
	"fmt"
	"slices"

	"github.com/digitalbiblesociety/dido/internal/audiomfcc"
	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

// Algorithm identifies an adjustment strategy.
type Algorithm string

const (
	// AUTO leaves boundaries unchanged.
	AUTO Algorithm = "auto"
	// OFFSET shifts all boundaries by a fixed delta (can be negative).
	OFFSET Algorithm = "offset"
	// PERCENT moves each boundary to N% through the nonspeech interval it falls in.
	PERCENT Algorithm = "percent"
	// AFTERCURRENT moves each boundary N seconds after the start of its nonspeech interval.
	AFTERCURRENT Algorithm = "aftercurrent"
	// BEFORENEXT moves each boundary N seconds before the end of its nonspeech interval.
	BEFORENEXT Algorithm = "beforenext"
	// RATE adjusts boundaries to keep chars/sec below the given maximum.
	RATE Algorithm = "rate"
	// RATEAGGRESSIVE is like RATE but also borrows from the next fragment.
	RATEAGGRESSIVE Algorithm = "rateaggressive"
)

// NonspeechRemove is the sentinel that causes nonspeech fragments to be
// removed entirely instead of inserted into the sync map. Compare against
// Params.NonspeechString.
const NonspeechRemove = ""

// defaultNoZeroDuration is the fallback duration applied to zero-length
// fragments when Params.NoZero is set but Params.NoZeroDuration is unset.
const defaultNoZeroDuration = 0.001

// rateEpsilon is the fudge factor for "is rate above maxRate?" comparisons
// in applyRate. Avoids float instability triggering spurious adjustments
// on fragments already sitting at the rate limit.
const rateEpsilon = 0.001

// Params controls boundary adjustment.
type Params struct {
	// Algorithm is the adjustment strategy.
	Algorithm Algorithm
	// Value is the numeric parameter for OFFSET (seconds), PERCENT (0–100),
	// AFTERCURRENT / BEFORENEXT (seconds), and RATE / RATEAGGRESSIVE (chars/s).
	Value float64
	// NoZero, when true, prevents any fragment from having zero duration.
	NoZero bool
	// NonspeechMinLength is the minimum nonspeech interval length (seconds)
	// to split out as a separate NONSPEECH fragment. Zero means no splitting.
	NonspeechMinLength float64
	// NonspeechString is the text placed in injected NONSPEECH fragments.
	// NonspeechRemove ("") means those fragments are removed instead.
	NonspeechString string
	// NonspeechTolerance is the search tolerance around nonspeech boundaries (seconds).
	NonspeechTolerance float64
	// NoZeroDuration is the duration given to zero-length fragments when
	// NoZero is true. Defaults to defaultNoZeroDuration when unset.
	NoZeroDuration float64
}

// nonspeechMatch pairs a long-enough nonspeech interval with the index of
// the (unique) regular fragment whose end falls inside its tolerance
// shadow. Produced by fragmentsEndingInsideNonspeech.
type nonspeechMatch struct {
	nsi timing.TimeInterval
	idx int
}

// fragItem is the internal mutable representation used during adjustment.
type fragItem struct {
	begin timing.TimeValue
	end   timing.TimeValue
	ftype syncmap.FragmentType
	tf    *text.Fragment
}

func (fi *fragItem) length() timing.TimeValue { return fi.end.Sub(fi.begin) }
func (fi *fragItem) hasZeroLength() bool      { return fi.begin.Equal(fi.end) }
func (fi *fragItem) isHeadOrTail() bool       { return fi.ftype == syncmap.Head || fi.ftype == syncmap.Tail }
func (fi *fragItem) isRegular() bool          { return fi.ftype == syncmap.Regular }
func (fi *fragItem) chars() int {
	if fi.tf == nil {
		return 0
	}
	return fi.tf.Chars()
}
func (fi *fragItem) rate() float64 {
	l := fi.length().Float64()
	if l <= 0 {
		return 0
	}
	return float64(fi.chars()) / l
}
func (fi *fragItem) rateLack(maxRate float64) float64 {
	if fi.rate() <= maxRate {
		return 0
	}
	// Seconds needed to bring rate down to maxRate.
	return float64(fi.chars())/maxRate - fi.length().Float64()
}
func (fi *fragItem) rateSlack(maxRate float64) float64 {
	l := fi.length().Float64()
	maxTime := float64(fi.chars()) / maxRate
	return l - maxTime
}

// Adjust converts DTW boundary frame indices + text fragments into a SyncMap,
// then applies boundary refinement as requested by p.
//
// boundaryIndices are frame indices (relative to realMFCC.Matrix()) produced
// by DTW; len(boundaryIndices) must equal len(textFragments)+1 (one boundary
// between each pair, plus one for the tail).
//
// The returned SyncMap contains HEAD, REGULAR (one per text fragment), and
// TAIL fragments; NONSPEECH fragments are added or removed depending on p.
func Adjust(
	boundaryIndices []int,
	textFragments []*text.Fragment,
	realMFCC *audiomfcc.AudioMFCC,
	p Params,
) (*syncmap.SyncMap, error) {
	if len(boundaryIndices) != len(textFragments)+1 {
		return nil, fmt.Errorf("aba: boundaryIndices length %d must be len(fragments)+1 = %d",
			len(boundaryIndices), len(textFragments)+1)
	}

	frags := buildFragList(boundaryIndices, textFragments, realMFCC)
	if p.NoZero {
		applyNoZero(frags, realMFCC.WindowShift(), p.NoZeroDuration)
	}
	if p.NonspeechMinLength > 0 && realMFCC.Mask() != nil {
		frags = injectNonspeech(frags, realMFCC, p)
	}
	if err := applyAlgorithm(frags, realMFCC, p); err != nil {
		return nil, err
	}
	frags = smoothFragList(frags, realMFCC.AudioLength(), p.NonspeechString)
	return toSyncMap(frags), nil
}

// ─── stage helpers ───────────────────────────────────────────────────────────

// buildFragList converts DTW frame indices + text fragments + the AudioMFCC
// view into the initial mutable fragment list. The result always has shape
// HEAD + len(textFragments) REGULAR + TAIL.
func buildFragList(boundaryIndices []int, textFragments []*text.Fragment, realMFCC *audiomfcc.AudioMFCC) []fragItem {
	mws := realMFCC.WindowShift()
	beginTime := timing.FromFloat64(float64(realMFCC.MiddleBegin()) * mws)
	endTime := timing.FromFloat64(float64(realMFCC.MiddleEnd()) * mws)

	timeValues := make([]timing.TimeValue, len(boundaryIndices)+2)
	timeValues[0] = beginTime
	for i, idx := range boundaryIndices {
		timeValues[i+1] = timing.FromFloat64(float64(idx) * mws)
	}
	timeValues[len(timeValues)-1] = endTime
	return intervalsToFragList(textFragments, timeValues)
}

// applyNoZero ensures no REGULAR fragment has zero length by redistributing
// time from neighbours. HEAD/TAIL are left to smoothFragList.
func applyNoZero(frags []fragItem, windowShift, requestedDur float64) {
	dur := requestedDur
	if dur <= 0 {
		dur = defaultNoZeroDuration
	}
	if windowShift > 0 {
		// Round up to the nearest mws multiple — the alignment grid is
		// quantised in frames, so a sub-frame value would be a lie.
		n := int(dur / windowShift)
		if n == 0 {
			n = 1
		}
		dur = float64(n) * windowShift
	}
	fixZeroLength(frags, timing.FromFloat64(dur), 1, len(frags)-1)
}

// injectNonspeech detects long-enough nonspeech intervals and inserts
// dedicated NONSPEECH fragments at the boundaries that fall inside them,
// shrinking adjacent regulars to make room. Returns the (possibly resized)
// list.
func injectNonspeech(frags []fragItem, realMFCC *audiomfcc.AudioMFCC, p Params) []fragItem {
	nsIntervals := realMFCC.Intervals(false)
	longNS := make([]timing.TimeInterval, 0, len(nsIntervals))
	for _, nsi := range nsIntervals {
		if nsi.Length().Float64() >= p.NonspeechMinLength {
			longNS = append(longNS, nsi)
		}
	}
	if len(longNS) == 0 {
		return frags
	}
	tol := timing.FromFloat64(p.NonspeechTolerance)
	matches := fragmentsEndingInsideNonspeech(frags, longNS, tol, 1, len(frags)-1)
	return injectLongNonspeech(frags, matches, p.NonspeechString)
}

// applyAlgorithm dispatches to the named adjustment strategy in p.Algorithm.
// AUTO and "" are no-ops.
func applyAlgorithm(frags []fragItem, realMFCC *audiomfcc.AudioMFCC, p Params) error {
	switch p.Algorithm {
	case AUTO, "":
		return nil
	case OFFSET:
		applyOffset(frags, timing.FromFloat64(p.Value), realMFCC.AudioLength())
	case PERCENT:
		applyOnNonspeech(frags, realMFCC, p.NonspeechTolerance, func(nsi timing.TimeInterval) timing.TimeValue {
			pct := p.Value / 100.0
			return timing.FromFloat64(nsi.Begin.Float64() + pct*(nsi.End.Float64()-nsi.Begin.Float64()))
		})
	case AFTERCURRENT:
		applyOnNonspeech(frags, realMFCC, p.NonspeechTolerance, func(nsi timing.TimeInterval) timing.TimeValue {
			delay := p.Value
			if delay < 0 {
				delay = 0
			}
			t := nsi.Begin.Float64() + delay
			if t > nsi.End.Float64() {
				t = nsi.End.Float64()
			}
			return timing.FromFloat64(t)
		})
	case BEFORENEXT:
		applyOnNonspeech(frags, realMFCC, p.NonspeechTolerance, func(nsi timing.TimeInterval) timing.TimeValue {
			delay := p.Value
			if delay < 0 {
				delay = 0
			}
			t := nsi.End.Float64() - delay
			if t < nsi.Begin.Float64() {
				t = nsi.Begin.Float64()
			}
			return timing.FromFloat64(t)
		})
	case RATE:
		applyRate(frags, p.Value, false)
	case RATEAGGRESSIVE:
		applyRate(frags, p.Value, true)
	default:
		return fmt.Errorf("aba: unknown algorithm %q", p.Algorithm)
	}
	return nil
}

// toSyncMap converts the internal fragment list to the public SyncMap.
func toSyncMap(frags []fragItem) *syncmap.SyncMap {
	sm := syncmap.NewSyncMap()
	for _, fi := range frags {
		sm.Add(syncmap.NewSyncMapFragment(fi.tf, fi.begin, fi.end, fi.ftype))
	}
	return sm
}

// ─── internal fragment list helpers ──────────────────────────────────────────

func intervalsToFragList(textFragments []*text.Fragment, timeValues []timing.TimeValue) []fragItem {
	// timeValues has len = 2 + len(textFragments):
	//   [0]         = begin of HEAD
	//   [1]         = end of HEAD / begin of REGULAR[0]
	//   [2..n-2]    = boundaries between REGULAR fragments
	//   [n-1]       = end of REGULAR[last] / begin of TAIL
	//   [n]         = end of TAIL
	n := len(timeValues)
	frags := make([]fragItem, 0, n-1)
	frags = append(frags, fragItem{
		begin: timeValues[0], end: timeValues[1], ftype: syncmap.Head,
		tf: &text.Fragment{Identifier: "HEAD"},
	})
	for i, tf := range textFragments {
		frags = append(frags, fragItem{
			begin: timeValues[i+1], end: timeValues[i+2], ftype: syncmap.Regular, tf: tf,
		})
	}
	frags = append(frags, fragItem{
		begin: timeValues[n-2], end: timeValues[n-1], ftype: syncmap.Tail,
		tf: &text.Fragment{Identifier: "TAIL"},
	})
	return frags
}

// fixZeroLength scans [minIdx, maxIdx) and redistributes time from
// neighbouring fragments so that no zero-length fragment remains.
func fixZeroLength(frags []fragItem, dur timing.TimeValue, minIdx, maxIdx int) {
	if maxIdx > len(frags) {
		maxIdx = len(frags)
	}
	i := minIdx
	for i < maxIdx {
		if !frags[i].hasZeroLength() {
			i++
			continue
		}
		// Find the run of zero/short-length fragments starting at i.
		slack := dur
		moves := []int{i}
		j := i + 1
		for j < maxIdx && frags[j].length().Less(slack) {
			if frags[j].hasZeroLength() {
				slack = slack.Add(dur)
			}
			moves = append(moves, j)
			j++
		}
		if j < maxIdx {
			// Steal `slack` from frags[j] by shifting its begin forward.
			newBegin := frags[j].begin.Add(slack)
			if newBegin.Less(frags[j].end) || newBegin.Equal(frags[j].end) {
				frags[j].begin = newBegin
				cur := frags[j].begin
				for k := len(moves) - 1; k >= 0; k-- {
					idx := moves[k]
					frags[idx].end = cur
					if frags[idx].hasZeroLength() {
						frags[idx].begin = cur.Sub(dur)
					}
					cur = frags[idx].begin
				}
			}
		} else {
			// j == maxIdx: spill the run into the post-list end zone.
			cur := frags[j-1].end.Add(slack)
			for k := len(moves) - 1; k >= 0; k-- {
				idx := moves[k]
				frags[idx].end = cur
				if frags[idx].hasZeroLength() {
					frags[idx].begin = cur.Sub(dur)
				}
				cur = frags[idx].begin
			}
		}
		i = j
	}
}

// moveTransitionPoint moves the boundary between frags[idx] and frags[idx+1]
// to val. No-op if val is past frags[idx+1].end or if the two fragments
// aren't currently adjacent (their shared boundary doesn't match).
func moveTransitionPoint(frags []fragItem, idx int, val timing.TimeValue) {
	if idx < 0 || idx > len(frags)-3 {
		return
	}
	if val.Greater(frags[idx+1].end) {
		return
	}
	if !frags[idx].end.Equal(frags[idx+1].begin) {
		return
	}
	frags[idx].end = val
	frags[idx+1].begin = val
}

// fragmentsEndingInsideNonspeech returns matches where a fragment's end
// falls inside a nonspeech interval's tolerance shadow. Multi-fragment
// matches on the same nonspeech are silently dropped — inserting a
// NONSPEECH for an ambiguous boundary would corrupt the alignment.
func fragmentsEndingInsideNonspeech(
	frags []fragItem,
	nsIntervals []timing.TimeInterval,
	tolerance timing.TimeValue,
	minIdx, maxIdx int,
) []nonspeechMatch {
	type bucket struct {
		nsi  timing.TimeInterval
		idxs []int
	}
	buckets := make([]bucket, len(nsIntervals))
	for i, nsi := range nsIntervals {
		buckets[i] = bucket{nsi: nsi}
	}
	listEnd := frags[len(frags)-1].end
	nsiIndex := 0
	fragIndex := minIdx
	for nsiIndex < len(nsIntervals) && fragIndex < maxIdx {
		nsi := nsIntervals[nsiIndex]
		if nsi.End.Greater(listEnd) {
			break
		}
		shadowBegin := nsi.Begin.Sub(tolerance)
		if shadowBegin.Less(timing.Zero) {
			shadowBegin = timing.Zero
		}
		shadowEnd := nsi.End.Add(tolerance)

		fi := frags[fragIndex]
		if fi.isHeadOrTail() {
			fragIndex++
			continue
		}
		if fi.end.Less(shadowBegin) || fi.end.Equal(shadowBegin) {
			fragIndex++
			continue
		}
		if fi.end.Greater(shadowEnd) {
			nsiIndex++
			continue
		}
		// fi.end is inside the shadow.
		if fi.begin.Greater(shadowBegin) || fi.begin.Equal(shadowBegin) {
			// Entire fragment inside the shadow → ambiguous, drop the bucket.
			buckets[nsiIndex].idxs = nil
			nsiIndex++
			fragIndex++
		} else {
			buckets[nsiIndex].idxs = append(buckets[nsiIndex].idxs, fragIndex)
			fragIndex++
		}
	}
	out := make([]nonspeechMatch, 0, len(buckets))
	for _, b := range buckets {
		if len(b.idxs) == 1 {
			out = append(out, nonspeechMatch{nsi: b.nsi, idx: b.idxs[0]})
		}
	}
	return out
}

// injectLongNonspeech inserts a NONSPEECH fragment for every match,
// adjusting the matching fragment + its right neighbour to make room. The
// returned slice is re-sorted in ascending begin order.
func injectLongNonspeech(frags []fragItem, matches []nonspeechMatch, nsString string) []fragItem {
	if len(matches) == 0 {
		return frags
	}
	var lines []string
	if nsString != NonspeechRemove {
		lines = []string{nsString}
	}
	// First pass: make room.
	for _, m := range matches {
		frags[m.idx].end = m.nsi.Begin
		if m.idx+1 < len(frags) {
			frags[m.idx+1].begin = m.nsi.End
		}
	}
	// Second pass: append the NONSPEECH fragments.
	for i, m := range matches {
		frags = append(frags, fragItem{
			begin: m.nsi.Begin,
			end:   m.nsi.End,
			ftype: syncmap.NonSpeech,
			tf:    &text.Fragment{Identifier: fmt.Sprintf("n%06d", i+1), Lines: lines},
		})
	}
	slices.SortFunc(frags, func(a, b fragItem) int {
		switch {
		case a.begin.Less(b.begin):
			return -1
		case b.begin.Less(a.begin):
			return 1
		default:
			return 0
		}
	})
	return frags
}

// applyOffset shifts every fragment by offset, clamping begin to 0 and
// end to audioLen. Note: a one-sided clamp silently changes duration —
// Python's aeneas matches this.
func applyOffset(frags []fragItem, offset, audioLen timing.TimeValue) {
	for i := range frags {
		b := frags[i].begin.Add(offset)
		e := frags[i].end.Add(offset)
		if b.Less(timing.Zero) {
			b = timing.Zero
		}
		if e.Greater(audioLen) {
			e = audioLen
		}
		// A large offset can push begin past the clamped end (or end below
		// 0), leaving begin > end — which panics in NewTimeInterval.
		// Collapse to a zero-length boundary instead of crashing the run.
		if e.Less(timing.Zero) {
			e = timing.Zero
		}
		if b.Greater(e) {
			b = e
		}
		frags[i].begin = b
		frags[i].end = e
	}
}

// applyOnNonspeech moves each boundary that falls inside a nonspeech
// interval's shadow to the point chosen by newTimeFn.
func applyOnNonspeech(frags []fragItem, realMFCC *audiomfcc.AudioMFCC, toleranceSec float64, newTimeFn func(timing.TimeInterval) timing.TimeValue) {
	if realMFCC.Mask() == nil {
		return
	}
	nsIntervals := realMFCC.Intervals(false)
	tol := timing.FromFloat64(toleranceSec)
	matches := fragmentsEndingInsideNonspeech(frags, nsIntervals, tol, 1, len(frags)-1)
	for _, m := range matches {
		moveTransitionPoint(frags, m.idx, newTimeFn(m.nsi))
	}
}

// applyRate enforces maxRate (chars/sec) by transferring time from
// neighbouring fragments. When aggressive is true, both previous and next
// neighbours can donate; otherwise only the previous one can.
func applyRate(frags []fragItem, maxRate float64, aggressive bool) {
	for i := range frags {
		if !frags[i].isRegular() {
			continue
		}
		if frags[i].rate() < maxRate+rateEpsilon {
			continue
		}
		fixFragmentRate(frags, i, maxRate, aggressive)
	}
}

func fixFragmentRate(frags []fragItem, idx int, maxRate float64, aggressive bool) {
	if fixPair(frags, idx, idx-1, maxRate) {
		return
	}
	if aggressive {
		fixPair(frags, idx, idx+1, maxRate)
	}
}

// fixPair tries to transfer slack from donorIdx to currentIdx. Returns
// true iff the transfer fully covered the lack — the caller relies on
// that to short-circuit the aggressive pass.
func fixPair(frags []fragItem, currentIdx, donorIdx int, maxRate float64) bool {
	if currentIdx < 0 || currentIdx >= len(frags) || donorIdx < 0 || donorIdx >= len(frags) {
		return false
	}
	// Donor must be an immediate neighbour.
	if currentIdx-donorIdx != 1 && donorIdx-currentIdx != 1 {
		return false
	}
	cur := frags[currentIdx]
	donor := frags[donorIdx]
	if cur.rate() <= maxRate {
		return true
	}
	donorIsPrev := donorIdx < currentIdx
	if donorIsPrev {
		if !cur.begin.Equal(donor.end) {
			return false
		}
	} else {
		if !donor.begin.Equal(cur.end) {
			return false
		}
	}
	lack := cur.rateLack(maxRate)
	slack := donor.rateSlack(maxRate)
	if slack <= 0 {
		return false
	}
	effective := lack
	if slack < effective {
		effective = slack
	}
	effectiveTV := timing.FromFloat64(effective)
	if donorIsPrev {
		moveTransitionPoint(frags, donorIdx, donor.end.Sub(effectiveTV))
	} else {
		moveTransitionPoint(frags, currentIdx, cur.end.Add(effectiveTV))
	}
	return effective >= lack
}

// smoothFragList snaps HEAD.begin to 0 and TAIL.end to audioLen, then
// filters out NONSPEECH fragments that should be removed (either because
// the caller asked for it via NonspeechRemove, or because they ended up
// zero-length after adjustment). Returns the filtered slice.
func smoothFragList(frags []fragItem, audioLen timing.TimeValue, nsString string) []fragItem {
	if len(frags) > 0 {
		frags[0].begin = timing.Zero
	}
	if len(frags) > 1 {
		frags[len(frags)-1].end = audioLen
	}
	remove := nsString == NonspeechRemove
	out := frags[:0]
	for _, fi := range frags {
		if fi.ftype == syncmap.NonSpeech && (remove || fi.hasZeroLength()) {
			continue
		}
		out = append(out, fi)
	}
	return out
}
