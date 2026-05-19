// Command dido-sab force-aligns scripture audio against USFM text and
// emits the SAB output files the Scripture App Builder reader consumes:
//
//	<stem>-aeneas.txt           `id|text\n` per phrase, as fed to the
//	                            aligner (Latin if transliterated, source
//	                            script otherwise)
//	<stem>-aeneas-original.txt  `id|text\n` in source script (only when
//	                            transliteration ran and the columns differ)
//	<stem>-timing.txt           SAB header + `start\tend\tid\n` rows
//
// For languages whose script eSpeak-NG can't natively pronounce (Thai,
// Devanagari, Tibetan, etc.), each phrase is romanised before alignment
// so the English voice can read it phonetically.
//
// Three entry shapes:
//
//	# Interactive launcher (no positional args on a TTY): full Bubble
//	# Tea flow — setup form → plan preview → live progress → results
//	# browser. Any flags passed are used as starting values.
//	dido-sab
//
//	# Single book / chapter range:
//	dido-sab -lang=tha -book=ISA -book-seq=23 -chapters=1-3 \
//	    <audio-dir> <usfm-file> <output-dir>
//
//	# Whole-bible batch (auto-discovers books by USFM \id tag and audio
//	# subdir name; renders a Bubble Tea TUI when stdout is a TTY):
//	dido-sab -lang=bod -batch \
//	    <audio-root> <usfm-dir> <output-dir>
//
// Section headings (\s, \s1, …, \sd) are dropped from verse text by
// default. Pass -include-section-headers to append heading text to the
// preceding verse instead.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/digitalbiblesociety/dido/internal/config"
	"github.com/digitalbiblesociety/dido/internal/pipeline"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/usfm"
)

// phraseSeparators is the set of punctuation that splits a verse into
// SAB-style "phrase" fragments. Includes ASCII end-of-clause punctuation
// plus Arabic, CJK, Devanagari, Tibetan/Burmese, Khmer, and Thai variants
// — mirrors the constant in usfm-tools/convert/cmd/audio-sync.
const phraseSeparators = ".?!:;,؟،。！？．，、।॥།။။።៕។ฯ๏๚๛"

// chapterFromName matches the `_NN.mp3` or `_NNN.mp3` suffix on FCBH/DAVR
// audio chapter files: "23_Isaiah_001.mp3" → "001".
var chapterFromName = regexp.MustCompile(`_(\d+)\.mp3$`)

type cliFlags struct {
	lang            string
	book            string
	bookSeq         int
	chapters        string
	chapterMin      int
	chapterMax      int
	nameStyle       string
	dryRun          bool
	includeSections bool
	resume          bool
	batch           bool
	noTUI           bool
	// workers is the requested chapter-level concurrency for batch
	// mode. 0 means "auto" (runtime.NumCPU()), set via -workers or
	// DIDO_BATCH_WORKERS. Resolved at run time by effectiveWorkers().
	workers int
}

func main() {
	var c cliFlags
	flag.StringVar(&c.lang, "lang", "", `aeneas task_language. Pass an espeak-ng voice code (e.g. "eng", "tha"). When the language has no native eSpeak voice and the text is non-Roman, dido auto-transliterates and uses Esperanto phonetics; force this explicitly with -lang=epo.`)
	flag.StringVar(&c.book, "book", "", "USFM book code for the SAB output stem (e.g. ISA). Required in single-book mode; ignored with -batch.")
	flag.IntVar(&c.bookSeq, "book-seq", 0, "1-based book position in the audio scope. Used in single-book mode for the SAB stem `C01-<bookSeq>-<CODE>-<chap>`. Ignored with -batch (derived from the canonical 66-book table).")
	flag.StringVar(&c.chapters, "chapters", "", "Limit to a chapter range, e.g. \"1-5\" or \"1\". Empty = all chapters that have an MP3. Applied per-book in batch mode.")
	flag.StringVar(&c.nameStyle, "name-style", "sab", `Output stem style: "sab" (C01-NN-CODE-CC) or "simple" (CODE-CCC).`)
	flag.BoolVar(&c.dryRun, "dry-run", false, "Print the planned chapter list and exit without aligning.")
	flag.BoolVar(&c.includeSections, "include-section-headers", false, "Append USFM section headings (\\s/\\s1/.../\\sd) to the preceding verse text. Default off — heading text is dropped from the SAB output.")
	flag.BoolVar(&c.resume, "resume", false, "Skip chapters whose -timing.txt output already exists. Useful for retrying after a partial run.")
	flag.BoolVar(&c.batch, "batch", false, "Whole-bible mode: args become <audio-root> <usfm-dir> <output-dir>; auto-discover and align every book.")
	flag.BoolVar(&c.noTUI, "no-tui", false, "In -batch mode, disable the Bubble Tea TUI and print plain log lines instead. Implicit when stdout is not a TTY.")
	flag.IntVar(&c.workers, "workers", 0, "Maximum concurrent chapter alignments in -batch mode. 0 = auto (runtime.NumCPU()). Also settable via DIDO_BATCH_WORKERS.")
	flag.Parse()

	// Interactive TUI: no positional args + TTY on stdout. Flags already
	// parsed are passed in as starting values so the user can run
	// `dido-sab -lang=tha` and skip the language prompt.
	if flag.NArg() == 0 && !c.noTUI && isTerminal(os.Stdout) {
		if err := runTUI(c); err != nil {
			exitf("%v", err)
		}
		return
	}

	if c.lang == "" {
		exitf("flag -lang required (e.g. -lang=tha or -lang=epo)")
	}
	if c.chapters != "" {
		min, max, err := parseChapterRange(c.chapters)
		if err != nil {
			exitf("-chapters: %v", err)
		}
		c.chapterMin = min
		c.chapterMax = max
	}

	if c.batch {
		if flag.NArg() != 3 {
			fmt.Fprintln(os.Stderr, "usage: dido-sab -batch [flags] <audio-root> <usfm-dir> <output-dir>")
			flag.PrintDefaults()
			os.Exit(1)
		}
		audioRoot := flag.Arg(0)
		usfmDir := flag.Arg(1)
		outDir := flag.Arg(2)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			exitf("mkdir %s: %v", outDir, err)
		}
		if err := runBatch(audioRoot, usfmDir, outDir, c); err != nil {
			exitf("%v", err)
		}
		return
	}

	if flag.NArg() != 3 {
		fmt.Fprintln(os.Stderr, "usage: dido-sab [flags] <audio-dir> <usfm-file> <output-dir>")
		flag.PrintDefaults()
		os.Exit(1)
	}
	audioDir := flag.Arg(0)
	usfmPath := flag.Arg(1)
	outDir := flag.Arg(2)

	if c.book == "" {
		exitf("flag -book required (e.g. -book=ISA)")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		exitf("mkdir %s: %v", outDir, err)
	}

	if err := runSingle(audioDir, usfmPath, outDir, c); err != nil {
		exitf("%v", err)
	}
}

func runSingle(audioDir, usfmPath, outDir string, c cliFlags) error {
	allVerses, err := usfm.ParseFileWithOptions(usfmPath, usfm.Options{
		IncludeSectionHeaders: c.includeSections,
	})
	if err != nil {
		return fmt.Errorf("parse USFM: %w", err)
	}

	mp3s, err := filepath.Glob(filepath.Join(audioDir, "*.mp3"))
	if err != nil {
		return fmt.Errorf("scan audio: %w", err)
	}
	sort.Strings(mp3s)
	if len(mp3s) == 0 {
		return fmt.Errorf("no MP3 files in %s", audioDir)
	}

	type task struct {
		chapter  int
		audioMP3 string
		stem     string
	}
	var tasks []task
	for _, mp3 := range mp3s {
		chap := parseChapterNumber(filepath.Base(mp3))
		if chap < 1 {
			continue
		}
		if c.chapterMin > 0 && chap < c.chapterMin {
			continue
		}
		if c.chapterMax > 0 && chap > c.chapterMax {
			continue
		}
		tasks = append(tasks, task{
			chapter:  chap,
			audioMP3: mp3,
			stem:     stemFor(c.nameStyle, c.bookSeq, c.book, chap),
		})
	}
	if len(tasks) == 0 {
		return fmt.Errorf("no chapters in plan (audio=%d files, chapter filter=%q)", len(mp3s), c.chapters)
	}

	fmt.Fprintf(os.Stderr, "Planned: %d chapters (%s ch %d–%d)\n",
		len(tasks), c.book, tasks[0].chapter, tasks[len(tasks)-1].chapter)
	if c.dryRun {
		for _, t := range tasks {
			fmt.Printf("would align: %s → %s/{%s-aeneas.txt,%s-timing.txt}\n",
				filepath.Base(t.audioMP3), outDir, t.stem, t.stem)
		}
		return nil
	}

	rc := config.Default()
	rc.SetGranularity(1)
	rc.SetTTS(1)
	tc := pipeline.TaskConfig{
		Language:   c.lang,
		TextFormat: text.FormatParsed,
	}

	for _, t := range tasks {
		if c.resume {
			timingPath := filepath.Join(outDir, t.stem+"-timing.txt")
			if info, err := os.Stat(timingPath); err == nil && info.Size() > 0 {
				fmt.Fprintf(os.Stderr, "  ↷ %s  (resume: timing file exists)\n", t.stem)
				continue
			}
		}
		res, err := alignChapter(t.audioMP3, allVerses, t.chapter, t.stem, outDir, tc, rc)
		if err != nil {
			return fmt.Errorf("chapter %d (%s): %w", t.chapter, filepath.Base(t.audioMP3), err)
		}
		fmt.Fprintf(os.Stderr, "  ✓ %s  (%d fragments)\n", res.Stem, res.Fragments)
	}
	return nil
}

// parseChapterNumber pulls the chapter number out of "23_Isaiah_001.mp3".
func parseChapterNumber(name string) int {
	m := chapterFromName.FindStringSubmatch(name)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// parseChapterRange parses "N" or "N-M".
func parseChapterRange(s string) (min, max int, err error) {
	if dash := strings.IndexByte(s, '-'); dash > 0 {
		min, _ = strconv.Atoi(strings.TrimSpace(s[:dash]))
		max, _ = strconv.Atoi(strings.TrimSpace(s[dash+1:]))
	} else {
		min, _ = strconv.Atoi(strings.TrimSpace(s))
		max = min
	}
	if min <= 0 || max < min {
		return 0, 0, fmt.Errorf("invalid range %q", s)
	}
	return min, max, nil
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
