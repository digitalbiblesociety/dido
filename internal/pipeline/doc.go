// Package pipeline orchestrates the single-level forced-alignment task.
// It is the Go equivalent of aeneas/executetask.py
// (ExecuteTask._execute_single_level_task).
//
// Lifecycle (a single Execute call):
//
//   1. Parse the user's config string into a TaskConfig (this package)
//      and a RuntimeConfig (config package). Unknown keys are surfaced
//      by ParseTaskConfigStrict / config.ParseStrict.
//
//   2. Two work streams run concurrently:
//        (a) decode the audio file via ffmpeg (or read WAV directly) →
//            MFCC of the real wave (AudioMFCC);
//        (b) synthesise the text fragments via espeak-ng → MFCC of the
//            synthetic wave + per-fragment time intervals.
//
//   3. Optionally detect head/tail lengths via the sd package. The SD
//      detector reuses the forward synthesis MFCC from step (b) for
//      head detection (a "no-resynth" optimisation, see C5 in
//      IMPROVEMENT_PLANS.md); tail detection still resynthesises in
//      reverse fragment order.
//
//   4. Align the real-wave middle section against the synthetic MFCC
//      via DTW (stripe or exact, decided by frame counts and the
//      dtw_margin config key).
//
//   5. Map DTW path positions to per-fragment boundaries (align package).
//
//   6. Apply the Adjust Boundary Algorithm chosen by the user (aba
//      package) to refine boundaries and tag head/tail/non-speech
//      fragments.
//
//   7. Return the resulting *syncmap.SyncMap. The caller serialises it
//      with syncmap/format.Write.
//
// Concurrency is bounded by tts_concurrency / DIDO_TTS_WORKERS for
// the synthesis pass; MFCC and DTW automatically fan out across cores
// inside their respective packages (see C3).
package pipeline
