// Package audio provides WAV file reading and writing.
// This is a port of aeneas/cwave/cwave_func.c + aeneas/wavfile.py.
package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// WAVFormatPCM is the only format we support reading.
const WAVFormatPCM = 0x0001

// WAVInfo holds all header and computed fields for an open WAV file.
type WAVInfo struct {
	ChunkSize      uint32
	FmtChunkSize   uint32
	AudioFormat    uint16
	NumChannels    uint16
	SampleRate     uint32
	ByteRate       uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	DataChunkSize  uint32
	NumSamples     uint32
	DataStart      int64 // byte offset where sample data begins
	BytesPerSample uint32
}

// WAVFile wraps an io.ReadSeekCloser together with its parsed header.
type WAVFile struct {
	f    io.ReadSeekCloser
	Info WAVInfo
}

// Open opens a mono PCM WAV file at path and parses its header.
// Caller must call Close() when done.
func Open(path string) (*WAVFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	wf := &WAVFile{f: f}
	if err := wf.readHeader(); err != nil {
		f.Close()
		return nil, err
	}
	return wf, nil
}

// OpenBytes parses a mono PCM WAV from an in-memory byte buffer; Close
// is a no-op.
//
// Streaming WAV producers (e.g. espeak-ng --stdout) emit sentinel chunk
// sizes (~0x7FFFFFFF) because the final length isn't known when the
// header is written. OpenBytes trusts len(b) over the header and clamps
// NumSamples to (len(b) - DataStart) / BytesPerSample whenever the
// header overstates the payload.
func OpenBytes(b []byte) (*WAVFile, error) {
	if len(b) == 0 {
		return nil, errors.New("audio: empty buffer")
	}
	wf := &WAVFile{f: nopCloser{bytes.NewReader(b)}}
	if err := wf.readHeader(); err != nil {
		return nil, err
	}
	available := int64(len(b)) - wf.Info.DataStart
	if available < 0 {
		available = 0
	}
	if int64(wf.Info.DataChunkSize) > available {
		wf.Info.DataChunkSize = uint32(available)
		denom := uint32(wf.Info.NumChannels) * wf.Info.BytesPerSample
		if denom > 0 {
			wf.Info.NumSamples = wf.Info.DataChunkSize / denom
		}
	}
	return wf, nil
}

type nopCloser struct{ io.ReadSeeker }

func (nopCloser) Close() error { return nil }

// Close closes the underlying file. No-op for in-memory readers.
func (wf *WAVFile) Close() error { return wf.f.Close() }

// ReadSamples reads number samples starting at fromSample into dst (must be pre-allocated).
// Samples are normalised to float64 in [-1, 1].
//
// ReadSamples allocates a temporary byte buffer per call. Callers that read
// the same file in repeated chunks (e.g. streaming pipelines) should prefer
// ReadSamplesWithScratch to amortise allocation across calls.
func (wf *WAVFile) ReadSamples(fromSample, number uint32, dst []float64) error {
	return wf.ReadSamplesWithScratch(fromSample, number, dst, nil)
}

// ReadSamplesWithScratch is ReadSamples with a caller-supplied byte buffer.
// If scratch is non-nil and large enough (≥ number*BytesPerSample), it is
// reused; otherwise a fresh buffer is allocated. The returned len(scratch)
// is undefined — callers should treat scratch as scratch space, not data.
//
// Typical use:
//
//	var scratch []byte
//	for ... {
//	    if err := wf.ReadSamplesWithScratch(off, n, dst, scratch); err != nil { ... }
//	    // grow once if needed
//	    if cap(scratch) < int(n)*int(wf.Info.BytesPerSample) {
//	        scratch = make([]byte, int(n)*int(wf.Info.BytesPerSample))
//	    }
//	}
//
// This pattern eliminates the per-call make([]byte, …) GC pressure that
// dominates allocations on long batch jobs.
func (wf *WAVFile) ReadSamplesWithScratch(fromSample, number uint32, dst []float64, scratch []byte) error {
	if fromSample+number > wf.Info.NumSamples {
		return fmt.Errorf("audio: read [%d, %d) exceeds NumSamples %d",
			fromSample, fromSample+number, wf.Info.NumSamples)
	}
	if uint32(len(dst)) < number {
		return fmt.Errorf("audio: dst len %d < requested %d samples", len(dst), number)
	}
	bps := wf.Info.BytesPerSample
	need := int(number) * int(bps)
	offset := wf.Info.DataStart + int64(fromSample)*int64(bps)
	if _, err := wf.f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	var buf []byte
	if cap(scratch) >= need {
		buf = scratch[:need]
	} else {
		buf = make([]byte, need)
	}
	if _, err := io.ReadFull(wf.f, buf); err != nil {
		return err
	}
	switch bps {
	case 1:
		// 8-bit PCM WAV is unsigned: byte 128 = silence, byte 0 = max
		// negative, byte 255 = max positive (RIFF / Microsoft spec).
		for i := uint32(0); i < number; i++ {
			dst[i] = (float64(buf[i]) - 128.0) / 128.0
		}
	case 2:
		for i := uint32(0); i < number; i++ {
			v := int16(binary.LittleEndian.Uint16(buf[i*2:]))
			dst[i] = float64(v) / 32768.0
		}
	case 4:
		for i := uint32(0); i < number; i++ {
			v := int32(binary.LittleEndian.Uint32(buf[i*4:]))
			dst[i] = float64(v) / 2147483648.0
		}
	default:
		return fmt.Errorf("audio: unsupported BytesPerSample %d", bps)
	}
	return nil
}

// ReadAllSamples reads the entire file into a newly allocated slice.
func (wf *WAVFile) ReadAllSamples() ([]float64, error) {
	n := wf.Info.NumSamples
	dst := make([]float64, n)
	if err := wf.ReadSamples(0, n, dst); err != nil {
		return nil, err
	}
	return dst, nil
}

// readHeader parses the RIFF/WAVE header and locates the fmt and data chunks.
func (wf *WAVFile) readHeader() error {
	r := wf.f

	// RIFF header: "RIFF" <4-byte size LE> "WAVE"
	var tag [4]byte
	if _, err := io.ReadFull(r, tag[:]); err != nil {
		return errors.New("audio: cannot read RIFF tag")
	}
	if tag != [4]byte{'R', 'I', 'F', 'F'} {
		return errors.New("audio: not a RIFF file")
	}
	if err := binary.Read(r, binary.LittleEndian, &wf.Info.ChunkSize); err != nil {
		return errors.New("audio: cannot read chunk size")
	}
	if _, err := io.ReadFull(r, tag[:]); err != nil {
		return errors.New("audio: cannot read WAVE tag")
	}
	if tag != [4]byte{'W', 'A', 'V', 'E'} {
		return errors.New("audio: not a WAVE file")
	}

	// Seek through subchunks to find "fmt " and "data".
	maxPos := int64(wf.Info.ChunkSize) + 8
	fmtFound, dataFound := false, false

	// After the 12-byte RIFF header we iterate over all subchunks.
	for {
		pos, _ := r.Seek(0, io.SeekCurrent)
		if pos+8 > maxPos {
			break
		}
		var id [4]byte
		if _, err := io.ReadFull(r, id[:]); err != nil {
			break
		}
		var size uint32
		if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
			break
		}
		chunkStart, _ := r.Seek(0, io.SeekCurrent)

		switch id {
		case [4]byte{'f', 'm', 't', ' '}:
			if size < 16 {
				return fmt.Errorf("audio: fmt chunk too small (%d)", size)
			}
			wf.Info.FmtChunkSize = size
			if err := binary.Read(r, binary.LittleEndian, &wf.Info.AudioFormat); err != nil {
				return errors.New("audio: cannot read AudioFormat")
			}
			if wf.Info.AudioFormat != WAVFormatPCM {
				return fmt.Errorf("audio: unsupported format 0x%04X (only PCM supported)", wf.Info.AudioFormat)
			}
			if err := binary.Read(r, binary.LittleEndian, &wf.Info.NumChannels); err != nil {
				return errors.New("audio: cannot read NumChannels")
			}
			if wf.Info.NumChannels != 1 {
				return fmt.Errorf("audio: only mono (1 channel) supported, got %d", wf.Info.NumChannels)
			}
			if err := binary.Read(r, binary.LittleEndian, &wf.Info.SampleRate); err != nil {
				return errors.New("audio: cannot read SampleRate")
			}
			if err := binary.Read(r, binary.LittleEndian, &wf.Info.ByteRate); err != nil {
				return errors.New("audio: cannot read ByteRate")
			}
			if err := binary.Read(r, binary.LittleEndian, &wf.Info.BlockAlign); err != nil {
				return errors.New("audio: cannot read BlockAlign")
			}
			if err := binary.Read(r, binary.LittleEndian, &wf.Info.BitsPerSample); err != nil {
				return errors.New("audio: cannot read BitsPerSample")
			}
			fmtFound = true

		case [4]byte{'d', 'a', 't', 'a'}:
			wf.Info.DataChunkSize = size
			wf.Info.DataStart = chunkStart
			dataFound = true
		}

		// Skip to next chunk (round up to even boundary per RIFF spec).
		next := chunkStart + int64(size)
		if size%2 != 0 {
			next++
		}
		if _, err := r.Seek(next, io.SeekStart); err != nil {
			break
		}

		if fmtFound && dataFound {
			break
		}
	}

	if !fmtFound {
		return errors.New("audio: fmt chunk not found")
	}
	if !dataFound {
		return errors.New("audio: data chunk not found")
	}
	if wf.Info.DataChunkSize == 0 {
		return errors.New("audio: data chunk is empty")
	}

	wf.Info.BytesPerSample = uint32(wf.Info.BitsPerSample) / 8
	wf.Info.NumSamples = wf.Info.DataChunkSize / (uint32(wf.Info.NumChannels) * wf.Info.BytesPerSample)
	return nil
}

// WriteMonoPCM16 writes samples (float64 in [-1,1]) as a mono 16-bit PCM WAV to w.
//
// All bytes are assembled in a single contiguous buffer and emitted with one
// Write call, which is ~200x faster than the prior per-sample binary.Write loop.
func WriteMonoPCM16(w io.Writer, sampleRate uint32, samples []float64) error {
	n := uint32(len(samples))
	dataSize := n * 2
	const fmtSize = uint32(16)
	chunkSize := 4 + 8 + fmtSize + 8 + dataSize

	// 12-byte RIFF + 8 + fmtSize fmt chunk + 8 byte data chunk header + samples.
	headerLen := 12 + 8 + int(fmtSize) + 8
	buf := make([]byte, headerLen+int(dataSize))

	// RIFF header
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], chunkSize)
	copy(buf[8:12], "WAVE")

	// fmt chunk
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], fmtSize)
	binary.LittleEndian.PutUint16(buf[20:22], uint16(WAVFormatPCM))
	binary.LittleEndian.PutUint16(buf[22:24], 1) // mono
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:32], sampleRate*2) // byteRate
	binary.LittleEndian.PutUint16(buf[32:34], 2)            // blockAlign
	binary.LittleEndian.PutUint16(buf[34:36], 16)           // bitsPerSample

	// data chunk header
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], dataSize)

	// samples
	data := buf[headerLen:]
	for i, s := range samples {
		// clamp
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		v := int16(math.Round(s * 32767))
		binary.LittleEndian.PutUint16(data[i*2:i*2+2], uint16(v))
	}

	_, err := w.Write(buf)
	return err
}
