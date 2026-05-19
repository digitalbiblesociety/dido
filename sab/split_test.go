package main

import (
	"testing"

	"github.com/digitalbiblesociety/transliterate/script"
)

// TestLatinTextUsesPunctuationOnly: Roman-script text has no engine
// in the transliterate registry, so Detect returns nil and splitVerse
// (punctuation-only) is used — no over-splitting on word boundaries.
func TestLatinTextUsesPunctuationOnly(t *testing.T) {
	in := "In the beginning God created the heavens, and the earth."
	if eng := script.Detect(in); eng != nil {
		t.Errorf("expected no engine for Latin text; got %q", eng.Name)
	}
	frags := splitVerse(in)
	want := []string{
		"In the beginning God created the heavens,",
		"and the earth.",
	}
	if len(frags) != len(want) {
		t.Fatalf("fragment count: got %d, want %d (frags=%q)", len(frags), len(want), frags)
	}
	for i := range frags {
		if frags[i] != want[i] {
			t.Errorf("frag[%d]: got %q, want %q", i, frags[i], want[i])
		}
	}
}

// TestThaiPhraseSplitting verifies that Thai verses split on whitespace
// into phrase-level fragments via the transliterate package's script-
// aware splitter — the punctuation-only local splitVerse would leave
// each Thai verse as a single fragment.
func TestThaiPhraseSplitting(t *testing.T) {
	v2 := "แผ่นดินโลกนั้นก็ปราศจากรูปร่างและว่างเปล่าอยู่ ความมืดอยู่เหนือผิวน้ำ และพระวิญญาณของพระเจ้าปกอยู่เหนือผิวน้ำนั้น"
	eng := script.Detect(v2)
	if eng == nil {
		t.Fatal("no script detected for Thai input")
	}
	frags := eng.Split(v2)
	want := []string{
		"แผ่นดินโลกนั้นก็ปราศจากรูปร่างและว่างเปล่าอยู่",
		"ความมืดอยู่เหนือผิวน้ำ",
		"และพระวิญญาณของพระเจ้าปกอยู่เหนือผิวน้ำนั้น",
	}
	if len(frags) != len(want) {
		t.Fatalf("fragment count: got %d, want %d (frags=%q)", len(frags), len(want), frags)
	}
	for i := range frags {
		if frags[i] != want[i] {
			t.Errorf("frag[%d]: got %q, want %q", i, frags[i], want[i])
		}
	}
}
