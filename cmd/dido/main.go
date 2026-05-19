// Command dido is the forced audio/text alignment CLI for the
// digitalbiblesociety/dido project. It is the Go-port equivalent of
// `python -m aeneas.tools.execute_task`.
//
// Usage: dido <audio_file> <text_file> <config_string> <output_file>
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/digitalbiblesociety/dido/internal/config"
	"github.com/digitalbiblesociety/dido/internal/pipeline"
	"github.com/digitalbiblesociety/dido/internal/syncmap"
	"github.com/digitalbiblesociety/dido/internal/syncmap/format"
	"github.com/digitalbiblesociety/dido/internal/text"
)

// isHierarchicalFormat returns true if the given text input format produces
// a multi-level fragment tree (and therefore benefits from multi-level
// alignment). Flat formats see Granularity > 1 as a no-op.
func isHierarchicalFormat(f text.Format) bool {
	return f == text.FormatMPlain || f == text.FormatMUnparsed
}

const version = "0.1.0-dev"

const usage = `dido %s — forced audio/text alignment

USAGE
  dido <audio_file> <text_file> <config_string> <output_file>
  dido --batch <batch_file.json>
  dido --version

ARGUMENTS
  audio_file       Mono PCM WAV at 16 kHz, or any audio FFmpeg can decode.
  text_file        Plain-text file; one fragment per line by default.
  config_string    Pipe-separated key=value pairs (see below).
  output_file      Where to write the sync map.

REQUIRED CONFIG KEY
  task_language=<bcp-47>      e.g. eng, fra, deu, eng-gb. Required.

COMMONLY-USED TASK KEYS
  is_text_type=<plain|parsed|mplain|munparsed|unparsed|subtitles>
                              Input text format. Default: plain.
  os_task_file_format=<fmt>   Output format (see "OUTPUT FORMATS"). Default: json.
  task_granularity=<1|2|3>    Deepest alignment level when the text format
                              is hierarchical (mplain / munparsed). 1 =
                              paragraph only (default); 2 = + sentence;
                              3 = + word. No effect on flat text formats.
  is_audio_file_detect_head_min=<seconds>
  is_audio_file_detect_head_max=<seconds>
                              Auto-detect head silence/intro; min ≤ head ≤ max.
  is_audio_file_detect_tail_min=<seconds>
  is_audio_file_detect_tail_max=<seconds>
                              Auto-detect tail silence/outro.

COMMONLY-USED RUNTIME KEYS
  dtw_algorithm=<stripe|exact>      DTW algorithm. Default: stripe.
  dtw_margin=<seconds>              Stripe half-width. Default: 60.
  mfcc_window_length=<seconds>      MFCC analysis window. Default: 0.100.
  mfcc_window_shift=<seconds>       MFCC frame shift. Default: 0.040.
  vad_log_energy_threshold=<float>  VAD speech threshold. Default: 0.699.
  vad_min_nonspeech_length=<sec>    Min silence run length. Default: 0.200.
  tts_concurrency=<n>               Max parallel espeak-ng processes
                                    (0 = NumCPU or DIDO_TTS_WORKERS env).
  tts_auto_transliterate=<bool>     Default: true. When a fragment's
                                    language has no native espeak-ng
                                    voice AND the text is in a non-
                                    Latin script (Greek, Hebrew,
                                    Devanagari, Syriac, etc.),
                                    romanise the text before
                                    synthesis. Set to false to keep
                                    raw text (the English voice will
                                    mispronounce it but the timestamps
                                    will still be in the audio's frame
                                    of reference).
  ffmpeg_path=<path>                Override ffmpeg binary (default "ffmpeg").

OUTPUT FORMATS
  %s

BATCH MODE
  dido --batch <batch_file.json>
                              Run multiple tasks defined in a JSON file.
                              The schema matches SIL go-aeneas and SAB's
                              AeneasTask serialisation; each task has the
                              fields description / audioFilename /
                              phraseFilename / parameters / outputFilename.
                              Both a bare top-level array and a wrapped
                              {"tasks":[...]} object are accepted.

ENVIRONMENT
  DIDO_TTS_WORKERS=<n>      Cap on parallel TTS subprocesses (0 = NumCPU).
                              Overridable per-task via tts_concurrency.
  DIDO_BATCH_WORKERS=<n>    Parallel tasks in --batch mode (default 2).
                              Each task already fans MFCC/DTW out across
                              NumCPU, so >2 oversubscribes the cores.

EXAMPLES
  dido audio.wav text.txt \
    "task_language=eng|is_text_type=plain|os_task_file_format=srt" \
    output.srt

  dido long_audio.mp3 chapters.txt \
    "task_language=eng|os_task_file_format=json|is_audio_file_detect_head_min=0|is_audio_file_detect_head_max=15" \
    output.json
`

func printUsage() {
	formats := strings.Join(format.SupportedFormats(), ", ")
	fmt.Fprintf(os.Stderr, usage, version, formats)
}

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("dido %s\n", version)
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printUsage()
		return
	}
	// Batch mode: `dido --batch <file.json>`. Matched before the
	// 4-positional-args path so a stray --batch in argv[1] isn't
	// silently treated as an audio filename.
	if len(os.Args) == 3 && os.Args[1] == "--batch" {
		if err := runBatch(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 5 {
		printUsage()
		os.Exit(1)
	}

	audioPath := os.Args[1]
	textPath := os.Args[2]
	configStr := os.Args[3]
	outputPath := os.Args[4]

	if err := run(audioPath, textPath, configStr, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(audioPath, textPath, configStr, outputPath string) error {
	// Parse config: task-level keys (language, format, aba) and runtime keys.
	tc, tcUnknown := pipeline.ParseTaskConfigStrict(configStr)
	rc, rcUnknown := config.ParseStrict(configStr)
	// A key may be task-level OR runtime — only flag those recognised by
	// neither parser.
	if unknown := intersectUnknown(tcUnknown, rcUnknown); len(unknown) > 0 {
		fmt.Fprintf(os.Stderr,
			"warning: %d unrecognised config key(s) ignored: %v\n", len(unknown), unknown)
	}
	// Level-1 granularity is the default for single-level tasks.
	rc.SetGranularity(1)
	rc.SetTTS(1)

	if tc.Language == "" {
		return fmt.Errorf("config must include task_language (e.g. task_language=eng)")
	}
	if tc.TextFormat == "" {
		tc.TextFormat = text.FormatPlain
	}

	// Read text file.
	textParams := text.Params{
		IDFormat:           text.DefaultIDFormat,
		IgnoreRegex:        tc.TextIgnoreRegex,
		TranslitMapPath:    tc.TextTranslitMapPath,
	}
	tf, err := text.ReadFile(textPath, tc.TextFormat, textParams)
	if err != nil {
		return fmt.Errorf("read text file: %w", err)
	}
	fragments := tf.Fragments()
	if len(fragments) == 0 {
		return fmt.Errorf("text file contains no fragments")
	}

	// Set language on top-level fragments that don't already carry one.
	// Hierarchical inputs (mplain, munparsed) inherit the top-level
	// language during alignment.
	for _, f := range fragments {
		if f.Language == "" {
			f.Language = tc.Language
		}
	}

	// Execute pipeline. Use the multi-level driver when granularity > 1
	// AND the input is a hierarchical text format (mplain / munparsed);
	// otherwise the flat Execute path is the right and faster choice.
	var sm *syncmap.SyncMap
	if tc.Granularity > 1 && isHierarchicalFormat(tc.TextFormat) {
		sm, err = pipeline.ExecuteMultilevel(audioPath, tf, tc, rc)
	} else {
		sm, err = pipeline.Execute(audioPath, fragments, tc, rc)
	}
	if err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}

	// Apply os_task_file_head_tail_format BEFORE serialisation so every
	// output format (TSV, JSON, SRT, SMIL, …) sees the same fragment
	// list. SAB always sets this key to "hidden"; default "" leaves the
	// HEAD/TAIL rows in place for aeneas parity.
	sm.ApplyHeadTailFormat(syncmap.HeadTailFormat(tc.HeadTailFormat))

	// Determine output format.
	outFmt := format.Format(tc.OutputFormat)
	if outFmt == "" {
		outFmt = format.FormatJSON
	}

	smilP := format.SMILParams{
		AudioRef: tc.SMILAudioRef,
		PageRef:  tc.SMILPageRef,
	}
	eafP := format.EAFParams{AudioRef: tc.EAFAudioRef}

	result, err := format.Write(sm, outFmt, smilP, eafP)
	if err != nil {
		return fmt.Errorf("format output: %w", err)
	}

	if err := os.WriteFile(outputPath, []byte(result), 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// intersectUnknown returns the keys that appear in both lists — i.e. keys
// not recognised by either the task parser or the runtime parser. The two
// inputs are assumed sorted.
func intersectUnknown(a, b []string) []string {
	set := make(map[string]bool, len(a))
	for _, k := range a {
		set[k] = true
	}
	var out []string
	for _, k := range b {
		if set[k] {
			out = append(out, k)
		}
	}
	return out
}
