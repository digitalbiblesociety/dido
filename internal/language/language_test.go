package language

import (
	"sort"
	"testing"
)

func TestIsAllowed(t *testing.T) {
	for _, code := range []Language{"eng", "ita", "deu", "jpn", "zho"} {
		if !IsAllowed(code) {
			t.Errorf("expected %q to be allowed", code)
		}
	}
	if IsAllowed("xyz") {
		t.Error("xyz should not be allowed")
	}
}

func TestHumanName(t *testing.T) {
	if HumanName("eng") != "English" {
		t.Errorf("got %q", HumanName("eng"))
	}
	if HumanName("xyz") != "xyz" {
		t.Error("unknown code should return itself")
	}
}

func TestAllowedValuesSorted(t *testing.T) {
	if !sort.StringsAreSorted(AllowedValues) {
		t.Error("AllowedValues should be sorted")
	}
}

func TestAllowedValuesCount(t *testing.T) {
	if len(AllowedValues) != len(CodeToHuman) {
		t.Errorf("AllowedValues count %d != CodeToHuman count %d",
			len(AllowedValues), len(CodeToHuman))
	}
}

// SIL ships ~7900 codes. If the registry comes back at a handful of
// codes the embedded data probably failed to load.
func TestRegistryLoadedFromSIL(t *testing.T) {
	if n := len(AllowedValues); n < 7000 {
		t.Fatalf("registry too small (%d codes); embedded SIL data probably failed to load", n)
	}
}

// Lookup returns the full Info row and identifies the source columns.
func TestLookupExposesInfo(t *testing.T) {
	info, ok := Lookup("eng")
	if !ok {
		t.Fatal("eng should be in the registry")
	}
	if info.Part1 != "en" {
		t.Errorf("Part1 for eng: got %q, want %q", info.Part1, "en")
	}
	if info.Scope != "I" || info.Type != "L" {
		t.Errorf("scope/type for eng: got %q/%q, want I/L", info.Scope, info.Type)
	}
}
