package language

import (
	"testing"
)

func TestSortLexicographic(t *testing.T) {
	ids := []string{"f010", "f002", "f020", "f1"}
	got := SortIDs(ids, IDSortLexicographic)
	want := []string{"f002", "f010", "f020", "f1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pos %d: want %q got %q", i, want[i], got[i])
		}
	}
}

func TestSortNumeric(t *testing.T) {
	ids := []string{"f010", "f2", "f020", "f1"}
	got := SortIDs(ids, IDSortNumeric)
	want := []string{"f1", "f2", "f010", "f020"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pos %d: want %q got %q", i, want[i], got[i])
		}
	}
}

func TestSortUnsorted(t *testing.T) {
	ids := []string{"f010", "f002", "f020"}
	got := SortIDs(ids, IDSortUnsorted)
	want := []string{"f010", "f002", "f020"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pos %d: want %q got %q", i, want[i], got[i])
		}
	}
}

func TestSortDoesNotMutateInput(t *testing.T) {
	ids := []string{"b", "a"}
	SortIDs(ids, IDSortLexicographic)
	if ids[0] != "b" {
		t.Fatal("SortIDs should not mutate the input slice")
	}
}
