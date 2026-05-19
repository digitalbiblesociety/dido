// Package mfcc computes Mel-frequency cepstral coefficients.
//
// This is a port of aeneas/cmfcc/cmfcc_func.c (the SPTK-style FFT
// implementation, not FFTW), so the algorithm and parameter defaults
// match the Python aeneas pipeline exactly. The Go implementation
// matches a clean numpy SPTK reference bit-for-bit; the upstream
// `cmfcc` C extension shows a small per-cell drift (~7% relative,
// documented in PARITY_REPORT.md) which does not propagate to
// downstream alignment quality.
//
// API
//
//   DefaultParams()                  → the same defaults as Python
//                                      RuntimeConfiguration.
//   Params.Validate()                → catches non-power-of-2 FFTOrder
//                                      and other bad configurations
//                                      before they produce silent garbage.
//   ComputeFromData(samples, sr, p)  → convenience: one-shot computation.
//   Compile(p, sr)                   → returns a *Compiled with the
//                                      static tables (sin tables,
//                                      Hamming window, Mel filter bank,
//                                      DCT matrix) precomputed once.
//   (*Compiled).Compute(samples)     → reuses the tables and per-frame
//                                      scratch buffers across calls;
//                                      essential when computing MFCC
//                                      on many short audio buffers
//                                      (e.g. the SD detector or the
//                                      Psalms benchmark).
//
// Parallelism
//
// Compute auto-parallelises across frames when num_frames is at least
// ParallelThreshold (default 256). It is bit-identical to the serial
// path: the only cross-frame state is the pre-emphasis "prior", which
// is derivable from the raw input samples and precomputed before work
// is split. MaxWorkers (default 0 → runtime.NumCPU()) caps the
// goroutine count.
//
// A *Compiled is NOT safe for concurrent use: hold one per goroutine.
package mfcc
