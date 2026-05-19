#!/usr/bin/env bash
# Reverse install.sh. Idempotent; works on macOS and Linux.

set -euo pipefail

HOST_OS=""
case "$(uname -s)" in
  Darwin) HOST_OS=mac ;;
  Linux)  HOST_OS=linux ;;
  *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

SAB_APP="/Applications/Scripture App Builder.app"
SAB_ROOT=""
SAB_LINUX_SEARCH=(
  "/usr/share/scripture-app-builder"
  "/opt/scripture-app-builder"
  "/opt/SIL/scripture-app-builder"
  "/usr/lib/scripture-app-builder"
)

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sab)        SAB_APP="$2"; shift 2 ;;
    --sab-root)   SAB_ROOT="$2"; shift 2 ;;
    -h|--help)
      sed -n '1,/^set -e/p' "$0" | sed -n '2,$p' | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

TARGET=""
case "$HOST_OS" in
  mac)
    TARGET="$SAB_APP/Contents/MacOS/go-aeneas"
    ;;
  linux)
    if [[ -z "$SAB_ROOT" ]]; then
      for cand in "${SAB_LINUX_SEARCH[@]}"; do
        if [[ -f "$cand/bin/scripture-app-builder.jar" ]]; then
          SAB_ROOT="$cand"; break
        fi
      done
    fi
    if [[ -z "$SAB_ROOT" ]]; then
      echo "no SAB install found under the standard locations." >&2
      echo "Pass --sab-root </path> if installed elsewhere." >&2
      exit 1
    fi
    TARGET="$SAB_ROOT/bin/go-aeneas"
    ;;
esac

BACKUP="$TARGET.sab-original"
FRESH_MARKER="$TARGET.dido-fresh-install"

# Two install modes leave different breadcrumbs: .sab-original (swap)
# or .dido-fresh-install (fresh).
HAS_BACKUP=0; HAS_FRESH=0
[[ -f "$BACKUP" ]] && HAS_BACKUP=1
[[ -f "$FRESH_MARKER" ]] && HAS_FRESH=1

if [[ $HAS_BACKUP -eq 0 && $HAS_FRESH -eq 0 ]]; then
  echo "no install marker found at $TARGET (neither .sab-original nor .dido-fresh-install)."
  echo "either install.sh was never run here, or the install has already been undone."
  exit 0
fi

SUDO=""
if [[ ! -w "$(dirname "$TARGET")" ]]; then
  if ! command -v sudo >/dev/null; then
    echo "the SAB tree is not writable as ${USER:-$(id -un)} and sudo is unavailable — re-run as root." >&2
    exit 1
  fi
  SUDO="sudo"
  $SUDO -v
fi

if [[ $HAS_BACKUP -eq 1 ]]; then
  if cmp -s "$BACKUP" "$TARGET"; then
    # Already restored; clean up orphaned backup.
    echo "==> $TARGET already matches the backup — removing orphan $BACKUP"
    $SUDO rm "$BACKUP"
    exit 0
  fi
  echo "==> restoring SAB go-aeneas from $(basename "$BACKUP")"
  $SUDO mv "$BACKUP" "$TARGET"
else
  echo "==> removing fresh dido install at $TARGET (no original to restore)"
  $SUDO rm -f "$TARGET" "$FRESH_MARKER"
fi

case "$HOST_OS" in
  mac)   SETTINGS="$HOME/Library/Preferences/SIL/App Builder/settings.xml" ;;
  linux) SETTINGS="$HOME/.local/share/SIL/App Builder/settings.xml" ;;
esac

if [[ -f "$SETTINGS" ]] && grep -qE '<preference[[:space:]]+name="aeneas-mode">sil</preference>' "$SETTINGS"; then
  echo "==> clearing aeneas-mode=sil from preferences"
  sed -i.uninstallbak -E '/<preference[[:space:]]+name="aeneas-mode">sil<\/preference>/d' "$SETTINGS"
  rm -f "$SETTINGS.uninstallbak"
fi

cat <<EOF

────────────────────────────────────────────────────────────────────
SAB's bundled go-aeneas has been restored.
SAB's aeneas-mode preference is back to its default (Python aeneas).

The feature-flag entry in <app-builder-folder>/keys.txt was left in
place — removing it is harmless but optional.
EOF
