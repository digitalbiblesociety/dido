// Package ffmpeg decodes audio files to raw PCM.
// For mono WAV files already at the target sample rate the file is read
// directly (no subprocess). All other formats fall back to the ffmpeg
// subprocess.
package ffmpeg

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/digitalbiblesociety/dido/internal/audio"
)

// Decode decodes the audio file at audioPath to mono float64 samples at
// sampleRate Hz. If binaryPath is "", "ffmpeg" is resolved from PATH.
//
// Fast path: if audioPath is already a mono PCM WAV at the requested rate the
// file is read directly without spawning a subprocess.
func Decode(audioPath string, sampleRate int, binaryPath string) ([]float64, uint32, error) {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	if s, r, err := tryDirectWAV(audioPath, sampleRate); err != nil {
		return nil, 0, err
	} else if s != nil {
		return s, r, nil
	}
	return decodeViaFFmpeg(audioPath, sampleRate, binaryPath)
}

// tryDirectWAV reads a mono PCM WAV already at targetRate without spawning
// ffmpeg. Returns (nil, 0, nil) when the file doesn't qualify.
func tryDirectWAV(path string, targetRate int) ([]float64, uint32, error) {
	wf, err := audio.Open(path)
	if err != nil {
		return nil, 0, nil // not a valid mono WAV; fall through to ffmpeg
	}
	defer wf.Close()
	if int(wf.Info.SampleRate) != targetRate {
		return nil, 0, nil // wrong sample rate; ffmpeg will resample
	}
	samples, err := wf.ReadAllSamples()
	if err != nil {
		return nil, 0, fmt.Errorf("wav: %w", err)
	}
	return samples, wf.Info.SampleRate, nil
}

func decodeViaFFmpeg(audioPath string, sampleRate int, binaryPath string) ([]float64, uint32, error) {
	if binaryPath == "" {
		binaryPath = "ffmpeg"
	}
	// Resolve the binary first so we can produce a friendlier "not on PATH"
	// error than the generic exec.Cmd "executable file not found".
	if _, lookErr := exec.LookPath(binaryPath); lookErr != nil {
		return nil, 0, fmt.Errorf(
			"audio: cannot decode %q: ffmpeg binary %q not found on PATH "+
				"(install ffmpeg, or convert the input to mono PCM 16-bit WAV at %d Hz beforehand): %w",
			audioPath, binaryPath, sampleRate, lookErr)
	}
	cmd := exec.Command(binaryPath,
		"-nostdin",
		"-i", audioPath,
		"-ar", strconv.Itoa(sampleRate),
		"-ac", "1",
		"-f", "s16le",
		"-loglevel", "error",
		"pipe:1",
	)
	raw, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			stderr = string(bytes.TrimRight(ee.Stderr, "\n"))
		}
		return nil, 0, fmt.Errorf(
			"audio: ffmpeg failed to decode %q (rate=%d Hz, mono): %w%s",
			audioPath, sampleRate, err, ffmpegStderrSuffix(stderr))
	}
	n := len(raw) / 2
	samples := make([]float64, n)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(raw[i*2 : i*2+2]))
		samples[i] = float64(v) / 32768.0
	}
	return samples, uint32(sampleRate), nil
}

func ffmpegStderrSuffix(stderr string) string {
	if stderr == "" {
		return ""
	}
	return "\n" + stderr
}
