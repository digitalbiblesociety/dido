package syncmap

// HeadTailFormat selects how the leading-silence (HEAD) and trailing-
// silence (TAIL) fragments produced by the SD detector appear in the
// serialised sync map. Values mirror aeneas's
// `os_task_file_head_tail_format` task-config key:
//
//	"add"     — emit HEAD and TAIL as their own rows / elements
//	            (aeneas default; same as empty string).
//	"hidden"  — drop HEAD and TAIL from the output entirely. The Regular
//	            fragments keep their detected begin/end times, so the
//	            first row's begin is whatever the SD detector picked,
//	            not 0; ditto the last row's end.
//	"stretch" — drop HEAD and TAIL, then extend the first Regular row's
//	            begin to 0 and the last Regular row's end to the audio
//	            duration so the on-screen timeline covers the whole file.
//
// Unknown values are treated as "add" (no-op) to match aeneas's
// permissive validator behaviour.
type HeadTailFormat string

const (
	HeadTailAdd     HeadTailFormat = "add"
	HeadTailHidden  HeadTailFormat = "hidden"
	HeadTailStretch HeadTailFormat = "stretch"
)

// ApplyHeadTailFormat rewrites the top-level fragment list according to
// htf and returns the same SyncMap (for fluent use). HEAD and TAIL only
// appear at the outermost level — multi-level alignment suppresses them
// in inner recursions — so the rewrite touches root children only.
//
// No-op when htf is empty, "add", or unrecognised.
func (s *SyncMap) ApplyHeadTailFormat(htf HeadTailFormat) *SyncMap {
	if s == nil || s.Tree == nil {
		return s
	}
	switch htf {
	case "", HeadTailAdd:
		return s
	case HeadTailStretch:
		s.stretchOverHeadTail()
		s.removeHeadTailChildren()
	case HeadTailHidden:
		s.removeHeadTailChildren()
	default:
		// Unknown value: aeneas accepts it silently; we do the same.
	}
	return s
}

// removeHeadTailChildren removes every direct child of the root whose
// SyncMapFragment has Type Head or Tail.
func (s *SyncMap) removeHeadTailChildren() {
	kept := s.Tree.children[:0]
	for _, c := range s.Tree.children {
		if isHeadOrTail(c.Value) {
			continue
		}
		kept = append(kept, c)
	}
	// Avoid leaking removed pointers into the underlying array's tail.
	for i := len(kept); i < len(s.Tree.children); i++ {
		s.Tree.children[i] = nil
	}
	s.Tree.children = kept
}

// stretchOverHeadTail extends the first Regular fragment's begin to the
// HEAD fragment's begin (typically zero) and the last Regular fragment's
// end to the TAIL fragment's end (typically the audio duration). Run
// BEFORE removeHeadTailChildren so the HEAD/TAIL spans are still
// available to consult.
func (s *SyncMap) stretchOverHeadTail() {
	var head, tail, first, last *SyncMapFragment
	for _, c := range s.Tree.children {
		f, ok := c.Value.(*SyncMapFragment)
		if !ok || f == nil {
			continue
		}
		switch f.Type {
		case Head:
			head = f
		case Tail:
			tail = f
		case Regular:
			if first == nil {
				first = f
			}
			last = f
		}
	}
	if first != nil && head != nil && first.Interval != nil && head.Interval != nil {
		first.Interval.Begin = head.Interval.Begin
	}
	if last != nil && tail != nil && last.Interval != nil && tail.Interval != nil {
		last.Interval.End = tail.Interval.End
	}
}

func isHeadOrTail(v any) bool {
	f, ok := v.(*SyncMapFragment)
	if !ok || f == nil {
		return false
	}
	return f.Type == Head || f.Type == Tail
}
