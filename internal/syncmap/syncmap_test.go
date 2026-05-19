package syncmap

import (
	"testing"

	"github.com/digitalbiblesociety/dido/internal/timing"
)

// stubTextFragment is a minimal implementation of TextFragment used by
// the tests to avoid pulling internal/text into the syncmap test scope
// (and the import cycle that would create).
type stubTextFragment struct {
	id    string
	text  string
	lines []string
	lang  string
}

func (s *stubTextFragment) Text() string          { return s.text }
func (s *stubTextFragment) GetIdentifier() string { return s.id }
func (s *stubTextFragment) GetLines() []string    { return s.lines }
func (s *stubTextFragment) GetLanguage() string   { return s.lang }

func newFrag(id, text, lang string) *SyncMapFragment {
	tf := &stubTextFragment{id: id, text: text, lang: lang, lines: []string{text}}
	return NewSyncMapFragment(tf,
		timing.FromFloat64(0), timing.FromFloat64(1), Regular)
}

func TestFragmentsAndLeavesFlat(t *testing.T) {
	sm := NewSyncMap()
	sm.Add(newFrag("a", "first", "eng"))
	sm.Add(newFrag("b", "second", "eng"))
	sm.Add(newFrag("c", "third", "eng"))

	frags := sm.Fragments()
	if len(frags) != 3 {
		t.Fatalf("Fragments len: got %d, want 3", len(frags))
	}
	leaves := sm.Leaves()
	if len(leaves) != 3 {
		t.Fatalf("Leaves len: got %d, want 3", len(leaves))
	}
	for i := range frags {
		if frags[i] != leaves[i] {
			t.Errorf("flat map: Fragments[%d] != Leaves[%d]", i, i)
		}
	}
}

// TestLeavesHierarchical builds a 3-level tree (paragraph → sentence →
// word) and verifies Leaves() returns only the word-level fragments,
// in DFS order, skipping the paragraph and sentence nodes.
func TestLeavesHierarchical(t *testing.T) {
	sm := NewSyncMap()
	// Paragraph 1: sentence A, sentence B.
	p1 := newFrag("p1", "para 1", "eng")
	p1Node := NewTree(p1)
	sm.Tree.AddChild(p1Node, true)

	s1a := newFrag("s1a", "sentence A", "eng")
	s1aNode := NewTree(s1a)
	p1Node.AddChild(s1aNode, true)
	s1aNode.AddChild(NewTree(newFrag("w1a1", "hello", "eng")), true)
	s1aNode.AddChild(NewTree(newFrag("w1a2", "world", "eng")), true)

	s1b := newFrag("s1b", "sentence B", "eng")
	s1bNode := NewTree(s1b)
	p1Node.AddChild(s1bNode, true)
	s1bNode.AddChild(NewTree(newFrag("w1b1", "foo", "eng")), true)

	// Paragraph 2: one sentence with two words.
	p2 := newFrag("p2", "para 2", "eng")
	p2Node := NewTree(p2)
	sm.Tree.AddChild(p2Node, true)

	s2 := newFrag("s2", "sentence C", "eng")
	s2Node := NewTree(s2)
	p2Node.AddChild(s2Node, true)
	s2Node.AddChild(NewTree(newFrag("w2a", "alpha", "eng")), true)
	s2Node.AddChild(NewTree(newFrag("w2b", "beta", "eng")), true)

	// Fragments returns the 2 paragraph-level nodes only.
	if got := len(sm.Fragments()); got != 2 {
		t.Errorf("Fragments len: got %d, want 2 (paragraphs)", got)
	}
	if sm.IsSingleLevel() {
		t.Error("expected multi-level map")
	}

	// Leaves returns the 5 word-level fragments in DFS order.
	leaves := sm.Leaves()
	wantIDs := []string{"w1a1", "w1a2", "w1b1", "w2a", "w2b"}
	if len(leaves) != len(wantIDs) {
		t.Fatalf("Leaves len: got %d, want %d", len(leaves), len(wantIDs))
	}
	for i, frag := range leaves {
		if got := frag.Identifier(); got != wantIDs[i] {
			t.Errorf("Leaves[%d]: got %q, want %q", i, got, wantIDs[i])
		}
	}
}

// TestSyncMapFragmentAccessorsNil exercises the nil-TextFragment guards
// added when the field became interface-typed.
func TestSyncMapFragmentAccessorsNil(t *testing.T) {
	f := NewSyncMapFragment(nil, timing.Zero, timing.Zero, Regular)
	if got := f.Identifier(); got != "" {
		t.Errorf("nil Identifier: got %q, want \"\"", got)
	}
	if got := f.Text(); got != "" {
		t.Errorf("nil Text: got %q, want \"\"", got)
	}
	if got := f.Lines(); got != nil {
		t.Errorf("nil Lines: got %v, want nil", got)
	}
	if got := f.Language(); got != "" {
		t.Errorf("nil Language: got %q, want \"\"", got)
	}
}
