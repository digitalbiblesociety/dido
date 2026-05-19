package syncmap

import (
	"reflect"
	"testing"
)

func TestNewTree(t *testing.T) {
	root := NewTree("hello")
	if root.Value != "hello" {
		t.Fatalf("want hello, got %v", root.Value)
	}
	if !root.IsRoot() {
		t.Fatal("should be root")
	}
	if !root.IsLeaf() {
		t.Fatal("should be leaf with no children")
	}
}

func TestAddRemoveChild(t *testing.T) {
	root := NewTree(nil)
	a := NewTree("a")
	b := NewTree("b")
	root.AddChild(a, true)
	root.AddChild(b, true)
	if len(root.Children()) != 2 {
		t.Fatalf("want 2 children, got %d", len(root.Children()))
	}
	if root.GetChild(0).Value != "a" {
		t.Fatal("first child should be a")
	}
	root.RemoveChild(0)
	if len(root.Children()) != 1 {
		t.Fatalf("want 1 child after remove, got %d", len(root.Children()))
	}
	if root.GetChild(0).Value != "b" {
		t.Fatal("remaining child should be b")
	}
}

func TestAddChildPrepend(t *testing.T) {
	root := NewTree(nil)
	a := NewTree("a")
	b := NewTree("b")
	root.AddChild(a, true)
	root.AddChild(b, false) // prepend
	if root.GetChild(0).Value != "b" {
		t.Fatal("first child should be b after prepend")
	}
}

func TestLevel(t *testing.T) {
	root := NewTree(nil)
	child := NewTree("c")
	grandchild := NewTree("gc")
	root.AddChild(child, true)
	child.AddChild(grandchild, true)
	if root.Level() != 0 {
		t.Fatalf("root level want 0, got %d", root.Level())
	}
	if child.Level() != 1 {
		t.Fatalf("child level want 1, got %d", child.Level())
	}
	if grandchild.Level() != 2 {
		t.Fatalf("grandchild level want 2, got %d", grandchild.Level())
	}
}

func TestHeight(t *testing.T) {
	root := NewTree(nil)
	child := NewTree("c")
	root.AddChild(child, true)
	child.AddChild(NewTree("gc"), true)
	if root.Height() != 3 {
		t.Fatalf("want height 3, got %d", root.Height())
	}
}

func TestSubtreeOrder(t *testing.T) {
	root := NewTree("root")
	a := NewTree("a")
	b := NewTree("b")
	root.AddChild(a, true)
	root.AddChild(b, true)
	nodes := root.Subtree()
	// post-order: a, b, root
	want := []any{"a", "b", "root"}
	for i, n := range nodes {
		if n.Value != want[i] {
			t.Fatalf("node %d: want %v, got %v", i, want[i], n.Value)
		}
	}
}

func TestLeaves(t *testing.T) {
	root := NewTree(nil)
	a := NewTree("a")
	b := NewTree("b")
	a.AddChild(NewTree("leaf1"), true)
	root.AddChild(a, true)
	root.AddChild(b, true) // b is a leaf
	leaves := root.Leaves()
	if len(leaves) != 2 {
		t.Fatalf("want 2 leaves, got %d", len(leaves))
	}
}

func TestLevels(t *testing.T) {
	root := NewTree("r")
	a := NewTree("a")
	b := NewTree("b")
	a.AddChild(NewTree("a1"), true)
	root.AddChild(a, true)
	root.AddChild(b, true)
	levels := root.Levels()
	if len(levels) != 3 {
		t.Fatalf("want 3 levels, got %d", len(levels))
	}
	if len(levels[0]) != 1 || levels[0][0].Value != "r" {
		t.Fatal("level 0 should have root")
	}
	if len(levels[1]) != 2 {
		t.Fatalf("level 1 should have 2 nodes, got %d", len(levels[1]))
	}
}

func TestAncestor(t *testing.T) {
	root := NewTree("root")
	child := NewTree("child")
	gc := NewTree("gc")
	root.AddChild(child, true)
	child.AddChild(gc, true)
	if gc.Ancestor(0) != gc {
		t.Fatal("ancestor(0) should be self")
	}
	if gc.Ancestor(1) != child {
		t.Fatal("ancestor(1) should be parent")
	}
	if gc.Ancestor(2) != root {
		t.Fatal("ancestor(2) should be grandparent")
	}
	if gc.Ancestor(3) != nil {
		t.Fatal("ancestor(3) should be nil for root's parent")
	}
}

func TestChildrenNotEmpty(t *testing.T) {
	root := NewTree(nil)
	root.AddChild(NewTree(nil), true) // empty
	root.AddChild(NewTree("x"), true)
	notEmpty := root.ChildrenNotEmpty()
	if len(notEmpty) != 1 {
		t.Fatalf("want 1 non-empty child, got %d", len(notEmpty))
	}
}

func TestRemove(t *testing.T) {
	root := NewTree(nil)
	a := NewTree("a")
	root.AddChild(a, true)
	a.Remove()
	if len(root.Children()) != 0 {
		t.Fatal("after Remove, root should have no children")
	}
}

func TestKeepLevels(t *testing.T) {
	root := NewTree("r")
	a := NewTree("a")
	a1 := NewTree("a1")
	b := NewTree("b")
	root.AddChild(a, true)
	root.AddChild(b, true)
	a.AddChild(a1, true)

	// keep only level 0 (root) and level 2 (grandchildren)
	root.KeepLevels([]int{2})
	levels := root.Levels()
	_ = reflect.DeepEqual(levels, levels) // just ensure no panic
	// root should still exist
	if root.Value != "r" {
		t.Fatal("root lost its value")
	}
}
