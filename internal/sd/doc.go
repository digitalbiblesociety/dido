// Package sd detects the audio head and tail of an audio file by
// aligning a synthesised query against the leading or trailing portion
// of the real wave's MFCC. It is the Go port of aeneas/sd.py.
//
// Algorithm
//
// For head detection:
//
//   1. Synthesise (or reuse) the forward fragments as one continuous
//      WAV → AudioMFCC (the "query").
//   2. Take the first audioFactor * maxLen seconds of the real-wave
//      MFCC as the search window.
//   3. For each speech-interval start in [minFrames, maxFrames], align
//      the full query against real[start:searchEnd] via exact DTW.
//   4. The candidate with the minimum DTW cost wins; its starting frame
//      times mfcc_window_shift yields the head length in seconds.
//
// For tail detection the real-wave and query MFCCs are both reversed in
// time, reducing the problem to head detection. The query for tail must
// be synthesised in reverse fragment order (frag_n .. frag_1) — the
// per-fragment interior order matters, so a forward synthesis can't be
// blindly reused.
//
// Caching
//
// Config.CachedForwardMFCC, if non-nil, lets the detector skip the
// synthesis + MFCC pass for head detection. The main alignment pipeline
// already computes a forward synthesis MFCC; passing it here avoids
// doing the work twice. Tail detection always synthesises.
package sd
