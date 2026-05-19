// Package syncmap provides the SyncMap type and related types.
// Port of aeneas/syncmap/__init__.py.
package syncmap

// SyncMap is a tree of SyncMapFragments.
type SyncMap struct {
	Tree *Tree
}

// NewSyncMap returns a SyncMap with an empty root node.
func NewSyncMap() *SyncMap {
	return &SyncMap{Tree: NewTree(nil)}
}

// Add appends f as the last child of the root.
func (s *SyncMap) Add(f *SyncMapFragment) {
	s.Tree.AddChild(NewTree(f), true)
}

// Fragments returns non-empty direct children of the root as
// *SyncMapFragment values. For multi-level sync maps this is the
// top-level partition only; use Leaves to walk all the way down to the
// deepest (leaf) fragments.
func (s *SyncMap) Fragments() []*SyncMapFragment {
	vals := s.Tree.VChildrenNotEmpty()
	out := make([]*SyncMapFragment, len(vals))
	for i, v := range vals {
		out[i] = v.(*SyncMapFragment)
	}
	return out
}

// Leaves returns the deepest non-empty fragments in DFS order.
//
// On a single-level sync map (root → flat list of fragments) this is
// identical to Fragments. On a hierarchical map (e.g. produced by mplain
// or munparsed text input — paragraph → sentence → phrase → word) Leaves
// returns every word-level fragment, skipping the intermediate
// paragraph/sentence/phrase nodes. This mirrors Python's syncmap.leaves().
//
// Use Fragments when you want the top-level partition only (the format
// you serialise depends on this); use Leaves when you want every aligned
// span regardless of nesting (e.g. for cue-by-cue subtitle output of a
// hierarchical alignment).
func (s *SyncMap) Leaves() []*SyncMapFragment {
	nodes := s.Tree.LeavesNotEmpty()
	out := make([]*SyncMapFragment, 0, len(nodes))
	for _, n := range nodes {
		if frag, ok := n.Value.(*SyncMapFragment); ok {
			out = append(out, frag)
		}
	}
	return out
}

// IsSingleLevel returns true if the tree height is at most 2 (root + flat list of children).
func (s *SyncMap) IsSingleLevel() bool {
	return s.Tree.Height() <= 2
}
