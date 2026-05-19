# Drop dido in as SAB's `go-aeneas`

These three scripts let dido stand in for SAB's bundled SIL go-aeneas
binary, so Scripture App Builder routes its audio-text-alignment jobs
through dido instead of Python aeneas — without any SAB-side
configuration the user has to chase down. Supports macOS and Linux.

## Files

- `build.sh` — cross-compiles dido for the target platform(s):
  - `./build.sh` → host OS only.
  - `./build.sh mac` → universal Mach-O (`darwin/amd64` + `darwin/arm64`,
    lipo-merged, ad-hoc code-signed).
  - `./build.sh linux` → ELF for `linux/amd64` + `linux/arm64`.
  - `./build.sh all` → mac + linux.
- `install.sh` — backs up the bundled binary, drops dido in, flips the
  SAB prefs that make SAB pick the SIL backend. Auto-detects platform.
- `uninstall.sh` — restores the bundled binary from the backup, resets
  the prefs to default. Idempotent.

## Install

```sh
bash packaging/sab/install.sh
```

The script auto-detects platform. CLI overrides:

| Flag                | Use                                                              |
|---------------------|------------------------------------------------------------------|
| `--sab </path>`     | macOS: non-default `Scripture App Builder.app` location.         |
| `--sab-root </p>`   | Linux: explicit install root (e.g. `/opt/scripture-app-builder`).|
| `--binary </path>`  | Use a pre-built `go-aeneas` instead of building from source.     |

If the install target isn't writable as the current user (the usual
case for `/Applications` on macOS or `/usr/share` on Linux), the
script asks for `sudo` once and uses it only for the in-bundle file
operations.

## What the installer touches

All four touch points are reversible:

| Path | Change |
|---|---|
| `<SAB-tree>/go-aeneas` | Replaced with dido. |
| `<SAB-tree>/go-aeneas.sab-original` | Backup of the bundled binary, created once. Re-runs never overwrite it. |
| `<keys.txt>` | Appends the feature-flag GUID `2402-4537-40F6-BFF6`. Without it, SAB ignores `aeneas-mode` and always uses Python aeneas. |
| `<settings.xml>` | Adds/updates `<preference name="aeneas-mode">sil</preference>` inside the `<preferences>` block. |

`<SAB-tree>`, `<settings.xml>`, and `<keys.txt>` resolve per platform:

| | macOS | Linux |
|---|---|---|
| Binary | `/Applications/Scripture App Builder.app/Contents/MacOS/go-aeneas` | `<sab-root>/.../go-aeneas` (auto-located under `/usr/share/scripture-app-builder/`, `/opt/scripture-app-builder/`, `/opt/SIL/scripture-app-builder/`, `/usr/lib/scripture-app-builder/`) |
| `settings.xml` | `~/Library/Preferences/SIL/App Builder/settings.xml` | `~/.local/share/SIL/App Builder/settings.xml` |
| `keys.txt` | `<dirname of app.def folder>/keys.txt` — derived from `settings.xml`. Default: `~/App Builder/Scripture Apps/keys.txt`. | same logic; default: `~/App Builder/Scripture Apps/keys.txt`. |

The `keys.txt` location comes from SAB's `getAppsFolderInMyDocuments()`,
which returns the parent directory of the user's configured `app.def`
folder. The installer parses `settings.xml` to find the right path and
falls back to the SAB default when `settings.xml` is missing.

## Why all four

Just swapping the binary isn't enough. SAB's `PhraseFileWriter` reads
two pieces of state before deciding which backend to invoke:

1. `Settings.hasKey("2402-4537-40F6-BFF6")` — a feature flag SIL ships
   so they can A/B between Python aeneas and their Go reimplementation
   from a single build. Without this key, SAB hard-codes the
   `READ_BEYOND` (Python) path regardless of any other preference.
2. `Settings.getPreference("aeneas-mode")` — the actual backend
   selection. Stored as the lower-case text form: `read-beyond` or
   `sil`.

Both have to be set. Either alone is a no-op.

## Uninstall

```sh
bash packaging/sab/uninstall.sh
```

Restores the bundled binary from `go-aeneas.sab-original`, removes the
`<preference name="aeneas-mode">sil</preference>` line from
`settings.xml` (SAB then falls back to its `READ_BEYOND` default).
Leaves the feature-flag entry in `keys.txt` — it's harmless to keep,
and removing it would only hide the radio buttons that let a user flip
backends inside SAB's settings UI.

## CI / pre-built binaries

The repo's `.github/workflows/release.yml` builds the same artefacts
this script would produce — macOS universal Mach-O, Linux amd64+arm64
ELFs, plus Windows amd64 — and attaches them to GitHub Releases on
every `v*` tag push. Users who don't want to install a Go toolchain
can grab the matching archive from the latest release and pass it to
`install.sh --binary <extracted-path>`.

## Manual verification

A useful sanity check after install:

```sh
# macOS
"/Applications/Scripture App Builder.app/Contents/MacOS/go-aeneas" --help

# Linux (adjust path to your distro's SAB install)
/usr/share/scripture-app-builder/.../go-aeneas --help
```

Should print dido's help (the `BATCH MODE` and `DIDO_BATCH_WORKERS`
sections give it away) rather than the original `go-aeneas` flag list.
Run again after uninstall to confirm the bundled binary is back.
