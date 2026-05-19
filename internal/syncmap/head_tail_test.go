package syncmap

import (
	"testing"

	"github.com/digitalbiblesociety/dido/internal/timing"
)

// headTailFixture builds a flat sync map shaped as the alignment
// pipeline produces it: HEAD, three Regular fragments, TAIL — with
// contiguous intervals that span [0, 10) seconds.
func headTailFixture() *SyncMap {
	sm := NewSyncMap()
	add := func(id string, begin, end float64, ft FragmentType) {
		tf := &stubTextFragment{id: id, text: id, lines: []string{id}}
		sm.Add(NewSyncMapFragment(tf,
			timing.FromFloat64(begin), timing.FromFloat64(end), ft))
	}
	add("HEAD", 0, 1, Head)
	add("a", 1, 4, Regular)
	add("b", 4, 6, Regular)
	add("c", 6, 9, Regular)
	add("TAIL", 9, 10, Tail)
	return sm
}

func ids(frags []*SyncMapFragment) []string {
	out := make([]string, len(frags))
	for i, f := range frags {
		out[i] = f.Identifier()
	}
	return out
}

func TestApplyHeadTailFormat_AddIsNoOp(t *testing.T) {
	for _, htf := range []HeadTailFormat{"", HeadTailAdd, "bogus"} {
		sm := headTailFixture()
		got := sm.ApplyHeadTailFormat(htf)
		if got != sm {
			t.Errorf("htf=%q: expected same SyncMap pointer", htf)
		}
		want := []string{"HEAD", "a", "b", "c", "TAIL"}
		if g := ids(sm.Fragments()); !equal(g, want) {
			t.Errorf("htf=%q: ids=%v, want %v", htf, g, want)
		}
	}
}

func TestApplyHeadTailFormat_HiddenDropsBothEnds(t *testing.T) {
	sm := headTailFixture()
	sm.ApplyHeadTailFormat(HeadTailHidden)
	want := []string{"a", "b", "c"}
	if g := ids(sm.Fragments()); !equal(g, want) {
		t.Errorf("hidden: ids=%v, want %v", g, want)
	}
	// The Regular intervals must NOT be stretched in hidden mode.
	first := sm.Fragments()[0]
	if first.Begin().Float64() != 1.0 {
		t.Errorf("hidden: first.Begin=%v, want 1.0 (HEAD must not be absorbed)", first.Begin().Float64())
	}
	last := sm.Fragments()[len(sm.Fragments())-1]
	if last.End().Float64() != 9.0 {
		t.Errorf("hidden: last.End=%v, want 9.0 (TAIL must not be absorbed)", last.End().Float64())
	}
}

func TestApplyHeadTailFormat_StretchAbsorbsEnds(t *testing.T) {
	sm := headTailFixture()
	sm.ApplyHeadTailFormat(HeadTailStretch)
	want := []string{"a", "b", "c"}
	if g := ids(sm.Fragments()); !equal(g, want) {
		t.Errorf("stretch: ids=%v, want %v", g, want)
	}
	first := sm.Fragments()[0]
	if first.Begin().Float64() != 0.0 {
		t.Errorf("stretch: first.Begin=%v, want 0.0", first.Begin().Float64())
	}
	last := sm.Fragments()[len(sm.Fragments())-1]
	if last.End().Float64() != 10.0 {
		t.Errorf("stretch: last.End=%v, want 10.0", last.End().Float64())
	}
}

func TestApplyHeadTailFormat_OnlyTouchesEnds(t *testing.T) {
	sm := headTailFixture()
	sm.ApplyHeadTailFormat(HeadTailStretch)
	mid := sm.Fragments()[1]
	if mid.Identifier() != "b" {
		t.Fatalf("middle fragment shifted: got %q", mid.Identifier())
	}
	if mid.Begin().Float64() != 4.0 || mid.End().Float64() != 6.0 {
		t.Errorf("middle fragment intervals modified: [%v, %v], want [4, 6]",
			mid.Begin().Float64(), mid.End().Float64())
	}
}

func TestApplyHeadTailFormat_NilSafe(t *testing.T) {
	var sm *SyncMap
	if got := sm.ApplyHeadTailFormat(HeadTailHidden); got != nil {
		t.Errorf("nil receiver returned non-nil")
	}
	empty := &SyncMap{}
	if got := empty.ApplyHeadTailFormat(HeadTailHidden); got != empty {
		t.Errorf("empty SyncMap (no Tree) should be returned as-is")
	}
}

func TestApplyHeadTailFormat_NoHeadTailIsNoOp(t *testing.T) {
	// A sync map without HEAD/TAIL (e.g. when SD detection was skipped)
	// should pass through unchanged under any htf value.
	sm := NewSyncMap()
	for _, id := range []string{"a", "b", "c"} {
		tf := &stubTextFragment{id: id, lines: []string{id}}
		sm.Add(NewSyncMapFragment(tf,
			timing.FromFloat64(0), timing.FromFloat64(1), Regular))
	}
	for _, htf := range []HeadTailFormat{HeadTailHidden, HeadTailStretch, HeadTailAdd} {
		sm.ApplyHeadTailFormat(htf)
		if g := ids(sm.Fragments()); !equal(g, []string{"a", "b", "c"}) {
			t.Errorf("htf=%q: regular-only fragments altered: %v", htf, g)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
