#!/usr/bin/env bash
# Exercise install.sh + uninstall.sh against an APFS clone of the real
# SAB.app, with $HOME redirected to a fake home. Verifies real-bundle
# behavior (signature invalidation, byte-identical restore) without
# touching the user's actual SAB install or prefs.
#
# Usage:
#   ./test-mac.sh
#   ./test-mac.sh --sab </path/to/Scripture App Builder.app>

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
DIDO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

REAL_SAB="/Applications/Scripture App Builder.app"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sab) REAL_SAB="$2"; shift 2 ;;
    -h|--help)
      sed -n '1,/^set -e/p' "$0" | sed -n '2,$p' | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "this test only runs on macOS." >&2; exit 1
fi
[[ -d "$REAL_SAB" ]] || { echo "Real SAB not found at $REAL_SAB" >&2; exit 1; }

TMP=$(mktemp -d /tmp/dido-mac-test.XXXXXX)
trap 'rm -rf "$TMP"' EXIT

# Clone to a non-.app path so macOS TCC App Management doesn't lock
# the bundle the first time a binary inside it executes (Sonoma+).
FAKE_SAB="$TMP/SAB-bundle"
FAKE_HOME="$TMP/home"
mkdir -p "$FAKE_HOME"

echo "════════ deep-fake setup ════════"
echo "tmp:        $TMP"
echo "fake SAB:   $FAKE_SAB  (non-.app path — bypasses TCC App Management)"
echo "fake HOME:  $FAKE_HOME"

echo
echo "==> cloning SAB.app contents (APFS cp -c; clone-on-write, ~instant)"
cp -cR "$REAL_SAB" "$FAKE_SAB"
echo "done. cloned size on disk: $(du -sh "$FAKE_SAB" 2>/dev/null | awk '{print $1}')"

chmod -R u+w "$FAKE_SAB" 2>/dev/null || true
xattr -cr "$FAKE_SAB" 2>/dev/null || true

if [[ ! -w "$FAKE_SAB/Contents/MacOS" ]]; then
  echo "✗ Contents/MacOS still not writable after chmod + xattr strip:"
  /bin/ls -laed "$FAKE_SAB/Contents/MacOS" >&2
  exit 1
fi
echo "==> clone is user-writable: $(/bin/ls -laed "$FAKE_SAB/Contents/MacOS" | awk '{print $1}')"

TARGET="$FAKE_SAB/Contents/MacOS/go-aeneas"
ORIG_SHA=$(shasum -a 256 "$TARGET" | awk '{print $1}')
ORIG_VER=$("$TARGET" --version 2>&1 | head -1 || true)
echo
echo "════════ snapshot ════════"
echo "orig sha:   $ORIG_SHA"
echo "orig ver:   $ORIG_VER"

# Run a verifier capturing its exit without tripping set -e.
run_check() {
  local out
  out=$("$@" 2>&1) && return 0
  local rc=$?
  printf '%s\n' "$out" | head -5
  return "$rc"
}

echo
echo "════════ pre-install signature ════════"
PRE_CS_RC=0; PRE_SP_RC=0
run_check codesign --verify --strict "$FAKE_SAB" || PRE_CS_RC=$?
echo "codesign --verify exit: $PRE_CS_RC"
run_check spctl --assess --type execute "$FAKE_SAB" || PRE_SP_RC=$?
echo "spctl --assess  exit: $PRE_SP_RC"

echo
echo "════════ install ════════"
HOME="$FAKE_HOME" bash "$DIDO_ROOT/packaging/sab/install.sh" --sab "$FAKE_SAB"

NEW_SHA=$(shasum -a 256 "$TARGET" | awk '{print $1}')
NEW_VER=$("$TARGET" --version 2>&1 | head -1 || true)
BACKUP_SHA=$(shasum -a 256 "$TARGET.sab-original" 2>/dev/null | awk '{print $1}' || true)

echo
echo "════════ post-install ════════"
[[ "$NEW_SHA" != "$ORIG_SHA" ]] && echo "✓ binary swapped" || { echo "✗ NOT swapped"; exit 1; }
echo "$NEW_VER" | grep -q dido && echo "✓ --version reports dido" || { echo "✗ wrong --version: $NEW_VER"; exit 1; }
[[ "$BACKUP_SHA" == "$ORIG_SHA" ]] && echo "✓ backup byte-identical to original" || { echo "✗ backup mismatch"; exit 1; }

KEYS_FILE="$FAKE_HOME/App Builder/Scripture Apps/keys.txt"
[[ -f "$KEYS_FILE" ]] && grep -qF "2402-4537-40F6-BFF6" "$KEYS_FILE" \
  && echo "✓ feature-flag in $KEYS_FILE" \
  || echo "ⓘ keys.txt absent (no settings.xml in fake \$HOME — expected)"

echo
echo "════════ post-install signature ════════"
POST_CS_RC=0; POST_SP_RC=0
run_check codesign --verify --strict "$FAKE_SAB" || POST_CS_RC=$?
echo "codesign --verify exit: $POST_CS_RC"
run_check spctl --assess --type execute "$FAKE_SAB" || POST_SP_RC=$?
echo "spctl --assess  exit: $POST_SP_RC"

echo
echo "════════ Gatekeeper assessment ════════"
if [[ $POST_CS_RC -ne 0 || $POST_SP_RC -ne 0 ]]; then
  cat <<EOF
⚠ The bundle's code signature is invalidated by the binary swap.

  codesign --verify:  pre=$PRE_CS_RC  post=$POST_CS_RC
  spctl --assess:     pre=$PRE_SP_RC  post=$POST_SP_RC

  Real-world impact: SAB.app still launches if Gatekeeper pre-approved
  it before the swap (the quarantine bit isn't reset). For users who
  copy SAB fresh from the .dmg after installing dido, Gatekeeper
  refuses to launch. Mitigations:
    codesign --force --deep --sign - "<SAB.app>"
    xattr -dr com.apple.quarantine "<SAB.app>"
EOF
else
  echo "✓ Bundle signature still valid after swap."
fi

echo
echo "════════ --batch smoke ════════"
ASSETS="$DIDO_ROOT/../aeneas/aeneas/tests/res/container/job/assets"
if [[ -f "$ASSETS/p001.mp3" ]]; then
  AUDIO="$ASSETS/p001.mp3"
else
  AUDIO="$TMP/silence.wav"
  ffmpeg -nostdin -y -f lavfi \
         -i 'anullsrc=channel_layout=mono:sample_rate=16000' \
         -t 3 "$AUDIO" >/dev/null 2>&1
fi
cat > "$TMP/phrases.txt" << 'EOF'
f001|From fairest creatures we desire increase,
f002|That thereby beauty's rose might never die,
EOF
cat > "$TMP/batch.json" << EOF
[{"description":"mac-smoke","audioFilename":"$AUDIO","phraseFilename":"$TMP/phrases.txt","parameters":"task_language=eng|is_text_type=parsed|os_task_file_format=tsv|os_task_file_head_tail_format=hidden","outputFilename":"$TMP/out.tsv"}]
EOF
"$TARGET" --batch "$TMP/batch.json" 2>&1 | tail -3
[[ -s "$TMP/out.tsv" ]] && echo "✓ --batch wrote output" || { echo "✗ --batch produced no output"; exit 1; }
echo "──── out.tsv (first 5 rows) ────"
head -5 "$TMP/out.tsv"

echo
echo "════════ uninstall ════════"
HOME="$FAKE_HOME" bash "$DIDO_ROOT/packaging/sab/uninstall.sh" --sab "$FAKE_SAB"

RESTORED_SHA=$(shasum -a 256 "$TARGET" | awk '{print $1}')
[[ "$RESTORED_SHA" == "$ORIG_SHA" ]] && echo "✓ byte-identical restore" || { echo "✗ restore MISMATCH (orig=$ORIG_SHA, now=$RESTORED_SHA)"; exit 1; }
[[ ! -f "$TARGET.sab-original" ]] && echo "✓ backup file cleaned up" || { echo "✗ backup not removed"; exit 1; }

echo
echo "════════ post-uninstall signature ════════"
FINAL_CS_RC=0
run_check codesign --verify --strict "$FAKE_SAB" || FINAL_CS_RC=$?
echo "codesign --verify exit: $FINAL_CS_RC  (pre-install was $PRE_CS_RC)"
[[ $FINAL_CS_RC -eq $PRE_CS_RC ]] && echo "✓ signature state restored to baseline" \
  || echo "ⓘ signature state differs (may indicate extended-attribute drift; not necessarily a defect)"

echo
echo "════════ all checks passed ════════"
