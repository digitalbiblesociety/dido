package tts

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/digitalbiblesociety/dido/internal/audio"
	"github.com/digitalbiblesociety/dido/internal/timing"
)

const (
	EngineEspeakNG = "espeak-ng"
	EngineEspeak   = "espeak"
)

const DefaultEngine = EngineEspeakNG

// ResolveBinary returns the executable path for engine. Priority:
// override → engine-specific env var (DIDO_ESPEAK_NG_PATH or
// DIDO_ESPEAK_PATH) → PATH lookup. Empty engine uses DefaultEngine.
func ResolveBinary(engine, override string) (string, error) {
	if engine == "" {
		engine = DefaultEngine
	}
	if override != "" {
		return override, nil
	}
	switch engine {
	case EngineEspeakNG:
		if env := os.Getenv("DIDO_ESPEAK_NG_PATH"); env != "" {
			return env, nil
		}
		p, err := exec.LookPath("espeak-ng")
		if err != nil {
			return "", fmt.Errorf("tts: espeak-ng not found in PATH: %w", err)
		}
		return p, nil
	case EngineEspeak:
		if env := os.Getenv("DIDO_ESPEAK_PATH"); env != "" {
			return env, nil
		}
		p, err := exec.LookPath("espeak")
		if err != nil {
			return "", fmt.Errorf("tts: espeak not found in PATH: %w", err)
		}
		return p, nil
	default:
		return "", fmt.Errorf("tts: unknown engine %q (want %q or %q)",
			engine, EngineEspeak, EngineEspeakNG)
	}
}

// ResolveBinaryPath is the espeak-ng-only legacy shim; prefer ResolveBinary.
func ResolveBinaryPath(override string) (string, error) {
	return ResolveBinary(EngineEspeakNG, override)
}

// Fragment is a single text fragment to synthesize.
type Fragment struct {
	Identifier string
	Language   string
	Text       string // filtered text (empty → zero-duration silence)
}

// SynthesisError is returned by SynthesizeMultiple when a single fragment
// fails to synthesise. It carries enough context (fragment id, language,
// resolved voice, and the offending text) to make a failed espeak-ng call
// debuggable from logs alone. The underlying espeak-ng error is reachable
// via errors.Unwrap.
type SynthesisError struct {
	FragmentID string
	Language   string
	Voice      string
	Text       string
	Err        error
}

// MaxTextSnippet is the maximum number of runes from Text shown in the
// SynthesisError message. Longer text is truncated with an ellipsis.
const MaxTextSnippet = 80

func (e *SynthesisError) Error() string {
	txt := e.Text
	if len([]rune(txt)) > MaxTextSnippet {
		// Truncate by rune boundary, not byte.
		r := []rune(txt)
		txt = string(r[:MaxTextSnippet-1]) + "…"
	}
	id := e.FragmentID
	if id == "" {
		id = "(unnamed)"
	}
	return fmt.Sprintf("tts: synthesise fragment %q (lang=%s, voice=%s, text=%q): %v",
		id, e.Language, e.Voice, txt, e.Err)
}

func (e *SynthesisError) Unwrap() error { return e.Err }

// Result is returned by SynthesizeMultiple.
type Result struct {
	// SampleRate is always DefaultSampleRate (22050) for eSpeak-ng.
	SampleRate uint32
	// Samples is the concatenated PCM audio for all fragments, normalised to [-1,1].
	Samples []float64
	// Intervals holds one entry per input fragment. Begin/End are in seconds
	// relative to the start of Samples.
	Intervals []timing.TimeInterval
}

// SynthesizeMultiple synthesizes each fragment and returns the combined audio
// with per-fragment time intervals. Non-empty fragments are synthesized in
// parallel (default: up to runtime.NumCPU() concurrent espeak-ng processes,
// override via the DIDO_TTS_WORKERS env var or SynthesizeMultipleWith);
// results are merged in fragment order so intervals remain contiguous.
//
// binaryPath is the path to the espeak-ng binary; pass "" to resolve from PATH.
//
// Empty-text fragments produce a zero-duration interval (no audio samples are
// appended for them), matching aeneas Python behaviour.
func SynthesizeMultiple(fragments []Fragment, binaryPath string) (*Result, error) {
	return SynthesizeMultipleWith(fragments, binaryPath, 0)
}

// SynthesizeMultipleWith is SynthesizeMultiple with an explicit concurrency
// cap. Pass 0 to use the default (DIDO_TTS_WORKERS env var, else NumCPU).
func SynthesizeMultipleWith(fragments []Fragment, binaryPath string, workers int) (*Result, error) {
	binary, err := ResolveBinaryPath(binaryPath)
	if err != nil {
		return nil, err
	}

	type work struct {
		samples []float64
		err     error
	}
	works := make([]work, len(fragments))

	concurrency := resolveTTSConcurrency(workers)
	sem := make(chan struct{}, concurrency)

	var wg sync.WaitGroup
	for i, frag := range fragments {
		if frag.Text == "" {
			continue
		}
		wg.Add(1)
		go func(i int, frag Fragment) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// Auto-transliterate non-Latin text when the assigned voice
			// is the English fallback. No-op for languages with a
			// native voice or for already-Latin text.
			textForSpeech, _ := PrepareTextForSpeech(frag.Text, frag.Language)
			s, e := synthesizeOne(binary, VoiceFor(frag.Language), textForSpeech)
			works[i] = work{s, e}
		}(i, frag)
	}
	wg.Wait()

	// Collect first error (preserves deterministic reporting).
	for i, w := range works {
		if w.err != nil {
			frag := fragments[i]
			return nil, &SynthesisError{
				FragmentID: frag.Identifier,
				Language:   frag.Language,
				Voice:      VoiceFor(frag.Language),
				Text:       frag.Text,
				Err:        w.err,
			}
		}
	}

	// Build result in fragment order so intervals are contiguous.
	res := &Result{
		SampleRate: DefaultSampleRate,
		Intervals:  make([]timing.TimeInterval, len(fragments)),
	}
	var allSamples []float64
	cur := uint64(0)
	for i := range fragments {
		begin := sampleToTime(cur, DefaultSampleRate)
		if works[i].samples != nil {
			allSamples = append(allSamples, works[i].samples...)
			cur += uint64(len(works[i].samples))
		}
		res.Intervals[i] = timing.NewTimeInterval(begin, sampleToTime(cur, DefaultSampleRate))
	}
	res.Samples = allSamples
	return res, nil
}

// synthesizeOne invokes espeak-ng for a single (voice, text) pair and returns
// the normalised PCM float64 samples from the output WAV. WAV is piped on
// stdout and decoded via audio.OpenBytes (which clamps eSpeak-ng's
// ~0x7FFFFFFF streaming-header sentinels to the actual buffer length).
func synthesizeOne(binary, voice, text string) ([]float64, error) {
	cmd := exec.Command(binary, "-v", voice, "--stdout")
	cmd.Stdin = strings.NewReader(text)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tts: %s: %w\n%s", binary, err, stderr.Bytes())
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("tts: %s produced no audio (stderr: %s)", binary, stderr.Bytes())
	}
	wf, err := audio.OpenBytes(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("tts: parse WAV: %w", err)
	}
	defer wf.Close()
	samples, err := wf.ReadAllSamples()
	if err != nil {
		return nil, fmt.Errorf("tts: read WAV: %w", err)
	}
	return samples, nil
}

// sampleToTime converts a sample index to a TimeValue at the given rate.
func sampleToTime(sample uint64, rate uint32) timing.TimeValue {
	return timing.FromInt64(int64(sample), int64(rate))
}

// resolveTTSConcurrency chooses how many parallel espeak-ng subprocesses to
// allow. Priority:
//  1. The explicit `workers` argument when > 0.
//  2. DIDO_TTS_WORKERS env var (positive integer).
//  3. runtime.NumCPU().
// Always returns ≥ 1.
func resolveTTSConcurrency(workers int) int {
	if workers > 0 {
		return workers
	}
	if v := os.Getenv("DIDO_TTS_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if n := runtime.NumCPU(); n > 0 {
		return n
	}
	return 1
}

