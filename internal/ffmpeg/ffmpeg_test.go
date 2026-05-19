package ffmpeg

import (
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/digitalbiblesociety/dido/internal/audio"
)

// writeTestWAV writes n float64 samples as a 16-bit mono WAV at rate Hz.
func writeTestWAV(t testing.TB, rate uint32, n int) string {
	t.Helper()
	f, err := os.CreateTemp("", "ffmpeg-test-*.wav")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()

	samples := make([]float64, n)
	for i := range samples {
		samples[i] = rand.Float64()*2 - 1
	}
	if err := audio.WriteMonoPCM16(f, rate, samples); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func TestDecodeWAVFastPath(t *testing.T) {
	const rate = 16000
	path := writeTestWAV(t, rate, rate*2) // 2 s of audio

	samples, got, err := Decode(path, rate, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != rate {
		t.Errorf("rate = %d, want %d", got, rate)
	}
	if len(samples) != rate*2 {
		t.Errorf("len(samples) = %d, want %d", len(samples), rate*2)
	}
}

func TestDecodeWAVWrongRateFallsThrough(t *testing.T) {
	// WAV at 44100 Hz, target 16000 Hz → tryDirectWAV returns nil → ffmpeg.
	// Without ffmpeg present we just verify tryDirectWAV returns nil,nil,nil.
	path := writeTestWAV(t, 44100, 44100)
	s, r, err := tryDirectWAV(path, 16000)
	if err != nil {
		t.Fatal(err)
	}
	if s != nil || r != 0 {
		t.Errorf("expected nil result for wrong-rate WAV")
	}
}

// TestDecodeFFmpegMissingMessage verifies that a missing ffmpeg binary
// produces an actionable error (not a cryptic "executable file not found").
// This route is triggered for non-WAV input or wrong-sample-rate WAV.
func TestDecodeFFmpegMissingMessage(t *testing.T) {
	// Force the fallback path by giving a non-existent file (tryDirectWAV
	// returns nil) and a non-existent ffmpeg binary.
	tmp, err := os.CreateTemp("", "ffmpeg-missing-*.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Write([]byte("not a real mp3"))
	tmp.Close()

	_, _, err = Decode(tmp.Name(), 16000, "/path/to/no/such/ffmpeg-binary-xyz")
	if err == nil {
		t.Fatal("expected error for missing ffmpeg binary")
	}
	msg := err.Error()
	for _, want := range []string{"ffmpeg", "not found", "PATH", "install"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q; got: %s", want, msg)
		}
	}
}

func BenchmarkDecodeWAVDirect(b *testing.B) {
	const rate = 16000
	path := writeTestWAV(b, rate, rate*30) // 30 s
	b.ResetTimer()
	for range b.N {
		if _, _, err := Decode(path, rate, ""); err != nil {
			b.Fatal(err)
		}
	}
}
