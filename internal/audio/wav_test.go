package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMono16000(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "audioformats", "mono.16000.wav")
	wf, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wf.Close()

	if wf.Info.SampleRate != 16000 {
		t.Errorf("SampleRate: got %d, want 16000", wf.Info.SampleRate)
	}
	if wf.Info.NumChannels != 1 {
		t.Errorf("NumChannels: got %d, want 1", wf.Info.NumChannels)
	}
	if wf.Info.BitsPerSample != 16 {
		t.Errorf("BitsPerSample: got %d, want 16", wf.Info.BitsPerSample)
	}
	// mono.16000.wav is 1704610 bytes; data = (1704610-44) / 2 = 852283 samples
	if wf.Info.NumSamples == 0 {
		t.Error("NumSamples is zero")
	}
}

func TestReadAllSamples(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "audioformats", "mono.16000.wav")
	wf, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wf.Close()

	samples, err := wf.ReadAllSamples()
	if err != nil {
		t.Fatal(err)
	}
	if uint32(len(samples)) != wf.Info.NumSamples {
		t.Errorf("got %d samples, want %d", len(samples), wf.Info.NumSamples)
	}
	// All samples should be in [-1, 1]
	for i, s := range samples {
		if s < -1.0 || s > 1.0 {
			t.Errorf("sample[%d] = %f out of range", i, s)
			break
		}
	}
}

func TestRoundtripWAV(t *testing.T) {
	// Write a simple sine wave and read it back
	sr := uint32(16000)
	dur := 0.1 // 100 ms
	n := int(float64(sr) * dur)
	orig := make([]float64, n)
	for i := range orig {
		orig[i] = math.Sin(2 * math.Pi * 440.0 * float64(i) / float64(sr))
	}

	var buf bytes.Buffer
	if err := WriteMonoPCM16(&buf, sr, orig); err != nil {
		t.Fatal(err)
	}

	// Write to temp file and read back
	tmp, err := os.CreateTemp(t.TempDir(), "*.wav")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Write(buf.Bytes())
	tmp.Close()

	wf, err := Open(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wf.Close()

	if wf.Info.SampleRate != sr {
		t.Errorf("SampleRate: got %d, want %d", wf.Info.SampleRate, sr)
	}
	if int(wf.Info.NumSamples) != n {
		t.Errorf("NumSamples: got %d, want %d", wf.Info.NumSamples, n)
	}

	got, err := wf.ReadAllSamples()
	if err != nil {
		t.Fatal(err)
	}

	// 16-bit quantisation: encode uses round(s*32767), decode uses /32768.
	// Max error ≤ 2/32768.
	const tol = 2.0 / 32768.0
	for i, g := range got {
		diff := math.Abs(g - orig[i])
		if diff > tol+1e-10 {
			t.Errorf("sample[%d]: got %f, want %f, diff %e", i, g, orig[i], diff)
			break
		}
	}
}

// build8BitWAV returns the bytes of a minimal mono 8-bit PCM WAV containing
// the supplied unsigned-byte samples. Used to test 8-bit decoding without
// relying on an external fixture (and without a corresponding 8-bit encoder).
func build8BitWAV(t *testing.T, sampleRate uint32, samples []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	dataSize := uint32(len(samples))
	fmtSize := uint32(16)
	chunkSize := 4 + 8 + fmtSize + 8 + dataSize
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, chunkSize)
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, fmtSize)
	binary.Write(&buf, binary.LittleEndian, uint16(WAVFormatPCM)) // AudioFormat
	binary.Write(&buf, binary.LittleEndian, uint16(1))            // mono
	binary.Write(&buf, binary.LittleEndian, sampleRate)
	binary.Write(&buf, binary.LittleEndian, sampleRate*1) // byteRate
	binary.Write(&buf, binary.LittleEndian, uint16(1))    // blockAlign
	binary.Write(&buf, binary.LittleEndian, uint16(8))    // bitsPerSample
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, dataSize)
	buf.Write(samples)
	return buf.Bytes()
}

// TestRead8BitUnsigned verifies the unsigned-byte → float64 conversion for
// 8-bit PCM WAV. The spec (Microsoft RIFF / WAVE) defines 8-bit samples as
// unsigned: byte 0 = max negative, byte 128 = silence, byte 255 = max
// positive. Reading byte X must produce (X-128)/128.0.
func TestRead8BitUnsigned(t *testing.T) {
	cases := []struct {
		raw  byte
		want float64
	}{
		{0, -1.0},                  // max negative
		{64, (64.0 - 128.0) / 128}, // -0.5
		{128, 0.0},                 // silence
		{192, (192.0 - 128.0) / 128}, // +0.5
		{255, (255.0 - 128.0) / 128}, // ~+0.992 (just below +1)
	}
	raw := make([]byte, len(cases))
	for i, c := range cases {
		raw[i] = c.raw
	}
	wavBytes := build8BitWAV(t, 8000, raw)

	tmp, err := os.CreateTemp(t.TempDir(), "*.wav")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Write(wavBytes)
	tmp.Close()

	wf, err := Open(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wf.Close()
	if wf.Info.BitsPerSample != 8 {
		t.Fatalf("BitsPerSample: got %d, want 8", wf.Info.BitsPerSample)
	}
	got, err := wf.ReadAllSamples()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(cases) {
		t.Fatalf("got %d samples, want %d", len(got), len(cases))
	}
	for i, c := range cases {
		if math.Abs(got[i]-c.want) > 1e-12 {
			t.Errorf("byte=%d: got %f, want %f", c.raw, got[i], c.want)
		}
	}
}
