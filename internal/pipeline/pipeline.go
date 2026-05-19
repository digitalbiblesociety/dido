// Package pipeline orchestrates the single-level forced alignment pipeline.
// Port of aeneas/executetask.py ExecuteTask._execute_single_level_task.
package pipeline

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/digitalbiblesociety/dido/internal/aba"
	"github.com/digitalbiblesociety/dido/internal/align"
	"github.com/digitalbiblesociety/dido/internal/audiomfcc"
	"github.com/digitalbiblesociety/dido/internal/config"
	"github.com/digitalbiblesociety/dido/internal/ffmpeg"
	"github.com/digitalbiblesociety/dido/internal/mfcc"
	"github.com/digitalbiblesociety/dido/internal/sd"
	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/timing"
	"github.com/digitalbiblesociety/dido/internal/tts"
	"github.com/digitalbiblesociety/dido/internal/vad"
)

// TaskConfig holds the per-task configuration parsed from the config string.
// These correspond to the Python TaskConfiguration FIELDS.
type TaskConfig struct {
	Language string // task_language

	// ABA boundary adjustment
	ABAlgorithm        string  // task_adjust_boundary_algorithm
	ABAAftercurrent    float64 // task_adjust_boundary_aftercurrent_value
	ABABeforenext      float64 // task_adjust_boundary_beforenext_value
	ABAOffset          float64 // task_adjust_boundary_offset_value
	ABAPercent         float64 // task_adjust_boundary_percent_value
	ABARate            float64 // task_adjust_boundary_rate_value
	ABANoZero          bool    // task_adjust_boundary_no_zero
	ABANonspeechMin    float64 // task_adjust_boundary_nonspeech_min
	ABANonspeechString string  // task_adjust_boundary_nonspeech_string

	// Text input
	TextFormat          text.Format // is_text_type
	TextIgnoreRegex     string      // is_text_ignore_regex
	TextTranslitMapPath string      // is_text_transliterate_map

	// Explicit head/process/tail lengths (nil = not set)
	AudioHead    *timing.TimeValue // is_audio_file_head_length
	AudioProcess *timing.TimeValue // is_audio_file_process_length
	AudioTail    *timing.TimeValue // is_audio_file_tail_length

	// Head/tail detection bounds (nil = no detection)
	HeadMin *timing.TimeValue // is_audio_file_detect_head_min
	HeadMax *timing.TimeValue // is_audio_file_detect_head_max
	TailMin *timing.TimeValue // is_audio_file_detect_tail_min
	TailMax *timing.TimeValue // is_audio_file_detect_tail_max

	// Output
	OutputFormat    string // os_task_file_format
	HeadTailFormat  string // os_task_file_head_tail_format: "" / "add" / "hidden" / "stretch"
	SMILAudioRef    string // os_task_file_smil_audio_ref
	SMILPageRef     string // os_task_file_smil_page_ref
	EAFAudioRef     string // os_task_file_eaf_audio_ref

	// Granularity selects the deepest text-tree level to align.
	//   1 = paragraph (default; single-level alignment as in execute_task)
	//   2 = paragraph + sentence
	//   3 = paragraph + sentence + phrase + word
	// Only meaningful when the input text format produces a hierarchical
	// tree (mplain, munparsed). Ignored for flat formats. The phrase
	// layer (introduced between sentence and word) is a dido extension
	// over the upstream aeneas API; set granularity=3 to enable it.
	Granularity int // task_granularity
}

// ParseTaskConfig extracts task-level parameters from the config string.
// Keys this package doesn't recognise are silently ignored; use
// ParseTaskConfigStrict to inspect them.
func ParseTaskConfig(configString string) TaskConfig {
	tc, _ := ParseTaskConfigStrict(configString)
	return tc
}

// ParseTaskConfigStrict is like ParseTaskConfig but also returns the set of
// keys not recognised by this parser. Note that runtime config keys
// (mfcc_*, dtw_*, vad_*, etc.) are not recognised here either — callers
// that want the union should intersect this list with the runtime parser's
// unknown set; see cmd/aeneas.
func ParseTaskConfigStrict(configString string) (TaskConfig, []string) {
	kv := config.ParseConfigString(configString)
	tc := TaskConfig{TextFormat: text.FormatPlain}
	var unknown []string
	for k, v := range kv {
		if !tc.set(k, v) {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 1 {
		sortStrings(unknown)
	}
	return tc, unknown
}

func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// set applies one key=value pair onto the task config. Returns true when
// the key was recognised.
func (tc *TaskConfig) set(key, val string) bool {
	switch key {
	case "task_language", "language":
		tc.Language = val
	case "task_adjust_boundary_algorithm", "aba_algorithm":
		tc.ABAlgorithm = val
	case "task_adjust_boundary_aftercurrent_value", "aba_aftercurrent_value":
		tc.ABAAftercurrent = parseFloat(val)
	case "task_adjust_boundary_beforenext_value", "aba_beforenext_value":
		tc.ABABeforenext = parseFloat(val)
	case "task_adjust_boundary_offset_value", "aba_offset_value":
		tc.ABAOffset = parseFloat(val)
	case "task_adjust_boundary_percent_value", "aba_percent_value":
		tc.ABAPercent = parseFloat(val)
	case "task_adjust_boundary_rate_value", "aba_rate_value":
		tc.ABARate = parseFloat(val)
	case "task_adjust_boundary_no_zero", "aba_no_zero":
		tc.ABANoZero = parseBool(val)
	case "task_adjust_boundary_nonspeech_min", "aba_nonspeech_min":
		tc.ABANonspeechMin = parseFloat(val)
	case "task_adjust_boundary_nonspeech_string", "aba_nonspeech_string":
		tc.ABANonspeechString = val
	case "is_text_type", "i_t_format":
		tc.TextFormat = text.Format(val)
	case "is_text_ignore_regex", "i_t_ignore_regex":
		tc.TextIgnoreRegex = val
	case "is_text_transliterate_map", "i_t_transliterate_map":
		tc.TextTranslitMapPath = val
	case "is_audio_file_head_length", "i_a_head":
		if tv, err := timing.ParseTimeValue(val); err == nil {
			tc.AudioHead = &tv
		}
	case "is_audio_file_process_length", "i_a_process":
		if tv, err := timing.ParseTimeValue(val); err == nil {
			tc.AudioProcess = &tv
		}
	case "is_audio_file_tail_length", "i_a_tail":
		if tv, err := timing.ParseTimeValue(val); err == nil {
			tc.AudioTail = &tv
		}
	case "is_audio_file_detect_head_min", "i_a_head_min":
		if tv, err := timing.ParseTimeValue(val); err == nil {
			tc.HeadMin = &tv
		}
	case "is_audio_file_detect_head_max", "i_a_head_max":
		if tv, err := timing.ParseTimeValue(val); err == nil {
			tc.HeadMax = &tv
		}
	case "is_audio_file_detect_tail_min", "i_a_tail_min":
		if tv, err := timing.ParseTimeValue(val); err == nil {
			tc.TailMin = &tv
		}
	case "is_audio_file_detect_tail_max", "i_a_tail_max":
		if tv, err := timing.ParseTimeValue(val); err == nil {
			tc.TailMax = &tv
		}
	case "os_task_file_format", "o_format":
		tc.OutputFormat = val
	case "os_task_file_head_tail_format", "o_h_t_format":
		tc.HeadTailFormat = val
	case "os_task_file_smil_audio_ref", "o_smil_audio_ref":
		tc.SMILAudioRef = val
	case "os_task_file_smil_page_ref", "o_smil_page_ref":
		tc.SMILPageRef = val
	case "os_task_file_eaf_audio_ref", "o_eaf_audio_ref":
		tc.EAFAudioRef = val
	case "task_granularity":
		tc.Granularity = parseInt(val)
	default:
		return false
	}
	return true
}

// Execute runs the single-level forced alignment pipeline.
//
// Audio decoding and text synthesis are launched concurrently; they are
// independent of each other and together are the dominant cost. Head/tail
// detection (if requested) runs after the audio path completes but overlaps
// with the remaining synthesis work. All subsequent steps (DTW, ABA) are
// sequential.
//
// Serialisation of the resulting SyncMap is the caller's responsibility.
func Execute(audioPath string, fragments []*text.Fragment, tc TaskConfig, rc config.RuntimeConfig) (*syncmap.SyncMap, error) {
	src := func() ([]float64, uint32, error) {
		return ffmpeg.Decode(audioPath, rc.FFMpegSampleRate, rc.FFMpegPath)
	}
	// nil cache: one call, nothing to amortise.
	return executeWithSource(src, timing.Zero, fragments, tc, rc, true, nil)
}

// sampleSource returns mono float64 audio samples at the requested sample
// rate. It is called inside the audio-path goroutine so that slow producers
// (e.g. ffmpeg) overlap with TTS synthesis instead of serialising in front
// of it.
type sampleSource func() (samples []float64, rate uint32, err error)

// staticSamples wraps an already-decoded buffer as a sampleSource. Used by
// the multi-level recursive path where the parent has already produced
// per-child sample slices.
func staticSamples(samples []float64, rate uint32) sampleSource {
	return func() ([]float64, uint32, error) { return samples, rate, nil }
}

// executeWithSource runs the alignment given a sample source (lazy decode
// or pre-decoded buffer) and an audio offset. audioOffset is added to every
// boundary time in the returned syncmap so the result refers to the
// original (un-sliced) audio. allowHeadTail gates the SD detector — true
// at top-level, false during multi-level recursion (the parent's window
// already trims silence).
//
// The sample source is invoked inside the real-MFCC goroutine so that
// audio decode (when the source is the ffmpeg fast path) runs in parallel
// with text synthesis on the TTS goroutine.
//
// cache may be nil. When non-nil it shares *mfcc.Compiled across
// recursive ExecuteMultilevel calls.
func executeWithSource(
	src sampleSource,
	audioOffset timing.TimeValue,
	fragments []*text.Fragment,
	tc TaskConfig,
	rc config.RuntimeConfig,
	allowHeadTail bool,
	cache *compiledCache,
) (*syncmap.SyncMap, error) {
	if len(fragments) == 0 {
		return syncmap.NewSyncMap(), nil
	}
	mp := rcToMFCCParams(rc)

	// Toggle auto-transliteration based on config. The flag is process-
	// global (atomic) because the TTS worker pool is package-level.
	if rc.TTSAutoTranslit {
		tts.AutoTransliterateDisabled.Store(0)
	} else {
		tts.AutoTransliterateDisabled.Store(1)
	}

	espeakPath, err := tts.ResolveBinary(rc.TTS, rc.TTSPath)
	if err != nil {
		return nil, fmt.Errorf("pipeline: espeak path: %w", err)
	}

	// ── Concurrent path A: decode (lazy) → real-wave MFCC ────────────────────
	type audioOut struct {
		mfcc *audiomfcc.AudioMFCC
		err  error
	}
	audioCh := make(chan audioOut, 1)
	go func() {
		samples, rate, err := src()
		if err != nil {
			audioCh <- audioOut{nil, fmt.Errorf("pipeline: audio decode: %w", err)}
			return
		}
		c, err := cache.get(mp, rate)
		if err != nil {
			audioCh <- audioOut{nil, fmt.Errorf("pipeline: real MFCC compile: %w", err)}
			return
		}
		am, err := audiomfcc.FromSamplesCompiled(samples, c)
		if err != nil {
			audioCh <- audioOut{nil, fmt.Errorf("pipeline: real MFCC: %w", err)}
			return
		}
		if rc.MFCCMaskNonspeech {
			am.RunVAD(vad.Params{
				LogEnergyThreshold: rc.MFCCMaskLogEnergyThreshold,
				MinNonspeechLength: rc.MFCCMaskMinNonspeechLength,
				ExtendBefore:       rc.MFCCMaskExtendBefore,
				ExtendAfter:        rc.MFCCMaskExtendAfter,
			})
		}
		audioCh <- audioOut{am, nil}
	}()

	// ── Concurrent path B: synthesize text → synth-wave MFCC ──────────────
	type synthOut struct {
		mfcc      *audiomfcc.AudioMFCC
		intervals []timing.TimeInterval
		err       error
	}
	synthCh := make(chan synthOut, 1)
	go func() {
		sr, err := tts.SynthesizeMultipleWith(toTTSFragments(fragments, tc.Language), espeakPath, rc.TTSConcurrency)
		if err != nil {
			synthCh <- synthOut{err: fmt.Errorf("pipeline: synthesis: %w", err)}
			return
		}
		if len(sr.Samples) == 0 {
			synthCh <- synthOut{err: fmt.Errorf("pipeline: synthesis produced no audio")}
			return
		}
		c, err := cache.get(mp, sr.SampleRate)
		if err != nil {
			synthCh <- synthOut{err: fmt.Errorf("pipeline: synth MFCC compile: %w", err)}
			return
		}
		sm, err := audiomfcc.FromSamplesCompiled(sr.Samples, c)
		if err != nil {
			synthCh <- synthOut{err: fmt.Errorf("pipeline: synth MFCC: %w", err)}
			return
		}
		synthCh <- synthOut{sm, sr.Intervals, nil}
	}()

	// ── Collect results ────────────────────────────────────────────────────
	var (
		ao audioOut
		so synthOut
		wg sync.WaitGroup
	)
	wg.Add(2)
	go func() { defer wg.Done(); ao = <-audioCh }()
	go func() { defer wg.Done(); so = <-synthCh }()
	wg.Wait()

	if ao.err != nil {
		return nil, ao.err
	}
	if so.err != nil {
		return nil, so.err
	}

	realMFCC := ao.mfcc

	// Head/tail detection. Skipped during multi-level recursion (allowHeadTail=false)
	// because the parent's audio window already excludes its own silence.
	if allowHeadTail {
		// The SD detector can reuse the forward synthesis MFCC we already
		// computed (so.mfcc) to skip a redundant synth+MFCC pass for head
		// detection; tail still resynthesises (see C5 in IMPROVEMENT_PLANS.md).
		headLen, processLen, tailLen, hterr := headTailLengths(realMFCC, so.mfcc, fragments, tc, rc, mp, espeakPath)
		if hterr != nil {
			return nil, fmt.Errorf("pipeline: head/tail: %w", hterr)
		}
		realMFCC.SetHeadMiddleTail(headLen, processLen, tailLen)
	}

	// DTW alignment → boundary frame indices.
	mws := rc.MFCCWindowShift
	delta := int(2.0 * rc.DTWMargin / mws)
	boundaries, err := align.ComputeBoundaries(realMFCC, so.mfcc, so.intervals, mws, delta)
	if err != nil {
		return nil, fmt.Errorf("pipeline: align: %w", err)
	}

	// Adjust boundaries → SyncMap.
	sm, err := aba.Adjust(boundaries, fragments, realMFCC, buildABAParams(tc, rc))
	if err != nil {
		return nil, fmt.Errorf("pipeline: adjust: %w", err)
	}
	// Shift all fragment boundaries by audioOffset so the result refers to
	// the absolute time in the original (un-sliced) audio.
	if !audioOffset.IsZero() {
		for _, frag := range sm.Fragments() {
			frag.Interval.Begin = frag.Interval.Begin.Add(audioOffset)
			frag.Interval.End = frag.Interval.End.Add(audioOffset)
		}
	}
	return sm, nil
}

// ExecuteMultilevel runs the alignment recursively over a hierarchical
// text tree (e.g. mplain / munparsed input → paragraph → sentence →
// phrase → word, see TaskConfig.Granularity).
//
// For flat text input or when tc.Granularity ≤ 1, this is equivalent to
// Execute on the top-level fragments. For deeper granularity, level-1
// alignment establishes top-level boundaries; then for each parent with
// children, the audio is sliced to that parent's time range and a fresh
// alignment runs on its children using the level-N MFCC parameters.
//
// The returned SyncMap mirrors the input text tree's hierarchy. Flat
// output formats (SRT, VTT, CSV, …) emit one cue per leaf (see A5);
// hierarchy-aware formats (JSON, XML, SMIL/TTML at multi-level) preserve
// the structure.
func ExecuteMultilevel(audioPath string, tf *text.TextFile, tc TaskConfig, rc config.RuntimeConfig) (*syncmap.SyncMap, error) {
	// Resolve the tree depth we will recurse through. tc.Granularity
	// follows aeneas's 1/2/3 dial:
	//   1 → depth 1 (paragraph only — single-level alignment)
	//   2 → depth 2 (paragraph + sentence)
	//   3 → depth 4 (paragraph + sentence + phrase + word)
	// The phrase layer is the dido-specific extension that fills the gap
	// between sentence and word in the text tree (see
	// internal/text/textfile.go and internal/text/unparsed.go).
	maxLevel := tc.Granularity
	switch {
	case maxLevel < 1:
		maxLevel = 1
	case maxLevel >= 3:
		maxLevel = 4
	}

	// Level 1: top-level alignment over the root children. We use the
	// existing single-level RuntimeConfig (rc.SetGranularity has already
	// been called by the caller with level 1).
	topNodes := tf.Tree.ChildrenNotEmpty()
	topFrags := make([]*text.Fragment, 0, len(topNodes))
	for _, n := range topNodes {
		if frag, ok := n.Value.(*text.Fragment); ok {
			topFrags = append(topFrags, frag)
		}
	}
	if len(topFrags) == 0 {
		return syncmap.NewSyncMap(), nil
	}

	// Decode lazily: the source closure runs inside the level-1 audio
	// goroutine in parallel with TTS, and captures the decoded buffer so
	// child levels can reuse it without re-decoding.
	var (
		samples []float64
		rate    uint32
	)
	src := func() ([]float64, uint32, error) {
		s, r, err := ffmpeg.Decode(audioPath, rc.FFMpegSampleRate, rc.FFMpegPath)
		if err != nil {
			return nil, 0, err
		}
		samples = s
		rate = r
		return s, r, nil
	}

	// Shared across the recursion: siblings at each level reuse one
	// *mfcc.Compiled per (params, rate) instead of rebuilding per call.
	cache := newCompiledCache()

	topSM, err := executeWithSource(src, timing.Zero, topFrags, tc, rc, true, cache)
	if err != nil {
		return nil, err
	}

	// If granularity stays at 1 or the tree is already flat, we're done.
	if maxLevel < 2 || allLeavesAtLevel1(topNodes) {
		return topSM, nil
	}

	// Build a hierarchical sync map: clone the input text tree's shape,
	// fill top level from topSM, then recurse for each parent with children.
	out := syncmap.NewSyncMap()
	topFragsByID := make(map[string]*syncmap.SyncMapFragment, len(topSM.Fragments()))
	for _, sf := range topSM.Fragments() {
		topFragsByID[sf.Identifier()] = sf
	}

	for _, topNode := range topNodes {
		topFrag, _ := topNode.Value.(*text.Fragment)
		if topFrag == nil {
			continue
		}
		sf := topFragsByID[topFrag.Identifier]
		if sf == nil {
			continue
		}
		parentNode := syncmap.NewTree(sf)
		out.Tree.AddChild(parentNode, true)

		if maxLevel >= 2 && len(topNode.ChildrenNotEmpty()) > 0 {
			if err := alignChildren(parentNode, topNode, samples, rate, sf,
				tc, rc, 2, maxLevel, cache); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// alignChildren aligns the children of textParent inside the audio
// window defined by parentFrag's [begin, end]. The resulting
// SyncMapFragments are attached to outParent.
//
// level is the current depth being aligned (2 = sentences, 3 = phrases,
// 4 = words). maxLevel caps recursion. cache is shared across the whole
// ExecuteMultilevel call so siblings reuse one *mfcc.Compiled.
func alignChildren(
	outParent *syncmap.Tree,
	textParent *syncmap.Tree,
	samples []float64,
	rate uint32,
	parentFrag *syncmap.SyncMapFragment,
	tc TaskConfig,
	rc config.RuntimeConfig,
	level, maxLevel int,
	cache *compiledCache,
) error {
	childNodes := textParent.ChildrenNotEmpty()
	childFrags := make([]*text.Fragment, 0, len(childNodes))
	for _, n := range childNodes {
		if f, ok := n.Value.(*text.Fragment); ok {
			childFrags = append(childFrags, f)
		}
	}
	if len(childFrags) == 0 {
		return nil
	}

	// Slice the parent's audio range out of the decoded samples.
	beginSec := parentFrag.Begin().Float64()
	endSec := parentFrag.End().Float64()
	beginIdx := int(beginSec * float64(rate))
	endIdx := int(endSec * float64(rate))
	if beginIdx < 0 {
		beginIdx = 0
	}
	if endIdx > len(samples) {
		endIdx = len(samples)
	}
	if endIdx <= beginIdx {
		// Empty audio window — distribute children evenly so the syncmap is
		// well-formed even though we couldn't align.
		evenly(outParent, childFrags, parentFrag.Begin(), parentFrag.End())
		return nil
	}
	childSamples := samples[beginIdx:endIdx]
	audioOffset := parentFrag.Begin()

	// Use the level-specific MFCC + DTW parameters.
	childRC := rc
	childRC.SetGranularity(level)

	childSM, err := executeWithSource(staticSamples(childSamples, rate), audioOffset,
		childFrags, tc, childRC, false /* no head/tail at inner levels */, cache)
	if err != nil {
		return fmt.Errorf("pipeline: level %d alignment for %q: %w",
			level, parentFrag.Identifier(), err)
	}

	// Attach each child sync-map fragment under outParent and recurse if
	// the text tree has another level below.
	childByID := make(map[string]*syncmap.SyncMapFragment, len(childSM.Fragments()))
	for _, c := range childSM.Fragments() {
		childByID[c.Identifier()] = c
	}
	for _, childTextNode := range childNodes {
		childTextFrag, _ := childTextNode.Value.(*text.Fragment)
		if childTextFrag == nil {
			continue
		}
		csf := childByID[childTextFrag.Identifier]
		if csf == nil {
			continue
		}
		childOutNode := syncmap.NewTree(csf)
		outParent.AddChild(childOutNode, true)

		if level < maxLevel && len(childTextNode.ChildrenNotEmpty()) > 0 {
			if err := alignChildren(childOutNode, childTextNode, samples, rate, csf,
				tc, rc, level+1, maxLevel, cache); err != nil {
				return err
			}
		}
	}
	return nil
}

// allLeavesAtLevel1 returns true if no top-level text node has children.
// In that case multi-level alignment is a no-op and the single-level
// result is the final answer.
func allLeavesAtLevel1(topNodes []*syncmap.Tree) bool {
	for _, n := range topNodes {
		if len(n.ChildrenNotEmpty()) > 0 {
			return false
		}
	}
	return true
}

// evenly distributes child fragments uniformly across [begin, end] when
// inner alignment can't run (e.g. degenerate audio slice).
func evenly(parent *syncmap.Tree, frags []*text.Fragment, begin, end timing.TimeValue) {
	n := len(frags)
	if n == 0 {
		return
	}
	span := end.Sub(begin).Float64()
	step := span / float64(n)
	for i, frag := range frags {
		b := begin.Add(timing.FromFloat64(float64(i) * step))
		e := begin.Add(timing.FromFloat64(float64(i+1) * step))
		if i == n-1 {
			e = end
		}
		sf := syncmap.NewSyncMapFragment(frag, b, e, syncmap.Regular)
		parent.AddChild(syncmap.NewTree(sf), true)
	}
}

// headTailLengths returns the head, process, and tail lengths for SetHeadMiddleTail.
// Explicit config values take priority; SD detection is used if detection bounds set.
// espeakPath must already be resolved by the caller.
//
// forwardSynthMFCC, if non-nil, is the forward-direction synthesis MFCC that
// the main alignment pipeline already computed; SD will reuse it for head
// detection to avoid a redundant synth + MFCC pass.
func headTailLengths(
	realMFCC *audiomfcc.AudioMFCC,
	forwardSynthMFCC *audiomfcc.AudioMFCC,
	fragments []*text.Fragment,
	tc TaskConfig,
	rc config.RuntimeConfig,
	mp mfcc.Params,
	espeakPath string,
) (headLen, processLen, tailLen *timing.TimeValue, err error) {
	if tc.AudioHead != nil || tc.AudioProcess != nil || tc.AudioTail != nil {
		return tc.AudioHead, tc.AudioProcess, tc.AudioTail, nil
	}

	zero := timing.Zero
	headLen = &zero
	tail := timing.Zero
	tailLen = &tail

	if tc.HeadMin == nil && tc.HeadMax == nil && tc.TailMin == nil && tc.TailMax == nil {
		return headLen, nil, tailLen, nil
	}

	mws := rc.MFCCWindowShift
	vadP := vad.ParamsFromSeconds(
		rc.VADLogEnergyThreshold,
		rc.VADMinNonspeechLength,
		rc.VADExtendBefore,
		rc.VADExtendAfter,
		mws,
	)
	detector := sd.New(realMFCC, toTTSFragments(fragments, tc.Language), sd.Config{
		MFCCWindowShift:   mws,
		MFCCParams:        mp,
		VADParams:         vadP,
		EspeakPath:        espeakPath,
		CachedForwardMFCC: forwardSynthMFCC,
	})

	if tc.HeadMin != nil || tc.HeadMax != nil {
		minH, maxH := sdBounds(tc.HeadMin, tc.HeadMax)
		h, detErr := detector.DetectHead(minH, maxH)
		if detErr != nil {
			return nil, nil, nil, detErr
		}
		headLen = &h
	}
	if tc.TailMin != nil || tc.TailMax != nil {
		minT, maxT := sdBounds(tc.TailMin, tc.TailMax)
		t, detErr := detector.DetectTail(minT, maxT)
		if detErr != nil {
			return nil, nil, nil, detErr
		}
		tailLen = &t
	}
	return headLen, nil, tailLen, nil
}

func sdBounds(minPtr, maxPtr *timing.TimeValue) (min, max timing.TimeValue) {
	if minPtr != nil {
		min = *minPtr
	}
	if maxPtr != nil {
		max = *maxPtr
	} else {
		max = sd.DefaultMaxLength
	}
	return min, max
}

func toTTSFragments(frags []*text.Fragment, defaultLang string) []tts.Fragment {
	out := make([]tts.Fragment, len(frags))
	for i, f := range frags {
		lang := f.Language
		if lang == "" {
			lang = defaultLang
		}
		txt := f.FilteredText()
		if txt == "" {
			txt = f.Text()
		}
		out[i] = tts.Fragment{Identifier: f.Identifier, Language: lang, Text: txt}
	}
	return out
}

func buildABAParams(tc TaskConfig, rc config.RuntimeConfig) aba.Params {
	algo := aba.Algorithm(tc.ABAlgorithm)
	if algo == "" {
		algo = aba.AUTO
	}
	var value float64
	switch algo {
	case aba.OFFSET:
		value = tc.ABAOffset
	case aba.PERCENT:
		value = tc.ABAPercent
	case aba.AFTERCURRENT:
		value = tc.ABAAftercurrent
	case aba.BEFORENEXT:
		value = tc.ABABeforenext
	case aba.RATE, aba.RATEAGGRESSIVE:
		value = tc.ABARate
	}
	return aba.Params{
		Algorithm:          algo,
		Value:              value,
		NoZero:             tc.ABANoZero,
		NonspeechMinLength: tc.ABANonspeechMin,
		NonspeechString:    tc.ABANonspeechString,
		NonspeechTolerance: rc.ABANonspeechTolerance,
		NoZeroDuration:     rc.ABANoZeroDuration,
	}
}

func rcToMFCCParams(rc config.RuntimeConfig) mfcc.Params {
	cp := rc.MFCCParams()
	return mfcc.Params{
		FilterBankSize: cp.FilterBankSize,
		MFCCSize:       cp.MFCCSize,
		FFTOrder:       cp.FFTOrder,
		LowerFrequency: cp.LowerFrequency,
		UpperFrequency: cp.UpperFrequency,
		EmphasisFactor: cp.EmphasisFactor,
		WindowLength:   cp.WindowLength,
		WindowShift:    cp.WindowShift,
	}
}

var trueVals = map[string]bool{"true": true, "True": true, "TRUE": true, "1": true, "yes": true, "YES": true}

func parseBool(v string) bool { return trueVals[v] }
func parseFloat(v string) float64 {
	f, _ := strconv.ParseFloat(v, 64)
	return f
}
func parseInt(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}
