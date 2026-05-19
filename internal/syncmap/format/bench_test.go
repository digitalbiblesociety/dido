package format

import (
	"testing"
)

// Run with: go test -bench=. -benchmem ./internal/syncmap/format/

func benchFormat(b *testing.B, f Format) {
	sm := sonnet001()
	smil := SMILParams{PageRef: "sonnet001.xhtml", AudioRef: "sonnet001.mp3"}
	eaf := EAFParams{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Write(sm, f, smil, eaf); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFormatSRT(b *testing.B)       { benchFormat(b, FormatSRT) }
func BenchmarkFormatVTT(b *testing.B)       { benchFormat(b, FormatVTT) }
func BenchmarkFormatJSON(b *testing.B)      { benchFormat(b, FormatJSON) }
func BenchmarkFormatCSV(b *testing.B)       { benchFormat(b, FormatCSV) }
func BenchmarkFormatTSV(b *testing.B)       { benchFormat(b, FormatTSV) }
func BenchmarkFormatXML(b *testing.B)       { benchFormat(b, FormatXMLLegacy) }
func BenchmarkFormatTTML(b *testing.B)      { benchFormat(b, FormatTTML) }
func BenchmarkFormatSMIL(b *testing.B)      { benchFormat(b, FormatSMIL) }
func BenchmarkFormatEAF(b *testing.B)       { benchFormat(b, FormatEAF) }
