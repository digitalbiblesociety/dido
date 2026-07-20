![Dido](./assets/dido_banner_animated.svg)

Dido aligns a transcript with an audio recording and writes a start and end time
for each text fragment. Digital Bible Society maintains this Go port of the
[aeneas forced alignment library](https://github.com/readbeyond/aeneas).

## Build and run

Build the CLI with Go 1.26 or later. Install `espeak-ng` for speech synthesis.
Dido reads mono 16-bit PCM WAV files directly at the target sample rate
(16 kHz by default). Install FFmpeg to decode other formats or resample audio.

```sh
go build -o dido ./cmd/dido
```

For example, put one English text fragment per line in `text.txt`. This command
aligns the lines with `audio.wav` and writes an SRT subtitle file.

```sh
./dido audio.wav text.txt \
  "task_language=eng|is_text_type=plain|os_task_file_format=srt" \
  output.srt
```

Set `task_language` for each task. Run `./dido --help` for common settings and
supported output formats.

## Alignment options

- For `mplain` and `munparsed` input, Dido aligns paragraphs by default.
  Set `task_granularity=2` to align sentences within each paragraph.
  Set `task_granularity=3` to also align phrases and words.
- When a language uses the fallback English voice, Dido transliterates supported
  non-Latin scripts before synthesis. Set `tts_auto_transliterate=false` to
  disable this for a task. See the
  [transliteration library](https://github.com/digitalbiblesociety/transliterate).

## Batch mode

Run multiple alignment tasks from one JSON file. Dido accepts the
`AeneasTask` format used by SIL go-aeneas and Scripture App Builder.
Use an array of tasks or an object with a `tasks` array.

```json
[
  {
    "description": "Chapter 1",
    "audioFilename": "audio.wav",
    "phraseFilename": "text.txt",
    "parameters": "task_language=eng|os_task_file_format=srt",
    "outputFilename": "output.srt"
  }
]
```

```sh
./dido --batch tasks.json
```

Dido continues after a task fails and returns a nonzero exit code if any task failed.

## Environment variables

| Variable | Purpose |
| --- | --- |
| `DIDO_ESPEAK_NG_PATH` | Path to `espeak-ng`. The per-task `tts_path` setting takes precedence. |
| `DIDO_TTS_WORKERS` | Maximum number of speech synthesis processes per task. Defaults to the number of logical CPUs. |
| `DIDO_BATCH_WORKERS` | Maximum number of concurrent batch tasks. Defaults to two, or one on a single logical CPU. |

## Development

```sh
go build ./...
go test ./...
```

The test suite skips comparisons with Python aeneas when it is unavailable.
With aeneas installed, use these commands to run the comparisons and
regenerate the [parity report](docs/PARITY_REPORT.md).

```sh
go test ./internal/parity/
go run ./cmd/parity-report
```

The KJV-Scorby Psalms benchmark covers 150 chapters, about 4.3 hours of audio.
Place the mono 16 kHz recordings in `wav/` and the transcripts in `text/`,
with one verse per line. Number the file pairs from `001.wav` and `001.txt`
through `150.wav` and `150.txt`.

```sh
PSALMS_PARITY_DIR=/path/to/fixtures \
  go test -run '^$' -bench BenchmarkPsalmsBook -benchtime=1x -timeout=2h ./internal/parity/
```

## License

[GNU Affero General Public License, version 3](LICENSE)

<p align="center">
  <a href="https://github.com/digitalbiblesociety">
    <img src="./assets/dbs-logo-half.svg" alt="Digital Bible Society" />
  </a>
</p>
