package pipeline

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/digitalbiblesociety/dido/internal/aba"
	"github.com/digitalbiblesociety/dido/internal/align"
	"github.com/digitalbiblesociety/dido/internal/audiomfcc"
	"github.com/digitalbiblesociety/dido/internal/config"
	"github.com/digitalbiblesociety/dido/internal/ffmpeg"
	"github.com/digitalbiblesociety/dido/internal/text"
	"github.com/digitalbiblesociety/dido/internal/tts"
)

// End-to-end pipeline benchmarks on the bundled sonnet fixture (p001.mp3,
// ~53 s of stereo 44.1 kHz audio + a 15-line plain-text sonnet). These
// exercise every stage we ship at runtime: ffmpeg subprocess decode → real
// MFCC, espeak-ng synthesis × N fragments → synth MFCC, stripe DTW, ABA
// boundary adjustment. They skip cleanly when ffmpeg or espeak-ng aren't
// on PATH.
//
// Run with:
//
//	go test -bench=Pipeline -benchtime=1x -timeout=10m ./internal/pipeline/
//
// `-benchtime=1x` is appropriate: each iteration spawns ffmpeg + ≥15
// espeak-ng processes, so even N=1 is informative. Use a higher N (e.g.
// `-benchtime=5x`) to dampen subprocess-startup jitter when comparing
// optimization candidates.

const (
	benchAudioFixture = "container/job/assets/p001.mp3" // ~53 s stereo 44.1 kHz
	// benchAudioFixtureWAV is mono PCM 16 kHz — same nominal duration as
	// benchAudioFixture, but eligible for the ffmpeg-bypass WAV fast path.
	// The content does not match the sonnet text, but the pipeline runs
	// the same code paths, so wall time is directly comparable.
	benchAudioFixtureWAV = "audioformats/mono.16000.wav"
	benchTextFixture     = "inputtext/sonnet_plain.txt" // 15-line plain sonnet
	benchConfigString    = "task_language=eng|is_text_type=plain|os_task_file_format=json"
	// benchAudioSeconds is the on-disk duration of benchAudioFixture, used
	// to compute a real-time factor (audio_seconds / wall_seconds; higher
	// is better). Updated by hand if the fixture changes.
	benchAudioSeconds = 53.27
)

func benchFixturePath(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	all := append([]string{base}, parts...)
	return filepath.Join(all...)
}

func skipIfMissingBenchBinaries(b *testing.B) {
	b.Helper()
	if _, err := exec.LookPath("espeak-ng"); err != nil {
		b.Skipf("espeak-ng not in PATH: %v", err)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		b.Skipf("ffmpeg not in PATH: %v", err)
	}
}

// loadSonnetFixture reads the sonnet text + builds task/runtime config in
// the same way cmd/dido does. The audio path is returned untouched; the
// caller decides whether to decode it once or once per iteration.
func loadSonnetFixture(b *testing.B) (audioPath string, fragments []*text.Fragment, tc TaskConfig, rc config.RuntimeConfig) {
	return loadSonnetFixtureWithAudio(b, benchAudioFixture)
}

func loadSonnetFixtureWithAudio(b *testing.B, audioFixture string) (audioPath string, fragments []*text.Fragment, tc TaskConfig, rc config.RuntimeConfig) {
	b.Helper()
	audioPath = benchFixturePath(audioFixture)
	textPath := benchFixturePath(benchTextFixture)

	tc = ParseTaskConfig(benchConfigString)
	rc = config.Parse(benchConfigString)
	rc.SetGranularity(1)
	rc.SetTTS(1)

	tf, err := text.ReadFile(textPath, tc.TextFormat, text.Params{IDFormat: text.DefaultIDFormat})
	if err != nil {
		b.Fatalf("read text: %v", err)
	}
	fragments = tf.Fragments()
	if len(fragments) == 0 {
		b.Fatal("no text fragments")
	}
	for _, f := range fragments {
		if f.Language == "" {
			f.Language = tc.Language
		}
	}
	return audioPath, fragments, tc, rc
}

// BenchmarkPipelineSonnet measures the full pipeline.Execute wall time on
// the 53-second sonnet fixture. The reported metric `rt_factor` is the
// real-time factor (audio_seconds / wall_seconds); aim for a number well
// above 1 — at 10× a one-hour audiobook aligns in six minutes.
func BenchmarkPipelineSonnet(b *testing.B) {
	skipIfMissingBenchBinaries(b)
	audioPath, fragments, tc, rc := loadSonnetFixture(b)

	b.ResetTimer()
	var total time.Duration
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		sm, err := Execute(audioPath, fragments, tc, rc)
		elapsed := time.Since(t0)
		if err != nil {
			b.Fatalf("Execute: %v", err)
		}
		if sm == nil || len(sm.Fragments()) == 0 {
			b.Fatal("Execute returned empty sync map")
		}
		total += elapsed
	}
	avg := total.Seconds() / float64(b.N)
	b.ReportMetric(avg, "s/op_wall")
	b.ReportMetric(benchAudioSeconds/avg, "rt_factor")
}

// BenchmarkPipelineSonnetWAV is BenchmarkPipelineSonnet against the
// pre-decoded mono-16k WAV fixture. The ffmpeg subprocess is skipped via
// the WAV fast path, so the gap between this number and
// BenchmarkPipelineSonnet quantifies the decode-stage cost. The text
// fixture does NOT match this audio's content — alignment quality is
// nonsense, but the timing numbers are still valid because every code
// path runs.
func BenchmarkPipelineSonnetWAV(b *testing.B) {
	skipIfMissingBenchBinaries(b)
	audioPath, fragments, tc, rc := loadSonnetFixtureWithAudio(b, benchAudioFixtureWAV)

	b.ResetTimer()
	var total time.Duration
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		sm, err := Execute(audioPath, fragments, tc, rc)
		elapsed := time.Since(t0)
		if err != nil {
			b.Fatalf("Execute: %v", err)
		}
		if sm == nil || len(sm.Fragments()) == 0 {
			b.Fatal("Execute returned empty sync map")
		}
		total += elapsed
	}
	avg := total.Seconds() / float64(b.N)
	b.ReportMetric(avg, "s/op_wall")
	b.ReportMetric(benchAudioSeconds/avg, "rt_factor")
}

// BenchmarkPipelineStagesSonnet runs each pipeline stage sequentially and
// reports per-stage wall time as ReportMetric custom counters. This is
// NOT the production wall time — pipeline.Execute overlaps decode/real-MFCC
// with TTS/synth-MFCC — but it lets us size which stages are worth
// optimizing.
//
// The reported metrics (one per b.Run; each averaged across b.N) are:
//
//	decode_s — ffmpeg subprocess decode of the input audio
//	rmfcc_s  — MFCC on the decoded real-wave samples
//	tts_s    — espeak-ng synthesis of all text fragments
//	smfcc_s  — MFCC on the synthesized audio
//	dtw_s    — stripe DTW path → fragment-boundary frame indices
//	aba_s    — boundary adjustment → final SyncMap
//	sum_s    — sum of the above (a serial upper bound on wall time)
func BenchmarkPipelineStagesSonnet(b *testing.B) {
	skipIfMissingBenchBinaries(b)
	audioPath, fragments, tc, rc := loadSonnetFixture(b)
	mp := rcToMFCCParams(rc)
	espeakPath, err := tts.ResolveBinary(rc.TTS, rc.TTSPath)
	if err != nil {
		b.Fatalf("resolve espeak: %v", err)
	}

	ttsFragments := toTTSFragments(fragments, tc.Language)
	abaParams := buildABAParams(tc, rc)
	mws := rc.MFCCWindowShift
	delta := int(2.0 * rc.DTWMargin / mws)

	var (
		decodeT, rmfccT, ttsT, smfccT, dtwT, abaT time.Duration
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		samples, rate, err := ffmpeg.Decode(audioPath, rc.FFMpegSampleRate, rc.FFMpegPath)
		if err != nil {
			b.Fatalf("decode: %v", err)
		}
		decodeT += time.Since(t0)

		t1 := time.Now()
		realAM, err := audiomfcc.FromSamples(samples, rate, mp)
		if err != nil {
			b.Fatalf("real mfcc: %v", err)
		}
		rmfccT += time.Since(t1)

		t2 := time.Now()
		sr, err := tts.SynthesizeMultipleWith(ttsFragments, espeakPath, rc.TTSConcurrency)
		if err != nil {
			b.Fatalf("tts: %v", err)
		}
		ttsT += time.Since(t2)

		t3 := time.Now()
		synthAM, err := audiomfcc.FromSamples(sr.Samples, sr.SampleRate, mp)
		if err != nil {
			b.Fatalf("synth mfcc: %v", err)
		}
		smfccT += time.Since(t3)

		t4 := time.Now()
		boundaries, err := align.ComputeBoundaries(realAM, synthAM, sr.Intervals, mws, delta)
		if err != nil {
			b.Fatalf("dtw: %v", err)
		}
		dtwT += time.Since(t4)

		t5 := time.Now()
		if _, err := aba.Adjust(boundaries, fragments, realAM, abaParams); err != nil {
			b.Fatalf("aba: %v", err)
		}
		abaT += time.Since(t5)
	}
	n := float64(b.N)
	decode := decodeT.Seconds() / n
	rmfcc := rmfccT.Seconds() / n
	ttsv := ttsT.Seconds() / n
	smfcc := smfccT.Seconds() / n
	dtwv := dtwT.Seconds() / n
	abav := abaT.Seconds() / n
	sum := decode + rmfcc + ttsv + smfcc + dtwv + abav
	b.ReportMetric(decode, "decode_s")
	b.ReportMetric(rmfcc, "rmfcc_s")
	b.ReportMetric(ttsv, "tts_s")
	b.ReportMetric(smfcc, "smfcc_s")
	b.ReportMetric(dtwv, "dtw_s")
	b.ReportMetric(abav, "aba_s")
	b.ReportMetric(sum, "sum_s")
}
