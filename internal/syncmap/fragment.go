// Package syncmap provides the SyncMap type and related types.
// Port of aeneas/syncmap/fragment.py.
package syncmap

import (
	"github.com/digitalbiblesociety/dido/internal/timing"
)

// FragmentType identifies the role of a SyncMapFragment.
type FragmentType int

const (
	Regular   FragmentType = 0
	Head      FragmentType = 1
	Tail      FragmentType = 2
	NonSpeech FragmentType = 3
)

// TextFragment is the abstract contract a SyncMapFragment's text payload
// must satisfy. *text.Fragment is the canonical implementation; defining
// the interface here (rather than referencing the concrete type) keeps
// the package import-cycle-free — internal/text imports internal/syncmap
// for the Tree type, so the reverse import is forbidden.
//
// The Get*-prefixed names match the accessor convention already used on
// *text.Fragment (which exposes Identifier/Language/Lines as fields and
// provides the Get* methods alongside).
type TextFragment interface {
	Text() string
	GetIdentifier() string
	GetLines() []string
	GetLanguage() string
}

// SyncMapFragment pairs a text fragment with its audio time interval.
// TextFragment is an interface (typically holding a *text.Fragment) so
// callers that build sync maps in tests or alternative front-ends can
// supply their own implementation without going through reflection.
type SyncMapFragment struct {
	TextFragment TextFragment
	Interval     *timing.TimeInterval
	Type         FragmentType
	Confidence   float64
}

// NewSyncMapFragment constructs a SyncMapFragment with the given text fragment,
// begin/end times, and fragment type. tf may be nil. Confidence defaults to 0.
func NewSyncMapFragment(tf TextFragment, begin, end timing.TimeValue, ft FragmentType) *SyncMapFragment {
	iv := timing.NewTimeInterval(begin, end)
	return &SyncMapFragment{
		TextFragment: tf,
		Interval:     &iv,
		Type:         ft,
	}
}

// Identifier returns the text fragment's identifier, or "" if nil.
func (f *SyncMapFragment) Identifier() string {
	if f.TextFragment == nil {
		return ""
	}
	return f.TextFragment.GetIdentifier()
}

// Text returns the text fragment's text, or "" if nil.
func (f *SyncMapFragment) Text() string {
	if f.TextFragment == nil {
		return ""
	}
	return f.TextFragment.Text()
}

// Lines returns the text fragment's per-line slice, or nil if no fragment.
func (f *SyncMapFragment) Lines() []string {
	if f.TextFragment == nil {
		return nil
	}
	return f.TextFragment.GetLines()
}

// Language returns the text fragment's language, or "" if no fragment.
func (f *SyncMapFragment) Language() string {
	if f.TextFragment == nil {
		return ""
	}
	return f.TextFragment.GetLanguage()
}

// Begin returns the interval's Begin value, or the zero TimeValue if Interval is nil.
func (f *SyncMapFragment) Begin() timing.TimeValue {
	if f.Interval == nil {
		return timing.Zero
	}
	return f.Interval.Begin
}

// End returns the interval's End value, or the zero TimeValue if Interval is nil.
func (f *SyncMapFragment) End() timing.TimeValue {
	if f.Interval == nil {
		return timing.Zero
	}
	return f.Interval.End
}

// IsRegular returns true if the fragment type is Regular.
func (f *SyncMapFragment) IsRegular() bool {
	return f.Type == Regular
}

// IsHeadOrTail returns true if the fragment type is Head or Tail.
func (f *SyncMapFragment) IsHeadOrTail() bool {
	return f.Type == Head || f.Type == Tail
}
