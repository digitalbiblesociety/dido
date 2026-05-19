// Package syncmap provides the Tree data structure and sync-map types.
// Tree is a generic rooted, ordered, levelled tree — port of aeneas/tree.py.
package syncmap

// Tree is a rooted, ordered, levelled tree node.
// value is typed as any; nil means the node is empty (no value assigned).
type Tree struct {
	Value    any
	children []*Tree
	parent   *Tree
	level    int
}

// NewTree returns a new root node with the given value (may be nil).
func NewTree(value any) *Tree {
	return &Tree{Value: value}
}

// Level returns the depth of this node (0 for the root).
func (t *Tree) Level() int { return t.level }

// IsLeaf returns true if this node has no children.
func (t *Tree) IsLeaf() bool { return len(t.children) == 0 }

// IsEmpty returns true if this node's Value is nil.
func (t *Tree) IsEmpty() bool { return t.Value == nil }

// IsRoot returns true if this node has no parent.
func (t *Tree) IsRoot() bool { return t.parent == nil }

// Parent returns the parent node, or nil for the root.
func (t *Tree) Parent() *Tree { return t.parent }

// Children returns the ordered list of direct children.
func (t *Tree) Children() []*Tree { return t.children }

// ChildrenNotEmpty returns direct children that have a non-nil Value.
func (t *Tree) ChildrenNotEmpty() []*Tree {
	var out []*Tree
	for _, c := range t.children {
		if !c.IsEmpty() {
			out = append(out, c)
		}
	}
	return out
}

// VChildrenNotEmpty returns the Values of non-empty direct children.
func (t *Tree) VChildrenNotEmpty() []any {
	nodes := t.ChildrenNotEmpty()
	vals := make([]any, len(nodes))
	for i, n := range nodes {
		vals[i] = n.Value
	}
	return vals
}

// AddChild adds node as a child. If asLast is true, appends; otherwise prepends.
// Updates the child's parent and level fields.
func (t *Tree) AddChild(node *Tree, asLast bool) {
	if asLast {
		t.children = append(t.children, node)
	} else {
		t.children = append([]*Tree{node}, t.children...)
	}
	node.parent = t
	newHeight := t.level + 1
	for _, n := range node.Subtree() {
		n.level += newHeight
	}
}

// RemoveChild removes the child at the given index.
func (t *Tree) RemoveChild(index int) {
	if index < 0 {
		index = len(t.children) + index
	}
	t.children = append(t.children[:index], t.children[index+1:]...)
}

// RemoveChildren removes all direct children, optionally resetting their parent.
func (t *Tree) RemoveChildren(resetParent bool) {
	if resetParent {
		for _, c := range t.children {
			c.parent = nil
		}
	}
	t.children = nil
}

// Remove removes this node from its parent's children list.
func (t *Tree) Remove() {
	if t.parent == nil {
		return
	}
	for i, c := range t.parent.children {
		if c == t {
			t.parent.RemoveChild(i)
			t.parent = nil
			return
		}
	}
}

// GetChild returns the child at the given index.
func (t *Tree) GetChild(index int) *Tree { return t.children[index] }

// Subtree returns all nodes in the subtree rooted here, in DFS post-order.
// This node is always the last element (matches Python tree.py dfs).
func (t *Tree) Subtree() []*Tree {
	var out []*Tree
	for _, c := range t.children {
		out = append(out, c.Subtree()...)
	}
	out = append(out, t)
	return out
}

// Pre returns nodes in pre-order (current first, then children).
func (t *Tree) Pre() []*Tree {
	out := []*Tree{t}
	for _, c := range t.children {
		out = append(out, c.Pre()...)
	}
	return out
}

// Leaves returns all leaf nodes in DFS order.
func (t *Tree) Leaves() []*Tree {
	var out []*Tree
	for _, n := range t.Subtree() {
		if n.IsLeaf() {
			out = append(out, n)
		}
	}
	return out
}

// LeavesNotEmpty returns non-empty leaf nodes in DFS order.
func (t *Tree) LeavesNotEmpty() []*Tree {
	var out []*Tree
	for _, n := range t.Subtree() {
		if n.IsLeaf() && !n.IsEmpty() {
			out = append(out, n)
		}
	}
	return out
}

// Height returns the number of levels in the subtree (1 for a single-node tree).
func (t *Tree) Height() int {
	max := t.level
	for _, n := range t.Subtree() {
		if n.level > max {
			max = n.level
		}
	}
	return max - t.level + 1
}

// Levels returns a slice of slices, indexed by relative level (0 = root level).
func (t *Tree) Levels() [][]*Tree {
	h := t.Height()
	out := make([][]*Tree, h)
	for _, n := range t.Subtree() {
		rel := n.level - t.level
		out[rel] = append(out[rel], n)
	}
	return out
}

// Ancestor returns the k-th ancestor (0 = self, 1 = parent, etc.).
func (t *Tree) Ancestor(k int) *Tree {
	node := t
	for i := 0; i < k && node != nil; i++ {
		node = node.parent
	}
	return node
}

// KeepLevels rearranges the subtree to keep only the given relative level indices.
// Modifies the tree in place; always keeps level 0 (root).
func (t *Tree) KeepLevels(levelIndices []int) {
	prevLevels := t.Levels()

	// Build set of levels to keep; always keep 0.
	keep := map[int]bool{0: true}
	h := len(prevLevels)
	for _, l := range levelIndices {
		if l >= 0 && l < h {
			keep[l] = true
		}
	}

	// Sorted descending.
	sortedLevels := sortedKeysDesc(keep, h)

	// First, remove children from the nodes we want to keep.
	for _, l := range sortedLevels {
		if l < len(prevLevels) {
			for _, node := range prevLevels[l] {
				node.RemoveChildren(false)
			}
		}
	}

	// Then reconnect each kept level to its nearest kept ancestor.
	for i := 0; i < len(sortedLevels)-1; i++ {
		l := sortedLevels[i]
		if l >= len(prevLevels) {
			continue
		}
		parentLevel := sortedLevels[i+1]
		for _, node := range prevLevels[l] {
			anc := node.Ancestor(l - parentLevel)
			if anc != nil {
				anc.AddChild(node, true)
			}
		}
	}
}

func sortedKeysDesc(m map[int]bool, maxVal int) []int {
	var keys []int
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort (small slice).
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] > keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
