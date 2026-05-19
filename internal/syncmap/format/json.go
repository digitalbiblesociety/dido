package format

import (
	"encoding/json"
	"fmt"

	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

// ── JSON format ──────────────────────────────────────────────────────────────

// jsonFragment mirrors the Python JSON output structure.
// Fields are ordered alphabetically (matching Python's sort_keys=True).
type jsonFragment struct {
	Begin    string         `json:"begin"`
	Children []jsonFragment `json:"children"`
	End      string         `json:"end"`
	ID       string         `json:"id"`
	Language string         `json:"language"`
	Lines    []string       `json:"lines"`
}

type jsonRoot struct {
	Fragments []jsonFragment `json:"fragments"`
}

// treeNodeToJSONFragment converts a *syncmap.Tree node whose Value is a
// *syncmap.SyncMapFragment into a jsonFragment, recursing into children.
func treeNodeToJSONFragment(node *syncmap.Tree) (jsonFragment, bool) {
	if node.IsEmpty() {
		return jsonFragment{}, false
	}
	frag, ok := node.Value.(*syncmap.SyncMapFragment)
	if !ok {
		return jsonFragment{}, false
	}

	jf := jsonFragment{
		Begin:    toSSMMM(frag.Begin()),
		End:      toSSMMM(frag.End()),
		ID:       frag.Identifier(),
		Children: []jsonFragment{},
	}

	// Language and Lines from underlying text fragment.
	jf.Language = frag.Language()
	if lines := frag.Lines(); lines != nil {
		jf.Lines = lines
	} else {
		jf.Lines = []string{}
	}

	// Recurse into children.
	for _, child := range node.ChildrenNotEmpty() {
		if cf, ok3 := treeNodeToJSONFragment(child); ok3 {
			jf.Children = append(jf.Children, cf)
		}
	}

	return jf, true
}

// formatJSON serialises sm to the aeneas JSON format.
func formatJSON(sm *syncmap.SyncMap) string {
	root := jsonRoot{Fragments: []jsonFragment{}}

	for _, child := range sm.Tree.ChildrenNotEmpty() {
		if jf, ok := treeNodeToJSONFragment(child); ok {
			root.Fragments = append(root.Fragments, jf)
		}
	}

	// Use json.MarshalIndent with a single space indent (matching Python indent=1).
	b, err := json.MarshalIndent(root, "", " ")
	if err != nil {
		return "{}"
	}
	return string(b) + "\n"
}

// parseJSON reads the aeneas JSON format into sm.
func parseJSON(input string, sm *syncmap.SyncMap) error {
	var root jsonRoot
	if err := json.Unmarshal([]byte(input), &root); err != nil {
		return fmt.Errorf("format: json: %w", err)
	}
	for _, jf := range root.Fragments {
		if err := addJSONFragment(sm, jf); err != nil {
			return err
		}
	}
	return nil
}

func addJSONFragment(sm *syncmap.SyncMap, jf jsonFragment) error {
	begin, err := parseSSMMM(jf.Begin)
	if err != nil {
		return fmt.Errorf("format: json: bad begin %q: %w", jf.Begin, err)
	}
	end, err := parseSSMMM(jf.End)
	if err != nil {
		return fmt.Errorf("format: json: bad end %q: %w", jf.End, err)
	}
	tf := &text.Fragment{
		Identifier: jf.ID,
		Language:   jf.Language,
		Lines:      jf.Lines,
	}
	frag := syncmap.NewSyncMapFragment(tf, begin, end, syncmap.Regular)
	sm.Add(frag)
	// Note: children are not re-nested (flat add); for round-trip fidelity of
	// hierarchical maps a more complex tree rebuild would be needed.
	return nil
}

// ── RBSE format ──────────────────────────────────────────────────────────────

type rbseItem struct {
	Begin string `json:"begin"`
	End   string `json:"end"`
	ID    string `json:"id"`
}

type rbseRoot struct {
	SMILData []rbseItem `json:"smil_data"`
	SMILIDs  []string   `json:"smil_ids"`
}

// formatRBSE serialises sm to the RBSE JSON format.
func formatRBSE(sm *syncmap.SyncMap) string {
	r := rbseRoot{
		SMILData: []rbseItem{},
		SMILIDs:  []string{},
	}
	// RBSE was designed for the EPUB Media Overlay flat case — one entry
	// per playable cue. Walk leaves so multi-level sync maps still
	// surface the innermost partition.
	for _, frag := range sm.Leaves() {
		id := frag.Identifier()
		r.SMILData = append(r.SMILData, rbseItem{
			Begin: toSSMMM(frag.Begin()),
			End:   toSSMMM(frag.End()),
			ID:    id,
		})
		r.SMILIDs = append(r.SMILIDs, id)
	}
	b, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		return "{}"
	}
	return string(b) + "\n"
}

// parseRBSE reads the RBSE JSON format into sm.
// Text is not stored in RBSE so fragments are created with empty text.
func parseRBSE(input string, sm *syncmap.SyncMap) error {
	var r rbseRoot
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		return fmt.Errorf("format: rbse: %w", err)
	}
	for _, item := range r.SMILData {
		begin, err := parseSSMMM(item.Begin)
		if err != nil {
			return fmt.Errorf("format: rbse: bad begin %q: %w", item.Begin, err)
		}
		end, err := parseSSMMM(item.End)
		if err != nil {
			return fmt.Errorf("format: rbse: bad end %q: %w", item.End, err)
		}
		tf := &text.Fragment{
			Identifier: item.ID,
			Lines:      []string{},
		}
		frag := syncmap.NewSyncMapFragment(tf, begin, end, syncmap.Regular)
		sm.Add(frag)
	}
	return nil
}

// Ensure timing import is used (it's used transitively via helper functions).
var _ = timing.Zero
