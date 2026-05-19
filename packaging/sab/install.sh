#!/usr/bin/env bash
# Install dido as SAB's go-aeneas backend. Detects platform.
# Idempotent; reversible via uninstall.sh.
#
# Usage:
#   ./install.sh
#   ./install.sh --sab </path/to/SAB.app>     # macOS, non-default location
#   ./install.sh --sab-root </path>           # Linux, non-default install
#   ./install.sh --binary </path/to/binary>   # skip the build

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

HOST_OS=""
case "$(uname -s)" in
  Darwin) HOST_OS=mac ;;
  Linux)  HOST_OS=linux ;;
  *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

SAB_APP_DEFAULT="/Applications/Scripture App Builder.app"
SAB_LINUX_SEARCH=(
  "/usr/share/scripture-app-builder"
  "/opt/scripture-app-builder"
  "/opt/SIL/scripture-app-builder"
  "/usr/lib/scripture-app-builder"
)

SAB_APP=""
SAB_ROOT=""
BIN_OVERRIDE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sab)        SAB_APP="$2"; shift 2 ;;
    --sab-root)   SAB_ROOT="$2"; shift 2 ;;
    --binary)     BIN_OVERRIDE="$2"; shift 2 ;;
    -h|--help)
      sed -n '1,/^set -e/p' "$0" | sed -n '2,$p' | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

FEATURE_KEY="2402-4537-40F6-BFF6"

TARGET=""
SETTINGS=""
case "$HOST_OS" in
  mac)
    SAB_APP="${SAB_APP:-$SAB_APP_DEFAULT}"
    if [[ ! -d "$SAB_APP" ]]; then
      echo "Scripture App Builder.app not found at:" >&2
      echo "  $SAB_APP" >&2
      echo "Pass --sab </path/to/Scripture App Builder.app> if installed elsewhere." >&2
      exit 1
    fi
    TARGET="$SAB_APP/Contents/MacOS/go-aeneas"
    SETTINGS="$HOME/Library/Preferences/SIL/App Builder/settings.xml"
    ;;
  linux)
    # Linux SAB .deb doesn't ship a go-aeneas binary; SAB still probes
    # <app.builder>/bin/go-aeneas for SIL mode, so we install fresh
    # there. Anchor on the canonical jar, not the missing binary.
    if [[ -z "$SAB_ROOT" ]]; then
      for cand in "${SAB_LINUX_SEARCH[@]}"; do
        if [[ -f "$cand/bin/scripture-app-builder.jar" ]]; then
          SAB_ROOT="$cand"; break
        fi
      done
      if [[ -z "$SAB_ROOT" ]]; then
        echo "Could not locate a Scripture App Builder install. Tried:" >&2
        for cand in "${SAB_LINUX_SEARCH[@]}"; do echo "  $cand" >&2; done
        echo "Pass --sab-root </path/to/SAB-install> if installed elsewhere." >&2
        exit 1
      fi
    fi
    if [[ ! -f "$SAB_ROOT/bin/scripture-app-builder.jar" ]]; then
      echo "no scripture-app-builder.jar found under $SAB_ROOT — wrong --sab-root?" >&2
      exit 1
    fi
    TARGET="$SAB_ROOT/bin/go-aeneas"
    SETTINGS="$HOME/.local/share/SIL/App Builder/settings.xml"
    ;;
esac

BACKUP="$TARGET.sab-original"
FRESH_MARKER="$TARGET.dido-fresh-install"

INSTALL_MODE="swap"
if [[ ! -f "$TARGET" && ! -f "$BACKUP" ]]; then
  INSTALL_MODE="fresh"
fi

if [[ -n "$BIN_OVERRIDE" ]]; then
  BIN="$BIN_OVERRIDE"
  [[ -f "$BIN" ]] || { echo "binary not found: $BIN" >&2; exit 1; }
else
  case "$HOST_OS" in
    mac)   BIN="$SCRIPT_DIR/dist/mac/go-aeneas" ;;
    linux) BIN="$SCRIPT_DIR/dist/linux/$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')/go-aeneas" ;;
  esac
  if [[ ! -f "$BIN" ]]; then
    echo "==> building go-aeneas (no --binary given, dist/ empty)"
    bash "$SCRIPT_DIR/build.sh" "$HOST_OS"
  fi
fi

echo "==> target SAB binary: $TARGET"
echo "==> source dido binary: $BIN"

SUDO=""
if [[ ! -w "$(dirname "$TARGET")" ]]; then
  echo "==> $(dirname "$TARGET") is not writable as ${USER:-$(id -un)}; will use sudo for the binary swap."
  if ! command -v sudo >/dev/null; then
    echo "sudo not available — re-run this script as root." >&2
    exit 1
  fi
  SUDO="sudo"
  $SUDO -v
fi

case "$INSTALL_MODE" in
  swap)
    if [[ -f "$BACKUP" ]]; then
      echo "==> backup already exists at $BACKUP (re-install)"
    else
      echo "==> backing up bundled go-aeneas → $(basename "$BACKUP")"
      $SUDO cp -p "$TARGET" "$BACKUP"
    fi
    ;;
  fresh)
    echo "==> no existing go-aeneas at $TARGET — installing dido fresh"
    $SUDO mkdir -p "$(dirname "$TARGET")"
    # Marker for uninstall to distinguish fresh-install from swap.
    $SUDO touch "$FRESH_MARKER"
    ;;
esac

echo "==> installing dido as $TARGET"
$SUDO cp -p "$BIN" "$TARGET"

# keys.txt lives under the parent of SAB's `app.def` folder (per
# Settings.getAppsFolderInMyDocuments() in the bytecode). Parse the
# user's settings.xml for the actual path; fall back to the SAB default.
keys_file_path() {
  local app_def=""
  if [[ -f "$SETTINGS" ]]; then
    app_def=$(grep -oE '<folder[[:space:]]+name="app\.def">[^<]+</folder>' "$SETTINGS" \
              | sed -E 's|.*>([^<]+)</folder>|\1|' | head -1)
  fi
  if [[ -z "$app_def" ]]; then
    app_def="$HOME/App Builder/Scripture Apps/App Projects"
  fi
  echo "$(dirname "$app_def")/keys.txt"
}

KEYS_FILE=$(keys_file_path)
KEYS_DIR=$(dirname "$KEYS_FILE")
mkdir -p "$KEYS_DIR"
if [[ -f "$KEYS_FILE" ]] && grep -qF "$FEATURE_KEY" "$KEYS_FILE"; then
  echo "==> feature-flag $FEATURE_KEY already present in $KEYS_FILE"
else
  echo "==> appending feature-flag $FEATURE_KEY to $KEYS_FILE"
  if [[ -f "$KEYS_FILE" && -s "$KEYS_FILE" ]] && [[ -n "$(tail -c1 "$KEYS_FILE")" ]]; then
    echo "" >> "$KEYS_FILE"
  fi
  echo "$FEATURE_KEY" >> "$KEYS_FILE"
fi

if [[ ! -f "$SETTINGS" ]]; then
  echo "==> warning: SAB settings.xml not found at:"
  echo "    $SETTINGS"
  echo "    Launch SAB at least once so it creates the file, then re-run."
  echo "    (Binary swap is in place; SAB will still default to Python until prefs are set.)"
  exit 0
fi

if grep -qE '<preference[[:space:]]+name="aeneas-mode">[^<]*</preference>' "$SETTINGS"; then
  echo "==> updating aeneas-mode preference → sil"
  sed -i.installbak -E 's|<preference[[:space:]]+name="aeneas-mode">[^<]*</preference>|<preference name="aeneas-mode">sil</preference>|' "$SETTINGS"
  rm -f "$SETTINGS.installbak"
else
  echo "==> inserting <preference name=\"aeneas-mode\">sil</preference>"
  if ! grep -q '</preferences>' "$SETTINGS"; then
    echo "    settings.xml has no </preferences> block — please add the preference manually:" >&2
    echo "    <preference name=\"aeneas-mode\">sil</preference>" >&2
    exit 1
  fi
  awk '
    /<\/preferences>/ && !done {
      print "    <preference name=\"aeneas-mode\">sil</preference>"
      done = 1
    }
    { print }
  ' "$SETTINGS" > "$SETTINGS.new"
  mv "$SETTINGS.new" "$SETTINGS"
fi

cat <<EOF

────────────────────────────────────────────────────────────────────
Done.

SAB will now route audio-text alignment through dido (SIL mode) on the
next launch. To revert, run:
  bash $SCRIPT_DIR/uninstall.sh

Files touched (all reversible):
  • SAB binary:  $TARGET
  • Backup:      $BACKUP
  • Feature key: $KEYS_FILE
  • Preference:  $SETTINGS
EOF
