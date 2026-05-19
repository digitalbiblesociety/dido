#!/usr/bin/env bash
# Build dido binaries named `go-aeneas` for install.sh to drop in.
# Usage: ./build.sh [mac|linux|all]   (default: host OS)

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
MODULE_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
DIST="$SCRIPT_DIR/dist"

target="${1:-host}"
if [[ "$target" == "host" ]]; then
  case "$(uname -s)" in
    Darwin) target="mac" ;;
    Linux)  target="linux" ;;
    *) echo "unsupported host OS: $(uname -s)" >&2; exit 1 ;;
  esac
fi

cd "$MODULE_ROOT"

build_arch() {
  local goos="$1" goarch="$2" out="$3"
  mkdir -p "$(dirname "$out")"
  echo "  building $goos/$goarch → ${out#"$MODULE_ROOT/"}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="-s -w" -o "$out" ./cmd/dido
}

build_mac() {
  local outdir="$DIST/mac"
  local universal="$outdir/go-aeneas"
  local amd64="$outdir/.go-aeneas.amd64"
  local arm64="$outdir/.go-aeneas.arm64"

  mkdir -p "$outdir"
  build_arch darwin amd64 "$amd64"
  build_arch darwin arm64 "$arm64"

  echo "  lipo-merging → $(basename "$universal")"
  lipo -create -output "$universal" "$amd64" "$arm64"

  echo "  codesign (ad-hoc)"
  codesign --force --identifier "go-aeneas-dido" --sign - "$universal" 2>&1 |
    sed 's/^/    /'

  rm -f "$amd64" "$arm64"
  echo
  file "$universal"
  ls -la "$universal"
}

build_linux() {
  build_arch linux amd64 "$DIST/linux/amd64/go-aeneas"
  build_arch linux arm64 "$DIST/linux/arm64/go-aeneas"
}

case "$target" in
  mac)   build_mac ;;
  linux) build_linux ;;
  all)   build_mac; build_linux ;;
  *)     echo "unknown target: $target (want: mac, linux, all)" >&2; exit 2 ;;
esac

echo
echo "Build complete. Output under $DIST/"
