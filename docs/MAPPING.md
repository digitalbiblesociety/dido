# Python → Go (dido) source map

This file maps each Python module in the upstream [aeneas
library](https://github.com/readbeyond/aeneas) to its Go counterpart
in `dido`. Useful when adding new functionality, debugging
parity gaps, or hunting down the equivalent of a Python feature.

Layout: paths under `aeneas/aeneas/` on the Python side; paths under
`internal/` (and `cmd/`) on the Go side. "—" means not yet ported;
see `IMPROVEMENT_PLANS.md` for the backlog.

## Numeric core

| Python | Go | Notes |
|---|---|---|
| `mfcc.py` (pure-Python) | `internal/mfcc/` | Algorithm matches the C extension, not the pure-Python fallback. |
| `cmfcc/cmfcc_func.c` | `internal/mfcc/`, `internal/mfcc/fft.go` | SPTK FFT + Mel filter bank + DCT. |
| `dtw.py` (pure-Python) | `internal/dtw/` (used as algorithmic reference) | Production path matches the C extension. |
| `cdtw/cdtw_func.c` | `internal/dtw/dtw.go` | Stripe + exact DTW, cost matrix accumulation, backtrace. |
| `cwave/cwave_func.c` | `internal/audio/wav.go` | Mono PCM 16-bit WAV read/write. |
| `vad.py` | `internal/vad/` | Energy-based VAD. Bit-identical mask vs. Python. |
| `sd.py` | `internal/sd/` | Head/tail length detection. |
| `audiofile.py` | `internal/audio/` + `internal/ffmpeg/` | WAV fast path + FFmpeg subprocess for other formats. |
| `audiofilemfcc.py` | `internal/audiomfcc/` | MFCC wrapper with lazy VAD + interval extraction. |
| `wavfile.py` | `internal/audio/wav.go` | The aeneas helper for cwave I/O. |
| `ffmpegwrapper.py` | `internal/ffmpeg/ffmpeg.go` | Audio decode via subprocess. |
| `ffprobewrapper.py` | — | Not used by the Go pipeline (we read the WAV header directly). |

## Text input

| Python | Go | Notes |
|---|---|---|
| `textfile.py` | `internal/text/` (split: `textfile.go`, `fragment.go`, `parsed.go`, `unparsed.go`, `filter.go`) | All input formats parsed (plain, parsed, mplain, munparsed, unparsed, subtitles). |
| `idsortingalgorithm.py` | `internal/language/idsort.go` | Numeric / lexicographic identifier sort orders. |
| `language.py` | `internal/language/language.go` | BCP-47 / ISO-639-3 language code normalisation. |
| `hierarchytype.py` | — (constants live in `internal/text/`) | |

## Synthesis

| Python | Go | Notes |
|--------|---|---|
| `synthesizer.py`                    | `internal/tts/`            | Orchestrator that fans out per-fragment synthesis. |
| `cewsubprocess.py`                  | `internal/tts/espeakng.go` | The subprocess wrapper proper. |
| `ttswrappers/basettswrapper.py`     | `internal/tts/`            | Base class equivalent. |
| `ttswrappers/espeakttswrapper.py`   | —                          | Classic espeak not supported; Go uses espeak-ng only. |
| `ttswrappers/espeakngttswrapper.py` | `internal/tts/espeakng.go` | The canonical Go path. |
| `ttswrappers/festivalttswrapper.py` | —                          | Out of scope (see `plan.md`). |
| `ttswrappers/awsttswrapper.py`      | —                          | Out of scope. |
| `ttswrappers/nuancettswrapper.py`   | —                          | Out of scope. |
| `ttswrappers/macosttswrapper.py`    | —                          | Out of scope. |

## Sync map + serialisation

| Python | Go | Notes |
|---|---|---|
| `syncmap/__init__.py` | `internal/syncmap/syncmap.go` | SyncMap top-level type + Fragments / Leaves. |
| `syncmap/fragment.py` | `internal/syncmap/fragment.go` | SyncMapFragment; typed TextFragment interface (see D1 in IMPROVEMENT_PLANS.md). |
| `syncmap/fragmentlist.py` | — (interval algebra subset only) | Full fragment-list helpers not ported. |
| `tree.py` | `internal/syncmap/tree.go` | Generic levelled tree. |
| `syncmap/headtailformat.py` | — | |
| `syncmap/format.py` (dispatch) | `internal/syncmap/format/format.go` | |
| `syncmap/smfbase.py` | `internal/syncmap/format/` | Base helpers (xmlEscape, time format helpers, etc.). |
| `syncmap/smfgsubtitles.py` | `internal/syncmap/format/subtitles.go` | SRT / VTT / SUB / SBV common engine. |
| `syncmap/smfsrt.py`, `smfvtt.py`, `smfsub.py` | `internal/syncmap/format/subtitles.go` | Variants — composed via `subtitlesConfig`. |
| `syncmap/smfgtabular.py` | `internal/syncmap/format/tabular.go` | CSV / TSV / SSV / TXT / AUD / TAB engine. |
| `syncmap/smfcsv.py`, `smftsv.py`, `smfssv.py`, `smftxt.py`, `smfaudacity.py` | `internal/syncmap/format/tabular.go` | Variants — composed via `tabularConfig`. |
| `syncmap/smfgxml.py` | `internal/syncmap/format/xml.go` | XML / SMIL / TTML / EAF common engine. |
| `syncmap/smfxml.py`, `smfxmllegacy.py` | `internal/syncmap/format/xml.go` | |
| `syncmap/smfsmil.py` | `internal/syncmap/format/xml.go` (formatSMIL) | |
| `syncmap/smfttml.py` | `internal/syncmap/format/xml.go` (formatTTML / DFXP) | |
| `syncmap/smfeaf.py` | `internal/syncmap/format/xml.go` (formatEAF) | |
| `syncmap/smfjson.py` | `internal/syncmap/format/json.go` | |
| `syncmap/smfrbse.py` | `internal/syncmap/format/json.go` (formatRBSE) | |
| `syncmap/smftextgrid.py` | — | Praat TextGrid not yet ported — see A4 in IMPROVEMENT_PLANS.md. |
| `syncmap/missingparametererror.py` | — (errors raised via `fmt.Errorf` in callers) | |

## Time

| Python | Go | Notes |
|---|---|---|
| `exacttiming.py` | `internal/timing/` | `math/big.Rat`-backed TimeValue / TimeInterval. |

## Configuration

| Python | Go | Notes |
|---|---|---|
| `configuration.py` | `internal/config/runtime.go` (`ParseConfigString`) | The pipe-separated key=value parser. |
| `runtimeconfiguration.py` | `internal/config/runtime.go` (`RuntimeConfig` + defaults) | All ~80 runtime keys mirrored. |
| `globalconstants.py` | — (constants embedded inline where used) | |
| `globalfunctions.py` | — (Go's stdlib covers most of these) | |

## Orchestration

| Python | Go | Notes |
|---|---|---|
| `executetask.py` | `internal/pipeline/pipeline.go` | Single-level path only; multi-level is tracked as A3. |
| `executejob.py` | — | Batch job runner not yet ported; tracked as A1. |
| `task.py` | `internal/pipeline/pipeline.go` (`TaskConfig`) | The configuration side ported; the Job/Container side is not. |
| `job.py` | — | See A1. |
| `container.py` | — | See A1. |
| `analyzecontainer.py` | — | See A1. |
| `validator.py` | — | See A1. |
| `adjustboundaryalgorithm.py` | `internal/aba/aba.go` | AFTERCURRENT, BEFORENEXT, OFFSET, PERCENT, RATE, RATEAGGRESSIVE all ported. |

## CLI tools

| Python | Go | Notes |
|---|---|---|
| `tools/execute_task.py` | `cmd/dido/` | The primary single-task CLI. |
| `tools/execute_job.py` | — | Tracked as A1. |
| `tools/validate.py` | — | Tracked as A1. |
| `tools/extract_mfcc.py` | — | Tracked as A1. |
| `tools/synthesize_text.py` | — | Easy add via `internal/tts`; not yet shipped. |
| `tools/read_audio.py`, `read_text.py`, `run_sd.py`, `run_vad.py` | — | Diagnostic tools; not yet shipped. |
| `tools/ffmpeg_wrapper.py`, `ffprobe_wrapper.py` | — | Use `ffmpeg`/`ffprobe` directly. |
| `tools/convert_syncmap.py` | — | Easy add via `internal/syncmap/format`; not yet shipped. |
| `tools/plot_waveform.py` | — | Out of scope per `plan.md`. |
| `tools/download.py` | — | Out of scope per `plan.md`. |
| `tools/hydra.py`, `abstract_cli_program.py` | — | CLI infrastructure not needed in Go (flag/main pattern). |

## Auto-transliteration (Go-only addition)

The Go port adds one feature with no Python counterpart: automatic
romanisation of text whose language has no native eSpeak-ng voice.

| Python | Go | Notes |
|---|---|---|
| — (Python feeds raw bytes to the English voice as a fallback) | `internal/tts/translit.go` (`PrepareTextForSpeech`) | Uses [digitalbiblesociety/transliterate](https://github.com/digitalbiblesociety/transliterate) — the only third-party dependency. Triggered when `IsFallbackVoice(lang) == true` AND `script.Detect(text)` finds a non-Latin script. Disable with `tts_auto_transliterate=false`. |

## Misc

| Python | Go | Notes |
|---|---|---|
| `logger.py` | — | Go's stdlib `log` or contextual `fmt.Errorf` covers callers' needs. |
| `diagnostics.py` | — | Not a runtime concern. |
| `downloader.py`, `plotter.py` | — | Out of scope. |
| `__init__.py` files | — | Go has no equivalent. |

## C extensions

| Python C ext | Go | Notes |
|---|---|---|
| `cmfcc/` (cmfcc_func.c, cmfcc_py.c) | `internal/mfcc/`, `internal/mfcc/fft.go` | Algorithm faithfully ported. The Go output matches a clean numpy SPTK reference bit-for-bit; the C extension shows ~7% per-cell drift from that reference (documented in PARITY_REPORT.md). |
| `cdtw/` (cdtw_func.c, cdtw_py.c) | `internal/dtw/dtw.go` | Bit-identical paths on the canonical fixtures. |
| `cwave/` (cwave_func.c) | `internal/audio/wav.go` | |
| `cint/` (cint.c) | — | uint32_t helpers — not needed; Go has typed ints. |
| `cew/` (libespeak bindings) | — | Replaced by `internal/tts` espeak-ng subprocess. |
| `cfw/` (libfestival bindings) | — | Out of scope. |

## Tests

| Python tests/ file | Go test equivalent |
|---|---|
| `test_cmfcc.py` | `internal/mfcc/mfcc_test.go` + `internal/parity/mfcc_parity_test.go` |
| `test_cdtw.py` | `internal/dtw/dtw_test.go` + `internal/parity/dtw_parity_test.go` |
| `test_vad.py` | `internal/vad/vad_test.go` + `internal/parity/vad_parity_test.go` |
| `test_sd.py` | — (no Go test yet; tracked as E1) |
| `test_audiofile.py` | `internal/audio/wav_test.go` + `internal/parity/wav_parity_test.go` |
| `test_audiofilemfcc.py` | `internal/audiomfcc/audiomfcc_test.go` |
| `test_textfile.py` | `internal/text/text_test.go` (subset) |
| `test_tree.py` | `internal/syncmap/tree_test.go` |
| `test_syncmap*.py` | `internal/syncmap/syncmap_test.go` + `internal/syncmap/format/` test files |
| `test_exacttiming.py` | `internal/timing/time_test.go` |
| `test_configuration.py` / `test_runtimeconfiguration.py` | `internal/config/runtime_test.go` |
| `test_adjustboundaryalgorithm.py` | `internal/aba/aba_test.go` |
| `test_globalfunctions.py`, `test_logger.py` | — |
| `test_idsortingalgorithm.py` | `internal/language/idsort_test.go` |
| End-to-end (`test_task.py`, `test_job.py`) | `tools/psalms-parity/` |
