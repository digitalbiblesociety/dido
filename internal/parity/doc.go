// Package parity provides a Go-side harness for invoking the upstream
// Python aeneas implementation and comparing its outputs against the
// Go port on identical inputs.
//
// Architecture
//
// The harness is structured around a small Python helper script
// (pyhelper.py) that accepts a JSON request on stdin and writes a JSON
// response on stdout. The protocol is documented at the top of that
// script. Tests interact with it through two Go-side entry points:
//
//   RunOnce(req, &resp)
//     Spawns a fresh python3 subprocess for a single request/response
//     cycle. The simplest API; right for one-shot use in a test.
//
//   StartServer() + (*Server).Do(req, &resp) + (*Server).Close()
//     Keeps a long-lived python3 process running in --server mode and
//     issues newline-delimited JSON requests over a pipe. The Python
//     startup cost (~150 ms) is amortised across the run; right for
//     benchmarks and parity test loops with many fragments.
//
// Operations
//
// pyhelper.py currently exposes:
//
//   mfcc         Compute MFCC from a WAV path (uses aeneas's AudioFile,
//                including FFmpeg downsampling)
//   mfcc_data    Compute MFCC from inline float64 samples (no resampling)
//   dtw_path     Run cdtw.compute_best_path on two MFCC text files
//   dtw_exact    Pure-Python exact DTW (slow; for cross-checks)
//   vad          Run aeneas.vad.VAD.run_vad on an energy vector
//   wav_samples  Read raw samples via scipy.io.wavfile (bypasses FFmpeg)
//   audiofile_mfcc  Construct an AudioFileMFCC and return its shape
//
// Both sides exchange float64 arrays losslessly through JSON. Test
// fixtures live under testdata/ (symlinks into the parent aeneas repo).
//
// Availability
//
// CheckAvailable() / SkipIfUnavailable(t) detect whether Python aeneas
// can be imported and skip the test cleanly if not. The harness is
// intentionally a build-time dependency only: dido has no Python
// requirement at runtime.
//
// See docs/PARITY_REPORT.md for the latest numerical agreement and
// speed-up numbers across the standard benchmark set. The
// BenchmarkPsalmsBook* benchmarks in psalms_book_bench_test.go cover
// end-to-end alignment parity on a real-world corpus.
package parity
