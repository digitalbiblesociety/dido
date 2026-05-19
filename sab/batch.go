package main

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/digitalbiblesociety/dido/internal/config"
	"github.com/digitalbiblesociety/dido/internal/dtw"
	"github.com/digitalbiblesociety/dido/internal/mfcc"
	"github.com/digitalbiblesociety/dido/internal/pipeline"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/usfm"
)

// book is one row of the canonical 66-book table: 1-based position, USFM
// code, and the English short-name used in FCBH/DAVR audio directory
// naming (`<NN>_<EnglishName>`).
type book struct {
	Seq  int
	Code string
	Name string
}

// canonBooks is the Protestant 66-book order. Names match the directory
// stems FCBH/DAVR ship (`01_Genesis`, `09_1Samuel`, `22_SongofSongs`,
// `40_Matthew`, …) so that an audio drop can be located by sequence
// number alone.
var canonBooks = []book{
	{1, "GEN", "Genesis"}, {2, "EXO", "Exodus"}, {3, "LEV", "Leviticus"}, {4, "NUM", "Numbers"},
	{5, "DEU", "Deuteronomy"}, {6, "JOS", "Joshua"}, {7, "JDG", "Judges"}, {8, "RUT", "Ruth"},
	{9, "1SA", "1Samuel"}, {10, "2SA", "2Samuel"}, {11, "1KI", "1Kings"}, {12, "2KI", "2Kings"},
	{13, "1CH", "1Chronicles"}, {14, "2CH", "2Chronicles"}, {15, "EZR", "Ezra"}, {16, "NEH", "Nehemiah"},
	{17, "EST", "Esther"}, {18, "JOB", "Job"}, {19, "PSA", "Psalms"}, {20, "PRO", "Proverbs"},
	{21, "ECC", "Ecclesiastes"}, {22, "SNG", "SongofSongs"}, {23, "ISA", "Isaiah"}, {24, "JER", "Jeremiah"},
	{25, "LAM", "Lamentations"}, {26, "EZK", "Ezekiel"}, {27, "DAN", "Daniel"}, {28, "HOS", "Hosea"},
	{29, "JOL", "Joel"}, {30, "AMO", "Amos"}, {31, "OBA", "Obadiah"}, {32, "JON", "Jonah"},
	{33, "MIC", "Micah"}, {34, "NAM", "Nahum"}, {35, "HAB", "Habakkuk"}, {36, "ZEP", "Zephaniah"},
	{37, "HAG", "Haggai"}, {38, "ZEC", "Zechariah"}, {39, "MAL", "Malachi"},
	{40, "MAT", "Matthew"}, {41, "MRK", "Mark"}, {42, "LUK", "Luke"}, {43, "JHN", "John"},
	{44, "ACT", "Acts"}, {45, "ROM", "Romans"}, {46, "1CO", "1Corinthians"}, {47, "2CO", "2Corinthians"},
	{48, "GAL", "Galatians"}, {49, "EPH", "Ephesians"}, {50, "PHP", "Philippians"}, {51, "COL", "Colossians"},
	{52, "1TH", "1Thessalonians"}, {53, "2TH", "2Thessalonians"}, {54, "1TI", "1Timothy"}, {55, "2TI", "2Timothy"},
	{56, "TIT", "Titus"}, {57, "PHM", "Philemon"}, {58, "HEB", "Hebrews"}, {59, "JAS", "James"},
	{60, "1PE", "1Peter"}, {61, "2PE", "2Peter"}, {62, "1JN", "1John"}, {63, "2JN", "2John"},
	{64, "3JN", "3John"}, {65, "JUD", "Jude"}, {66, "REV", "Revelation"},
}

// codeAliases maps non-canonical USFM codes seen in the wild to their
// canonical equivalents. Most live drops use the SIL/UBS short forms,
// but a handful of older USFM files still ship the three-letter Paratext
// abbreviations (PSS for Psalms, EZE for Ezekiel, JUE for Jude, …).
var codeAliases = map[string]string{
	"PSS": "PSA",
	"PRV": "PRO",
	"SOS": "SNG",
	"SOL": "SNG",
	"CAN": "SNG", // Canticles
	"EZE": "EZK",
	"JOE": "JOL",
	"AMS": "AMO",
	"OBD": "OBA",
	"NAH": "NAM",
	"ZEP": "ZEP",
	"MRK": "MRK",
	"MAR": "MRK",
	"JHN": "JHN",
	"JOH": "JHN",
	"1CR": "1CO",
	"2CR": "2CO",
	"PHI": "PHP",
	"PHIL": "PHP",
	"1TS": "1TH",
	"2TS": "2TH",
	"PHN": "PHM",
	"PLM": "PHM",
	"JAM": "JAS",
	"1JO": "1JN",
	"2JO": "2JN",
	"3JO": "3JN",
	"JDE": "JUD",
	"JUE": "JUD",
}

// chapterTask is one chapter of one book ready to align.
type chapterTask struct {
	Chapter  int
	AudioMP3 string
	Stem     string
}

// bookPlan is everything needed to align one book end-to-end.
type bookPlan struct {
	book
	USFM     string
	AudioDir string
	Chapters []chapterTask
}

// progressEvent is emitted by the batch runner and consumed by either
// the plain text logger or the Bubble Tea TUI. One channel handles all
// transitions; consumers switch on Kind.
type progressEvent struct {
	Kind    string // "batch_start" "book_start" "book_skip" "chapter_done" "chapter_resume" "chapter_err" "book_done" "book_failed" "batch_done"
	Total   int    // for batch_start: number of books in the plan
	Book    book
	Chap    int // 1-based chapter
	NumChap int // total chapters in the book (for book_start)
	Frags   int // fragments aligned in chapter_done
	Reason  string
	Err     error
	Elapsed time.Duration
}

// progressSink consumes progressEvents. Two implementations: plainLogger
// (stderr lines) and tuiSink (sends to a Bubble Tea program).
type progressSink interface {
	Send(progressEvent)
	Close()
}

// runBatch is the entry point for `dido-sab -batch`. It discovers book
// pairings, builds the chapter plan, and runs alignment with progress
// streamed to either a TUI or plain logger.
func runBatch(audioRoot, usfmDir, outDir string, c cliFlags) error {
	plans, err := discoverBooks(audioRoot, usfmDir, c)
	if err != nil {
		return err
	}
	if len(plans) == 0 {
		return fmt.Errorf("no book pairings found under audio=%s usfm=%s", audioRoot, usfmDir)
	}

	if c.dryRun {
		for _, p := range plans {
			fmt.Printf("%2d  %-3s %-15s  %d chapters  (usfm=%s, audio=%s)\n",
				p.Seq, p.Code, p.Name, len(p.Chapters),
				filepath.Base(p.USFM), filepath.Base(p.AudioDir))
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

	useTUI := !c.noTUI && isTerminal(os.Stdout)
	if useTUI {
		sink, tuiDone := newTUISink(plans)
		go func() {
			defer sink.Close()
			runBatchWorker(plans, outDir, c, tc, rc, sink)
		}()
		<-tuiDone
		return nil
	}

	sink := newPlainSink(os.Stderr)
	runBatchWorker(plans, outDir, c, tc, rc, sink)
	sink.Close()
	return nil
}

// chapterAligner is the seam between runBatchWorker and the real
// alignChapter() implementation, so tests can inject a deterministic
// fake without spawning ffmpeg + espeak-ng. Production callers pass
// the literal alignChapter function.
type chapterAligner func(audioMP3 string, verses []usfm.Verse, chapter int,
	stem, outDir string, tc pipeline.TaskConfig, rc config.RuntimeConfig) (chapterResult, error)

// effectiveWorkers resolves the requested worker count into a real
// number. The default (0) is intentionally conservative — 2 — because
// the alignment pipeline is already parallel internally (mfcc + dtw
// fan out across NumCPU goroutines each). Cranking the batch level up
// to NumCPU on top of that just oversubscribes the cores and, on
// external/exFAT storage, makes things slower from disk thrashing.
// Users with fast local SSDs can crank this up via the Settings UI.
func effectiveWorkers(requested int) int {
	if requested <= 0 {
		return defaultBatchWorkers()
	}
	if requested > 64 {
		return 64
	}
	return requested
}

// defaultBatchWorkers is the auto-resolution rule for `Workers = 0`.
// Two concurrent chapters reliably overlap one chapter's subprocess
// I/O (ffmpeg decode, espeak-ng synth) with another chapter's compute
// without saturating memory bandwidth. A 1-core machine stays at 1.
func defaultBatchWorkers() int {
	if runtime.NumCPU() <= 1 {
		return 1
	}
	return 2
}

// scaleInnerPipelineWorkers caps the per-chapter MFCC and DTW
// goroutine count so the total compute-bound goroutine count across
// the batch stays around NumCPU. Without this, batch=4 × inner=NumCPU
// spawns 4 × NumCPU goroutines all fighting for the same cores —
// which is exactly the regression that prompted this fix.
//
// Returns a function that restores the previous limits, intended for
// `defer restore()` at the caller. mfcc.MaxWorkers / dtw.CostMatrixMaxWorkers
// are package-level vars so this is a global side effect; safe here
// because a single batch run owns the process.
func scaleInnerPipelineWorkers(batchWorkers int) func() {
	prevMFCC := mfcc.MaxWorkers
	prevDTW := dtw.CostMatrixMaxWorkers
	inner := runtime.NumCPU() / batchWorkers
	if inner < 1 {
		inner = 1
	}
	mfcc.MaxWorkers = inner
	dtw.CostMatrixMaxWorkers = inner
	return func() {
		mfcc.MaxWorkers = prevMFCC
		dtw.CostMatrixMaxWorkers = prevDTW
	}
}

// runBatchWorker is the actual alignment loop. Split out so the TUI
// goroutine can wait on completion while events stream in. The
// dispatch is sequential for workers == 1 (preserving the legacy event
// ordering) and a context-cancellable worker pool otherwise.
func runBatchWorker(plans []bookPlan, outDir string, c cliFlags,
	tc pipeline.TaskConfig, rc config.RuntimeConfig, sink progressSink) {
	runBatchWorkerWith(plans, outDir, c, tc, rc, sink, alignChapter)
}

// runBatchWorkerWith is the injectable variant used by tests.
func runBatchWorkerWith(plans []bookPlan, outDir string, c cliFlags,
	tc pipeline.TaskConfig, rc config.RuntimeConfig, sink progressSink,
	align chapterAligner) {

	sink.Send(progressEvent{Kind: "batch_start", Total: len(plans)})
	batchStart := time.Now()

	workers := effectiveWorkers(c.workers)
	if workers <= 1 {
		runBatchSequential(plans, outDir, c, tc, rc, sink, align)
	} else {
		runBatchParallel(plans, outDir, c, tc, rc, sink, align, workers)
	}

	sink.Send(progressEvent{Kind: "batch_done", Elapsed: time.Since(batchStart)})
}

// runBatchSequential is the legacy code path. It's the safest choice
// when workers == 1 because the event order matches what older tests
// and the plain-text logger already expect.
func runBatchSequential(plans []bookPlan, outDir string, c cliFlags,
	tc pipeline.TaskConfig, rc config.RuntimeConfig, sink progressSink,
	align chapterAligner) {

	for _, p := range plans {
		sink.Send(progressEvent{Kind: "book_start", Book: p.book, NumChap: len(p.Chapters)})

		// One USFM parse per book; reused across chapters.
		verses, err := usfm.ParseFileWithOptions(p.USFM, usfm.Options{
			IncludeSectionHeaders: c.includeSections,
		})
		if err != nil {
			sink.Send(progressEvent{Kind: "book_failed", Book: p.book,
				Err: fmt.Errorf("parse USFM: %w", err)})
			continue
		}

		bookStart := time.Now()
		failed := false
		for _, ct := range p.Chapters {
			if c.resume {
				timingPath := filepath.Join(outDir, ct.Stem+"-timing.txt")
				if info, err := os.Stat(timingPath); err == nil && info.Size() > 0 {
					sink.Send(progressEvent{Kind: "chapter_resume", Book: p.book, Chap: ct.Chapter})
					continue
				}
			}
			chStart := time.Now()
			res, err := align(ct.AudioMP3, verses, ct.Chapter, ct.Stem, outDir, tc, rc)
			if err != nil {
				sink.Send(progressEvent{Kind: "chapter_err", Book: p.book, Chap: ct.Chapter, Err: err})
				failed = true
				break
			}
			sink.Send(progressEvent{Kind: "chapter_done", Book: p.book, Chap: ct.Chapter,
				Frags: res.Fragments, Elapsed: time.Since(chStart)})
		}
		if failed {
			sink.Send(progressEvent{Kind: "book_failed", Book: p.book, Elapsed: time.Since(bookStart)})
		} else {
			sink.Send(progressEvent{Kind: "book_done", Book: p.book, Elapsed: time.Since(bookStart)})
		}
	}
}

// bookCtx tracks per-book state shared by the workers and the
// completion logic. The mutex guards every field below it; the start
// timestamp is captured on the first chapter actually picked up
// (rather than at dispatch time) so the "elapsed" reported to the UI
// reflects real work, not queue latency.
type bookCtx struct {
	plan   bookPlan
	verses []usfm.Verse

	mu         sync.Mutex
	startedAt  time.Time
	started    bool
	chapsDone  int
	chapsTotal int
	failed     bool
	finalised  bool // book_done / book_failed already emitted
}

// runBatchParallel fans chapter-level jobs out across `workers`
// goroutines. The first chapter_err triggers ctxCancel so in-flight
// chapters get a chance to abandon their pipeline calls early and
// queued chapters are dropped. The "abort whole batch" policy is the
// user-selected default; books with no chapters dequeued before the
// cancel stay "pending" in the TUI, which is honest.
func runBatchParallel(plans []bookPlan, outDir string, c cliFlags,
	tc pipeline.TaskConfig, rc config.RuntimeConfig, sink progressSink,
	align chapterAligner, workers int) {

	// Divide the CPU budget between batch-level workers and the
	// per-chapter MFCC/DTW fan-out so total compute goroutines stay
	// ≈ NumCPU. Without this, workers=4 on an 8-core machine spawns
	// 32 compute goroutines, all thrashing the cores.
	restoreInner := scaleInnerPipelineWorkers(workers)
	defer restoreInner()

	type job struct {
		book *bookCtx
		ct   chapterTask
	}

	// Pre-parse every book's USFM up front. A failure here can't
	// block sibling books — they each enqueue independently.
	bookCtxs := make([]*bookCtx, 0, len(plans))
	for i := range plans {
		p := plans[i]
		verses, err := usfm.ParseFileWithOptions(p.USFM, usfm.Options{
			IncludeSectionHeaders: c.includeSections,
		})
		if err != nil {
			sink.Send(progressEvent{Kind: "book_failed", Book: p.book,
				Err: fmt.Errorf("parse USFM: %w", err)})
			continue
		}
		bookCtxs = append(bookCtxs, &bookCtx{
			plan:       p,
			verses:     verses,
			chapsTotal: len(p.Chapters),
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobsCh := make(chan job)
	var wg sync.WaitGroup

	// finaliseBook emits book_done/failed exactly once per book. The
	// per-book mutex makes the increment-and-check atomic across
	// workers; finalised guards against a double-emit if abort
	// cancellation races a natural last-chapter completion.
	finaliseBook := func(bc *bookCtx) {
		bc.mu.Lock()
		if bc.finalised {
			bc.mu.Unlock()
			return
		}
		// Only emit a "book finished" event for books we actually
		// started — books cancelled before any chapter ran stay in
		// the "pending" UI bucket.
		if !bc.started {
			bc.finalised = true
			bc.mu.Unlock()
			return
		}
		elapsed := time.Since(bc.startedAt)
		fail := bc.failed
		bc.finalised = true
		bc.mu.Unlock()
		if fail {
			sink.Send(progressEvent{Kind: "book_failed", Book: bc.plan.book, Elapsed: elapsed})
		} else {
			sink.Send(progressEvent{Kind: "book_done", Book: bc.plan.book, Elapsed: elapsed})
		}
	}

	// runJob processes one chapter and updates the book counters.
	// All cancellation checkpoints live here; alignChapter itself is
	// uninterruptible from the outside, so we honour ctx only at job
	// boundaries.
	runJob := func(j job) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		bc := j.book

		// First worker to touch this book emits book_start. Hold bc.mu
		// through the Send so siblings can't race past and emit
		// chapter_done before the book_start that announces them.
		bc.mu.Lock()
		if !bc.started {
			bc.started = true
			bc.startedAt = time.Now()
			sink.Send(progressEvent{Kind: "book_start",
				Book: bc.plan.book, NumChap: bc.chapsTotal})
		}
		bc.mu.Unlock()

		if c.resume {
			timingPath := filepath.Join(outDir, j.ct.Stem+"-timing.txt")
			if info, err := os.Stat(timingPath); err == nil && info.Size() > 0 {
				sink.Send(progressEvent{Kind: "chapter_resume",
					Book: bc.plan.book, Chap: j.ct.Chapter})
				bc.mu.Lock()
				bc.chapsDone++
				done := bc.chapsDone == bc.chapsTotal
				bc.mu.Unlock()
				if done {
					finaliseBook(bc)
				}
				return
			}
		}

		chStart := time.Now()
		res, err := align(j.ct.AudioMP3, bc.verses, j.ct.Chapter, j.ct.Stem, outDir, tc, rc)
		if err != nil {
			sink.Send(progressEvent{Kind: "chapter_err",
				Book: bc.plan.book, Chap: j.ct.Chapter, Err: err})
			bc.mu.Lock()
			bc.failed = true
			bc.chapsDone++
			bc.mu.Unlock()
			cancel() // abort-whole-batch policy
			finaliseBook(bc)
			return
		}
		sink.Send(progressEvent{Kind: "chapter_done",
			Book: bc.plan.book, Chap: j.ct.Chapter,
			Frags: res.Fragments, Elapsed: time.Since(chStart)})
		bc.mu.Lock()
		bc.chapsDone++
		done := bc.chapsDone == bc.chapsTotal
		bc.mu.Unlock()
		if done {
			finaliseBook(bc)
		}
	}

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobsCh {
				runJob(j)
			}
		}()
	}

	// Dispatcher: feed every chapter into the queue, bailing out on
	// ctx cancellation so the abort doesn't have to drain a 3000-row
	// queue.
dispatch:
	for _, bc := range bookCtxs {
		for _, ct := range bc.plan.Chapters {
			select {
			case <-ctx.Done():
				break dispatch
			case jobsCh <- job{book: bc, ct: ct}:
			}
		}
	}
	close(jobsCh)
	wg.Wait()

	// Books that started but didn't reach chapsTotal (because of the
	// abort) still need their final event. Books that never started
	// remain "pending" in the UI on purpose.
	for _, bc := range bookCtxs {
		bc.mu.Lock()
		started := bc.started
		finalised := bc.finalised
		bc.mu.Unlock()
		if started && !finalised {
			finaliseBook(bc)
		}
	}
}

// discoverBooks scans the USFM directory and audio root, pairs them by
// canonical code/sequence, and returns a sorted list of bookPlans.
func discoverBooks(audioRoot, usfmDir string, c cliFlags) ([]bookPlan, error) {
	usfmByCode, err := indexUSFM(usfmDir)
	if err != nil {
		return nil, fmt.Errorf("scan USFM: %w", err)
	}
	audioBySeq, err := indexAudioDirs(audioRoot)
	if err != nil {
		return nil, fmt.Errorf("scan audio: %w", err)
	}

	var plans []bookPlan
	for _, b := range canonBooks {
		uPath, hasU := usfmByCode[b.Code]
		aDir, hasA := audioBySeq[b.Seq]
		if !hasU || !hasA {
			continue
		}
		mp3s, err := filepath.Glob(filepath.Join(aDir, "*.mp3"))
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", aDir, err)
		}
		sort.Strings(mp3s)
		if len(mp3s) == 0 {
			continue
		}
		var chs []chapterTask
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
			chs = append(chs, chapterTask{
				Chapter:  chap,
				AudioMP3: mp3,
				Stem:     stemFor(c.nameStyle, b.Seq, b.Code, chap),
			})
		}
		if len(chs) == 0 {
			continue
		}
		plans = append(plans, bookPlan{book: b, USFM: uPath, AudioDir: aDir, Chapters: chs})
	}
	return plans, nil
}

// idLine extracts the book code from a USFM `\id <CODE>` line. Codes are
// usually 3 ASCII letters, occasionally prefixed with a digit (1SA, 1CO).
var idLine = regexp.MustCompile(`(?m)^\s*\\id\s+([A-Z0-9]{2,4})\b`)

// indexUSFM walks usfmDir for *.usfm files and reads each one's `\id`
// header to map canonical book code → path. Codes not in the canonical
// 66-book table (via codeAliases) are dropped silently.
func indexUSFM(usfmDir string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(usfmDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".usfm") && !strings.HasSuffix(name, ".sfm") {
			return nil
		}
		code, err := readIDTag(path)
		if err != nil || code == "" {
			return nil
		}
		if canon, ok := codeAliases[code]; ok {
			code = canon
		}
		if _, isCanon := canonCodeSeq()[code]; !isCanon {
			return nil
		}
		// Prefer the first match for a given code; subsequent duplicates
		// (e.g. revisions) are ignored.
		if _, exists := out[code]; !exists {
			out[code] = path
		}
		return nil
	})
	return out, err
}

// readIDTag reads the leading bytes of a USFM file and returns the value
// of the `\id` marker. Many USFM files have a BOM and/or comment header,
// so we scan up to the first 8 KB rather than only the first line.
func readIDTag(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	buf, _ := br.Peek(8192)
	if m := idLine.FindSubmatch(buf); m != nil {
		return strings.ToUpper(string(m[1])), nil
	}
	return "", nil
}

// audioDirPattern matches `NN_<Name>` or `NNN_<Name>` style chapter-set
// directory names. The capture groups expose the sequence and name.
var audioDirPattern = regexp.MustCompile(`^(\d{1,3})_(.+)$`)

// indexAudioDirs walks audioRoot looking for directories whose name
// matches `NN_<EnglishName>` from the canonical table, and that contain
// at least one *.mp3. Returns sequence → directory path.
func indexAudioDirs(audioRoot string) (map[int]string, error) {
	nameSeq := map[string]int{}
	for _, b := range canonBooks {
		nameSeq[strings.ToLower(b.Name)] = b.Seq
	}
	out := map[int]string{}
	err := filepath.WalkDir(audioRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		m := audioDirPattern.FindStringSubmatch(d.Name())
		if m == nil {
			return nil
		}
		seq, _ := strconvAtoi(m[1])
		canonSeq, ok := nameSeq[strings.ToLower(m[2])]
		if !ok {
			return nil
		}
		// Defensive: the sequence prefix and the English name should agree.
		// If they don't, trust the English name.
		if seq != canonSeq {
			seq = canonSeq
		}
		// Confirm there's at least one MP3 inside before recording.
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		hasMP3 := false
		for _, e := range entries {
			if strings.HasSuffix(strings.ToLower(e.Name()), ".mp3") {
				hasMP3 = true
				break
			}
		}
		if !hasMP3 {
			return nil
		}
		if _, exists := out[seq]; !exists {
			out[seq] = path
		}
		return nil
	})
	return out, err
}

// canonCodeSeq returns a lookup of canonical code → seq.
func canonCodeSeq() map[string]int {
	out := make(map[string]int, len(canonBooks))
	for _, b := range canonBooks {
		out[b.Code] = b.Seq
	}
	return out
}

// strconvAtoi is a tiny wrapper that swallows the error (callers have
// already gated input via a digit-only regex match).
func strconvAtoi(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-digit %q", r)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
