package parity

import (
	"fmt"
	"path/filepath"
	"testing"
)

// SAB-shape parity: dido vs `python -m aeneas.tools.execute_task`
// driven with the exact config string SAB writes. Asserts only the
// shape SAB integration depends on — row count, ID order, column
// layout, head/tail placement. Per-row timing drift is logged for
// observability but not asserted; aeneas's cmfcc/cdtw build is
// noticeably machine-dependent (Python/numpy version, fresh
// compile vs cached wheel) and the math-level parity is already
// covered by mfcc_parity_test.go / dtw_parity_test.go against
// synthetic fixtures. Tests Skip cleanly when Python aeneas isn't
// importable.

const sonnet1Phrases = `f001|I
f002|From fairest creatures we desire increase,
f003|That thereby beauty's rose might never die,
f004|But as the riper should by time decease,
f005|His tender heir might bear his memory:
f006|But thou contracted to thine own bright eyes,
f007|Feed'st thy light's flame with self-substantial fuel,
f008|Making a famine where abundance lies,
f009|Thy self thy foe, to thy sweet self too cruel.
f010|Thou that art now the world's fresh ornament,
f011|And only herald to the gaudy spring,
f012|Within thine own bud buriest thy content,
f013|And tender churl mak'st waste in niggarding.
f014|Pity the world, or else this glutton be,
f015|To eat the world's due, by the grave and thee.
`

const sonnet2Phrases = `f001|II
f002|When forty winters shall besiege thy brow,
f003|And dig deep trenches in thy beauty's field,
f004|Thy youth's proud livery so gazed on now,
f005|Will be a tatter'd weed of small worth held:
f006|Then being asked, where all thy beauty lies,
f007|Where all the treasure of thy lusty days;
f008|To say, within thine own deep sunken eyes,
f009|Were an all-eating shame, and thriftless praise.
f010|How much more praise deserv'd thy beauty's use,
f011|If thou couldst answer 'This fair child of mine
f012|Shall sum my count, and make my old excuse,'
f013|Proving his beauty by succession thine!
f014|This were to be new made when thou art old,
f015|And see thy blood warm when thou feel'st it cold.
`

func fixtureAudioPath(parts ...string) string {
	return Fixture(append([]string{}, parts...)...)
}

func TestSABParity_SonnetTSV_Hidden(t *testing.T) {
	SkipIfUnavailable(t)
	didoBin := BuildDido(t)

	scratch := t.TempDir()
	audio := fixtureAudioPath("container", "job", "assets", "p001.mp3")
	phrases := filepath.Join(scratch, "p001.txt")
	WriteFile(t, phrases, sonnet1Phrases)

	// `tts=espeak` pins dido to classic eSpeak (aeneas's default) so
	// both stacks share one TTS binary; aeneas ignores it as a task
	// key but lands there by default. Linux CI: /usr/bin/espeak;
	// macOS Homebrew: `espeak` symlinks to espeak-ng.
	const params = "task_language=eng|is_text_type=parsed|os_task_file_format=tsv|os_task_file_head_tail_format=hidden|tts=espeak"

	didoOut := filepath.Join(scratch, "dido.tsv")
	pyOut := filepath.Join(scratch, "py.tsv")

	batchJSON := filepath.Join(scratch, "batch.json")
	WriteFile(t, batchJSON, fmt.Sprintf(
		`[{"description":"Sonnet I","audioFilename":%q,"phraseFilename":%q,"parameters":%q,"outputFilename":%q}]`,
		audio, phrases, params, didoOut))
	RunDidoBatch(t, didoBin, batchJSON)
	RunPyAeneas(t, audio, phrases, params, pyOut)

	dido := ParseTSV(t, didoOut)
	py := ParseTSV(t, pyOut)
	delta := CompareTSV(t, dido, py)
	t.Logf("sonnet TSV hidden: n=%d  mean=%.3fs  p95=%.3fs  max=%.3fs",
		delta.N, delta.Mean, delta.P95, delta.MaxAbs)
}

func TestSABParity_SonnetTSV_HeadTailAdd(t *testing.T) {
	SkipIfUnavailable(t)
	didoBin := BuildDido(t)

	scratch := t.TempDir()
	audio := fixtureAudioPath("container", "job", "assets", "p001.mp3")
	phrases := filepath.Join(scratch, "p001.txt")
	WriteFile(t, phrases, sonnet1Phrases)

	const params = "task_language=eng|is_text_type=parsed|os_task_file_format=tsv|os_task_file_head_tail_format=add|tts=espeak"

	didoOut := filepath.Join(scratch, "dido.tsv")
	pyOut := filepath.Join(scratch, "py.tsv")

	batchJSON := filepath.Join(scratch, "batch.json")
	WriteFile(t, batchJSON, fmt.Sprintf(
		`[{"description":"Sonnet I","audioFilename":%q,"phraseFilename":%q,"parameters":%q,"outputFilename":%q}]`,
		audio, phrases, params, didoOut))
	RunDidoBatch(t, didoBin, batchJSON)
	RunPyAeneas(t, audio, phrases, params, pyOut)

	dido := ParseTSV(t, didoOut)
	py := ParseTSV(t, pyOut)
	if len(dido) == 0 || dido[0].ID != "HEAD" {
		t.Errorf("dido: expected first row id=HEAD; got rows=%d first=%q", len(dido), dido[0].ID)
	}
	if len(dido) == 0 || dido[len(dido)-1].ID != "TAIL" {
		t.Errorf("dido: expected last row id=TAIL; got last=%q", dido[len(dido)-1].ID)
	}
	if len(py) == 0 || py[0].ID != "HEAD" || py[len(py)-1].ID != "TAIL" {
		t.Logf("py edges: first=%q last=%q (dido edges: first=%q last=%q)",
			py[0].ID, py[len(py)-1].ID, dido[0].ID, dido[len(dido)-1].ID)
	}
	delta := CompareTSV(t, dido, py)
	t.Logf("sonnet TSV add: n=%d  mean=%.3fs  p95=%.3fs  max=%.3fs",
		delta.N, delta.Mean, delta.P95, delta.MaxAbs)
}

func TestSABParity_BatchTwoTasks(t *testing.T) {
	SkipIfUnavailable(t)
	didoBin := BuildDido(t)

	scratch := t.TempDir()
	audio1 := fixtureAudioPath("container", "job", "assets", "p001.mp3")
	audio2 := fixtureAudioPath("container", "job", "assets", "p002.mp3")
	phr1 := filepath.Join(scratch, "p001.txt")
	phr2 := filepath.Join(scratch, "p002.txt")
	WriteFile(t, phr1, sonnet1Phrases)
	WriteFile(t, phr2, sonnet2Phrases)

	const params = "task_language=eng|is_text_type=parsed|os_task_file_format=tsv|os_task_file_head_tail_format=hidden|tts=espeak"

	didoOut1 := filepath.Join(scratch, "d1.tsv")
	didoOut2 := filepath.Join(scratch, "d2.tsv")
	pyOut1 := filepath.Join(scratch, "py1.tsv")
	pyOut2 := filepath.Join(scratch, "py2.tsv")

	batchJSON := filepath.Join(scratch, "batch.json")
	WriteFile(t, batchJSON, fmt.Sprintf(
		`[{"description":"Sonnet I","audioFilename":%q,"phraseFilename":%q,"parameters":%q,"outputFilename":%q},`+
			`{"description":"Sonnet II","audioFilename":%q,"phraseFilename":%q,"parameters":%q,"outputFilename":%q}]`,
		audio1, phr1, params, didoOut1,
		audio2, phr2, params, didoOut2))
	RunDidoBatch(t, didoBin, batchJSON)
	RunPyAeneas(t, audio1, phr1, params, pyOut1)
	RunPyAeneas(t, audio2, phr2, params, pyOut2)

	for _, c := range []struct {
		name    string
		dido, py string
	}{
		{"Sonnet I", didoOut1, pyOut1},
		{"Sonnet II", didoOut2, pyOut2},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := ParseTSV(t, c.dido)
			p := ParseTSV(t, c.py)
			delta := CompareTSV(t, d, p)
			t.Logf("%s: n=%d  mean=%.3fs  p95=%.3fs  max=%.3fs",
				c.name, delta.N, delta.Mean, delta.P95, delta.MaxAbs)
		})
	}
}
