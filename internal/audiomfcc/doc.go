// Package audiomfcc wraps an MFCC matrix with lazy VAD and head/middle/
// tail interval extraction. It is the Go port of aeneas/audiofilemfcc.py.
//
// The matrix layout is matrix[coeff][frame]; row 0 is the energy
// coefficient and is consumed by VAD before being discarded by DTW.
//
// Typical use
//
//   am, _ := audiomfcc.FromSamples(samples, sampleRate, mfcc.DefaultParams())
//   am.RunVAD(vadParams)               // populate the mask
//   am.SetHeadMiddleTail(head, mid, tail)
//   middle := am.Slice(am.MiddleBegin(), am.MiddleEnd())
//
// FromSamplesCompiled is the high-throughput entry point when many
// short MFCCs are computed in a row (e.g. inside the SD detector):
// pass a pre-built mfcc.Compiled to skip the static-table setup cost
// on every call.
//
// Reverse() flips the matrix in place along the time axis and updates
// any cached VAD intervals so the head/tail detector can treat tail
// detection as a head problem on the reversed wave.
package audiomfcc
