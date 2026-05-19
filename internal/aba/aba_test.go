package aba

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/audiomfcc"
	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/timing"
	"github.com/digitalbiblesociety/dido/internal/vad"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func tv(s float64) timing.TimeValue { return timing.FromFloat64(s) }

// makeAudioMFCC builds a synthetic AudioMFCC with the given energy row.
func makeAudioMFCC(energy []float64, windowShift float64) *audiomfcc.AudioMFCC {
	m := make([][]float64, 2)
	m[0] = make([]float64, len(energy))
	copy(m[0], energy)
	m[1] = make([]float64, len(energy))
	return audiomfcc.New(m, windowShift)
}

// simpleFragments returns n text fragments labelled f000001 … f000n.
func simpleFragments(n int) []*text.Fragment {
	frags := make([]*text.Fragment, n)
	for i := range frags {
		frags[i] = &text.Fragment{
			Identifier: fmt.Sprintf("f%06d", i+1),
			Lines:      []string{"hello world"},
		}
	}
	return frags
}

// regular builds a REGULAR fragItem with text of `chars` characters,
// laid between [begin, end] seconds.
func regular(begin, end float64, chars int) fragItem {
	return fragItem{
		begin: tv(begin), end: tv(end), ftype: syncmap.Regular,
		tf: &text.Fragment{Lines: []string{strings.Repeat("a", chars)}},
	}
}

func head(end float64) fragItem {
	return fragItem{begin: tv(0), end: tv(end), ftype: syncmap.Head, tf: &text.Fragment{Identifier: "HEAD"}}
}

func tail(begin, end float64) fragItem {
	return fragItem{begin: tv(begin), end: tv(end), ftype: syncmap.Tail, tf: &text.Fragment{Identifier: "TAIL"}}
}

// near reports whether two seconds-floats agree to 1e-9.
func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// ─── Adjust (public API) ─────────────────────────────────────────────────────

func TestAdjustAUTO(t *testing.T) {
	boundaries := []int{2, 4, 6, 8, 10}
	frags := simpleFragments(4)
	energy := make([]float64, 12)
	for i := range energy {
		energy[i] = 1.0
	}
	a := makeAudioMFCC(energy, 0.040)
	a.SetMiddle(0, 10)

	sm, err := Adjust(boundaries, frags, a, Params{Algorithm: AUTO})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(sm.Fragments()); got != 6 {
		t.Fatalf("want 6 fragments (HEAD + 4 REGULAR + TAIL), got %d", got)
	}
}

func TestAdjustOffset(t *testing.T) {
	boundaries := []int{2, 4, 6, 8, 10}
	frags := simpleFragments(4)
	energy := make([]float64, 12)
	for i := range energy {
		energy[i] = 1.0
	}
	a := makeAudioMFCC(energy, 0.040)
	a.SetMiddle(0, 10)

	sm, err := Adjust(boundaries, frags, a, Params{Algorithm: OFFSET, Value: 0.040})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(sm.Fragments()); got != 6 {
		t.Fatalf("want 6, got %d", got)
	}
}

func TestAdjustBadBoundaryCount(t *testing.T) {
	frags := simpleFragments(3)
	energy := make([]float64, 10)
	a := makeAudioMFCC(energy, 0.040)
	_, err := Adjust([]int{2, 4, 6}, frags, a, Params{Algorithm: AUTO})
	if err == nil {
		t.Error("expected error for mismatched boundary count")
	}
}

func TestAdjustUnknownAlgorithm(t *testing.T) {
	boundaries := []int{2, 4}
	frags := simpleFragments(1)
	energy := make([]float64, 6)
	a := makeAudioMFCC(energy, 0.040)
	a.SetMiddle(0, 4)
	_, err := Adjust(boundaries, frags, a, Params{Algorithm: "bogus"})
	if err == nil {
		t.Error("expected error for unknown algorithm")
	}
}

func TestAdjustNonspeechPercent(t *testing.T) {
	energy := []float64{5, 5, 5, 5, 0, 0, 0, 5, 5, 5, 5, 5}
	a := makeAudioMFCC(energy, 0.040)
	a.SetMiddle(0, 12)
	a.RunVAD(vad.Params{LogEnergyThreshold: 2.0, MinNonspeechLength: 3})

	boundaries := []int{5, 10, 12}
	frags := simpleFragments(2)
	sm, err := Adjust(boundaries, frags, a, Params{
		Algorithm:          PERCENT,
		Value:              50.0,
		NonspeechTolerance: 0.040,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(sm.Fragments()); got < 3 {
		t.Fatalf("want at least 3 fragments, got %d", got)
	}
}

// ─── stage helpers ───────────────────────────────────────────────────────────

func TestFixZeroLength(t *testing.T) {
	frags := []fragItem{
		{begin: tv(0), end: tv(0.080), ftype: syncmap.Head},
		{begin: tv(0.080), end: tv(0.080), ftype: syncmap.Regular},
		{begin: tv(0.080), end: tv(0.200), ftype: syncmap.Regular},
		{begin: tv(0.200), end: tv(0.280), ftype: syncmap.Tail},
	}
	fixZeroLength(frags, tv(0.040), 1, 3)
	if frags[1].hasZeroLength() {
		t.Errorf("fragment 1 should no longer be zero-length: %+v", frags[1])
	}
}

func TestMoveTransitionPoint(t *testing.T) {
	frags := []fragItem{
		{begin: tv(0), end: tv(0.100)},
		{begin: tv(0.100), end: tv(0.200)},
		{begin: tv(0.200), end: tv(0.300)},
	}
	moveTransitionPoint(frags, 0, tv(0.120))
	if !near(frags[0].end.Float64(), 0.120) {
		t.Errorf("frags[0].end = %.3f, want 0.120", frags[0].end.Float64())
	}
	if !near(frags[1].begin.Float64(), 0.120) {
		t.Errorf("frags[1].begin = %.3f, want 0.120", frags[1].begin.Float64())
	}
}

func TestMoveTransitionPointNoOpsOnNonAdjacent(t *testing.T) {
	frags := []fragItem{
		{begin: tv(0), end: tv(0.100)},
		{begin: tv(0.150), end: tv(0.200)}, // gap: 0.100 != 0.150
		{begin: tv(0.200), end: tv(0.300)},
	}
	moveTransitionPoint(frags, 0, tv(0.120))
	if !near(frags[0].end.Float64(), 0.100) {
		t.Errorf("non-adjacent boundary should not have moved; got %.3f", frags[0].end.Float64())
	}
}

func TestSmoothFragList_SnapsHeadAndTail(t *testing.T) {
	audioLen := tv(1.0)
	frags := []fragItem{
		{begin: tv(0.1), end: tv(0.3), ftype: syncmap.Head},
		{begin: tv(0.3), end: tv(0.7), ftype: syncmap.Regular},
		{begin: tv(0.7), end: tv(0.9), ftype: syncmap.Tail},
	}
	frags = smoothFragList(frags, audioLen, NonspeechRemove)
	if frags[0].begin.Float64() != 0 {
		t.Errorf("HEAD.begin should be 0, got %.3f", frags[0].begin.Float64())
	}
	if frags[len(frags)-1].end.Float64() != 1.0 {
		t.Errorf("TAIL.end should be 1.0, got %.3f", frags[len(frags)-1].end.Float64())
	}
}

func TestSmoothFragList_KeepsNamedNonspeech(t *testing.T) {
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.4, 5),
		{begin: tv(0.4), end: tv(0.5), ftype: syncmap.NonSpeech, tf: &text.Fragment{Lines: []string{"(silence)"}}},
		regular(0.5, 0.9, 5),
		tail(0.9, 1.0),
	}
	frags = smoothFragList(frags, tv(1.0), "(silence)")
	if got := len(frags); got != 5 {
		t.Fatalf("named NONSPEECH should survive smoothing: want 5 frags, got %d", got)
	}
	if frags[2].ftype != syncmap.NonSpeech {
		t.Errorf("NONSPEECH not at index 2: types = %v", types(frags))
	}
}

func TestSmoothFragList_RemovesNonspeechWhenRequested(t *testing.T) {
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.4, 5),
		{begin: tv(0.4), end: tv(0.5), ftype: syncmap.NonSpeech, tf: &text.Fragment{Lines: []string{"(silence)"}}},
		regular(0.5, 0.9, 5),
		tail(0.9, 1.0),
	}
	frags = smoothFragList(frags, tv(1.0), NonspeechRemove)
	if got := len(frags); got != 4 {
		t.Fatalf("NONSPEECH should be removed: want 4 frags, got %d", got)
	}
}

func TestSmoothFragList_RemovesZeroLengthNonspeechEvenWhenNamed(t *testing.T) {
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.5, 5),
		{begin: tv(0.5), end: tv(0.5), ftype: syncmap.NonSpeech, tf: &text.Fragment{Lines: []string{"(silence)"}}},
		regular(0.5, 0.9, 5),
		tail(0.9, 1.0),
	}
	frags = smoothFragList(frags, tv(1.0), "(silence)")
	if got := len(frags); got != 4 {
		t.Fatalf("zero-length NONSPEECH should be removed: want 4 frags, got %d", got)
	}
}

// ─── applyOffset ─────────────────────────────────────────────────────────────

func TestApplyOffset_ShiftsAll(t *testing.T) {
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.5, 5),
		tail(0.5, 1.0),
	}
	applyOffset(frags, tv(0.05), tv(1.0))
	if !near(frags[0].end.Float64(), 0.15) {
		t.Errorf("HEAD.end after +0.05 offset: got %.3f, want 0.15", frags[0].end.Float64())
	}
	if !near(frags[1].begin.Float64(), 0.15) {
		t.Errorf("R[0].begin: got %.3f, want 0.15", frags[1].begin.Float64())
	}
}

func TestApplyOffset_ClampsEndToAudioLen(t *testing.T) {
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.5, 5),
		tail(0.5, 1.0),
	}
	applyOffset(frags, tv(0.8), tv(1.0))
	if !near(frags[2].end.Float64(), 1.0) {
		t.Errorf("TAIL.end should clamp to audioLen=1.0; got %.3f", frags[2].end.Float64())
	}
}

func TestApplyOffset_ClampsBeginToZero(t *testing.T) {
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.5, 5),
		tail(0.5, 1.0),
	}
	applyOffset(frags, tv(-0.5), tv(1.0))
	if !near(frags[0].begin.Float64(), 0) {
		t.Errorf("HEAD.begin should clamp to 0; got %.3f", frags[0].begin.Float64())
	}
}

// ─── applyRate ───────────────────────────────────────────────────────────────

// Setup: HEAD + REGULAR(5 chars, 1.0s, rate 5) + REGULAR(20 chars, 1.0s, rate 20) + TAIL.
// maxRate = 10 → frag2 over rate, frag1 has 0.5s of slack.
// fixPair takes 0.5s from frag1, leaving frag2 at rate ~13.3 (still over).
func TestApplyRate_PrevDonatesPartialSlack(t *testing.T) {
	frags := []fragItem{
		head(0),
		regular(0, 1.0, 5),
		regular(1.0, 2.0, 20),
		tail(2.0, 2.0),
	}
	applyRate(frags, 10, false)

	if !near(frags[1].end.Float64(), 0.5) {
		t.Errorf("frag1 should shrink end to 0.5 (donating 0.5s); got %.3f", frags[1].end.Float64())
	}
	if !near(frags[2].begin.Float64(), 0.5) {
		t.Errorf("frag2 should expand begin to 0.5; got %.3f", frags[2].begin.Float64())
	}
	// Final rate of frag2: 20 / 1.5 = 13.33; still above maxRate but the
	// best we can do without borrowing from the right.
	if frags[2].rate() <= 10 {
		t.Errorf("frag2 rate should still exceed maxRate after partial donation; got %.3f", frags[2].rate())
	}
}

func TestApplyRate_AggressiveBorrowsFromNext(t *testing.T) {
	// Same setup as above, plus a third REGULAR with 0.5s of slack.
	// Conservative pass shrinks frag1 by 0.5s; aggressive then shrinks frag3
	// by 0.5s. Final frag2 = [0.5, 2.5], length 2.0, rate = 10 (at limit).
	frags := []fragItem{
		head(0),
		regular(0, 1.0, 5),
		regular(1.0, 2.0, 20),
		regular(2.0, 3.0, 5),
		tail(3.0, 3.0),
	}
	applyRate(frags, 10, true)

	if !near(frags[2].begin.Float64(), 0.5) {
		t.Errorf("frag2.begin should be 0.5 after prev donation; got %.3f", frags[2].begin.Float64())
	}
	if !near(frags[2].end.Float64(), 2.5) {
		t.Errorf("frag2.end should be 2.5 after aggressive next donation; got %.3f", frags[2].end.Float64())
	}
	if !near(frags[2].rate(), 10) {
		t.Errorf("frag2 rate should land at maxRate=10; got %.3f", frags[2].rate())
	}
}

func TestApplyRate_NonAggressiveDoesNotBorrowFromNext(t *testing.T) {
	// Frag1 has zero slack (rate already at maxRate); only frag3 could help.
	// Non-aggressive mode must leave frag2 over.
	frags := []fragItem{
		head(0),
		regular(0, 1.0, 10), // rate 10 = maxRate → zero slack
		regular(1.0, 2.0, 20),
		regular(2.0, 3.0, 5),
		tail(3.0, 3.0),
	}
	applyRate(frags, 10, false)
	if !near(frags[1].end.Float64(), 1.0) {
		t.Errorf("frag1 should be untouched (no slack); got end %.3f", frags[1].end.Float64())
	}
	if !near(frags[2].end.Float64(), 2.0) {
		t.Errorf("non-aggressive should not have touched frag2.end; got %.3f", frags[2].end.Float64())
	}
}

func TestApplyRate_BelowMaxRateUntouched(t *testing.T) {
	frags := []fragItem{
		head(0),
		regular(0, 1.0, 5), // rate 5 < 10
		regular(1.0, 2.0, 5),
		tail(2.0, 2.0),
	}
	applyRate(frags, 10, false)
	if !near(frags[1].end.Float64(), 1.0) || !near(frags[2].end.Float64(), 2.0) {
		t.Error("nothing should move when all rates are under the limit")
	}
}

// ─── fragmentsEndingInsideNonspeech ──────────────────────────────────────────

func TestFragmentsEndingInsideNonspeech_SingleMatch(t *testing.T) {
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.5, 5),
		regular(0.5, 1.0, 5),
		tail(1.0, 1.2),
	}
	ns := []timing.TimeInterval{
		{Begin: tv(0.45), End: tv(0.55)},
	}
	got := fragmentsEndingInsideNonspeech(frags, ns, tv(0.05), 1, len(frags)-1)
	if len(got) != 1 || got[0].idx != 1 {
		t.Fatalf("want one match at idx=1; got %+v", got)
	}
}

func TestFragmentsEndingInsideNonspeech_AmbiguousMultiMatchDropped(t *testing.T) {
	// Two fragment-ends fall inside the same nonspeech shadow → dropped.
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.48, 5),
		regular(0.48, 0.52, 5),
		regular(0.52, 1.0, 5),
		tail(1.0, 1.2),
	}
	ns := []timing.TimeInterval{
		{Begin: tv(0.45), End: tv(0.55)},
	}
	got := fragmentsEndingInsideNonspeech(frags, ns, tv(0.05), 1, len(frags)-1)
	if len(got) != 0 {
		t.Errorf("ambiguous multi-match should be dropped; got %+v", got)
	}
}

func TestFragmentsEndingInsideNonspeech_FragSpansShadow(t *testing.T) {
	// Fragment fully inside the shadow → nsi invalidated, no match.
	frags := []fragItem{
		head(0.1),
		regular(0.46, 0.54, 5),
		regular(0.54, 1.0, 5),
		tail(1.0, 1.2),
	}
	ns := []timing.TimeInterval{
		{Begin: tv(0.45), End: tv(0.55)},
	}
	got := fragmentsEndingInsideNonspeech(frags, ns, tv(0.05), 1, len(frags)-1)
	if len(got) != 0 {
		t.Errorf("fragment fully inside shadow should not match; got %+v", got)
	}
}

// ─── injectLongNonspeech ─────────────────────────────────────────────────────

func TestInjectLongNonspeech_OneMatchInserts(t *testing.T) {
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.5, 5),
		regular(0.5, 1.0, 5),
		tail(1.0, 1.2),
	}
	matches := []nonspeechMatch{
		{nsi: timing.TimeInterval{Begin: tv(0.45), End: tv(0.55)}, idx: 1},
	}
	got := injectLongNonspeech(frags, matches, "(silence)")
	if len(got) != len(frags)+1 {
		t.Fatalf("want %d frags after injection; got %d", len(frags)+1, len(got))
	}
	// Find the NONSPEECH fragment.
	var ns *fragItem
	for i := range got {
		if got[i].ftype == syncmap.NonSpeech {
			ns = &got[i]
			break
		}
	}
	if ns == nil {
		t.Fatalf("no NONSPEECH fragment inserted; types = %v", types(got))
	}
	if !near(ns.begin.Float64(), 0.45) || !near(ns.end.Float64(), 0.55) {
		t.Errorf("NONSPEECH window wrong: [%.3f, %.3f]", ns.begin.Float64(), ns.end.Float64())
	}
}

func TestInjectLongNonspeech_NoMatchesPassthrough(t *testing.T) {
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.5, 5),
		tail(0.5, 1.0),
	}
	got := injectLongNonspeech(frags, nil, "(silence)")
	if len(got) != len(frags) {
		t.Errorf("no matches should pass through unchanged; got len %d, want %d", len(got), len(frags))
	}
}

func TestInjectLongNonspeech_RemoveSentinelOmitsLines(t *testing.T) {
	frags := []fragItem{
		head(0.1),
		regular(0.1, 0.5, 5),
		regular(0.5, 1.0, 5),
		tail(1.0, 1.2),
	}
	matches := []nonspeechMatch{
		{nsi: timing.TimeInterval{Begin: tv(0.45), End: tv(0.55)}, idx: 1},
	}
	got := injectLongNonspeech(frags, matches, NonspeechRemove)
	// Even with the "remove" sentinel, injection still inserts the NONSPEECH
	// fragment — smoothFragList is what filters them out later. The
	// difference is that the inserted fragment carries no text lines.
	for _, fi := range got {
		if fi.ftype == syncmap.NonSpeech {
			if len(fi.tf.Lines) != 0 {
				t.Errorf("NonspeechRemove should leave lines empty; got %v", fi.tf.Lines)
			}
			return
		}
	}
	t.Error("no NONSPEECH fragment found")
}

// ─── helpers for tests ───────────────────────────────────────────────────────

func types(frags []fragItem) []syncmap.FragmentType {
	out := make([]syncmap.FragmentType, len(frags))
	for i, f := range frags {
		out[i] = f.ftype
	}
	return out
}
