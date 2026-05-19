package parity

import (
	"testing"

	"github.com/digitalbiblesociety/dido/internal/dtw"
)

// dtwPathResp mirrors the JSON returned by the Python helper for "dtw_path".
type dtwPathResp struct {
	Error string  `json:"error"`
	Trace string  `json:"trace"`
	N     int     `json:"n"`
	M     int     `json:"m"`
	Delta int     `json:"delta"`
	Len   int     `json:"len"`
	Path  [][]int `json:"path"`
}

// TestDTWStripeParity compares the Go stripe DTW against Python cdtw on the
// canonical (mfcc1, mfcc2) fixture pair. Bit-exact equality is expected: DTW
// is min/add over a deterministic order so float64 operations are reproducible.
func TestDTWStripeParity(t *testing.T) {
	SkipIfUnavailable(t)

	cases := []struct {
		name        string
		mfcc1, mfcc2 string
		delta        int
	}{
		{"default_3000", "mfcc1_12_1332", "mfcc2_12_868", 3000},
		{"small_300", "mfcc1_12_1332", "mfcc2_12_868", 300},
		{"medium_1000", "mfcc1_12_1332", "mfcc2_12_868", 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path1 := Fixture("cdtw", tc.mfcc1)
			path2 := Fixture("cdtw", tc.mfcc2)

			m1, err := ReadMFCCText(path1)
			if err != nil {
				t.Fatal(err)
			}
			m2, err := ReadMFCCText(path2)
			if err != nil {
				t.Fatal(err)
			}

			goPath := dtw.ComputePathStripe(m1, m2, tc.delta)
			goPairs := make([][2]int, len(goPath))
			for i, p := range goPath {
				goPairs[i] = [2]int{p.I, p.J}
			}

			req := map[string]any{
				"op":    "dtw_path",
				"mfcc1": path1,
				"mfcc2": path2,
				"delta": tc.delta,
			}
			var resp dtwPathResp
			if err := RunOnce(req, &resp); err != nil {
				t.Fatalf("python helper: %v", err)
			}
			if resp.Error != "" {
				t.Fatalf("python error: %s", resp.Error)
			}
			pyPairs := make([][2]int, len(resp.Path))
			for i, p := range resp.Path {
				pyPairs[i] = [2]int{p[0], p[1]}
			}

			if msg, ok := CompareIntPaths(tc.name, goPairs, pyPairs); !ok {
				t.Error(msg)
			}
		})
	}
}

// TestDTWPathFixtureLength matches the assertion in test_cdtw.py: the canonical
// (mfcc1, mfcc2) pair under delta=3000 must produce a path of length 1418
// starting at (0,0) and ending at (n-1, m-1).
func TestDTWPathFixtureLength(t *testing.T) {
	m1, err := ReadMFCCText(Fixture("cdtw", "mfcc1_12_1332"))
	if err != nil {
		t.Fatal(err)
	}
	m2, err := ReadMFCCText(Fixture("cdtw", "mfcc2_12_868"))
	if err != nil {
		t.Fatal(err)
	}
	path := dtw.ComputePathStripe(m1, m2, 3000)
	if len(path) != 1418 {
		t.Errorf("path length: got %d, want 1418", len(path))
	}
	if path[0] != (dtw.PathCell{I: 0, J: 0}) {
		t.Errorf("path[0]: got %v, want (0,0)", path[0])
	}
	wantEnd := dtw.PathCell{I: len(m1[0]) - 1, J: len(m2[0]) - 1}
	if path[len(path)-1] != wantEnd {
		t.Errorf("path end: got %v, want %v", path[len(path)-1], wantEnd)
	}
}
