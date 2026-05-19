![Dido](./assets/dido_banner_animated.svg)

A Go port of the [aeneas](https://github.com/readbeyond/aeneas) forced
audio/text alignment library — pure Go core, espeak-ng for synthesis,
FFmpeg for audio decode. Maintained by
[Digital Bible Society](https://github.com/digitalbiblesociety).

- Multi-level alignment supported (`task_granularity=2|3` for mplain /
  munparsed input): paragraph → sentence → phrase → word, each level aligned
  independently within its parent's audio window.
- Automatic transliteration for languages without an espeak-ng voice:
  when a fragment's language falls back to English AND its text is in
  a non-Latin script (Greek, Hebrew, Devanagari, Syriac, …), the text
  is romanised before synthesis so the timestamps stay accurate. Toggle
  with `tts_auto_transliterate=false`. Powered by
  [digitalbiblesociety/transliterate](https://github.com/digitalbiblesociety/transliterate).

```
go build -o dido ./cmd/dido

./dido audio.wav text.txt \
  "task_language=eng|is_text_type=plain|os_task_file_format=srt" \
  output.srt
```

## Requirements

- **Go ≥ 1.22** to build.
- **espeak-ng** on `PATH` for TTS synthesis. Override per-task with the
  `tts_path` config key, or globally with the `DIDO_ESPEAK_NG_PATH`
  env var.
- **ffmpeg** on `PATH` for audio decoding. Mono PCM 16-bit WAV at the
  target sample rate (16 kHz by default) is read directly without going
  through ffmpeg, so a WAV-only pipeline doesn't need it.
- **Python aeneas** is only required for the parity tests and the
  `parity-report` tool — never at runtime.

## Build

```
go build ./...
```

Or just the CLI:

```
go build -o dido ./cmd/dido
./dido --help
```

## Test

The standard suite is fast and self-contained:

```
go test ./...
```

The `internal/parity` package additionally compares each numeric stage
to the upstream Python implementation. It's skipped automatically if
Python or aeneas isn't importable.

```
# Compare Go vs Python on the standard fixtures (skips when Python aeneas
# is unavailable):
go test ./internal/parity/

# Regenerate the cross-implementation summary table:
go run ./cmd/parity-report   # writes PARITY_REPORT.md
```

End-to-end alignment parity on a real-world corpus
(KJV-Scorby Psalms — 150 chapters, ~4.3 h of audio):

```
# Prepares fixtures, runs both pipelines, prints aggregate drift stats.
# Defaults assume a specific local path; see the README in the harness
# directory or set MP3_DIR / USFM_JSON.
./tools/psalms-parity/run_all.sh
```


## License

[AGPL v3](LICENSE).

<p align="center">
  <img src="https://raw.githubusercontent.com/digitalbiblesociety/dido/refs/heads/main/assets/dbs-logo-half.svg" />
</p>