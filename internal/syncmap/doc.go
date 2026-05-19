// Package syncmap provides the SyncMap, SyncMapFragment, and Tree
// types — the data structures the alignment pipeline produces.
//
// A SyncMap is a Tree of SyncMapFragments. For the typical single-level
// task the tree is two levels deep (root → flat list of fragments).
// Multi-level workflows (paragraph → sentence → phrase → word) build deeper
// trees; use Fragments() for the top-level partition and Leaves() to
// walk all the way down to the deepest aligned spans.
//
// SyncMapFragment.TextFragment is typed as the TextFragment interface
// (Text, GetIdentifier, GetLines, GetLanguage). *internal/text.Fragment
// is the canonical implementation; tests and alternative front-ends can
// supply their own.
//
// Serialisers live in internal/syncmap/format. See its doc for the
// full list of supported output formats and their aliasing conventions
// (e.g. FormatTTML ≡ FormatDFXP, FormatCSV ≡ FormatCSVM).
package syncmap
