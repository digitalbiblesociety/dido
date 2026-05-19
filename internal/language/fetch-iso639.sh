#!/usr/bin/env sh
# Refresh the embedded SIL ISO 639-3 registry, at most once per day.
#
# - Cache file: data/iso-639-3-<YYYY-MM-DD>.tab (one per day on disk)
# - If today's file already exists, exit fast — no network call.
# - Otherwise, remove any stale dated files and download a new one.
#
# Invoked by the //go:generate directive in iso639.go. Run manually with:
#
#   go generate ./internal/language/

set -eu

cd "$(dirname "$0")"

URL="https://iso639-3.sil.org/sites/iso639-3/files/downloads/iso-639-3.tab"
today=$(date +%Y-%m-%d)
target="data/iso-639-3-${today}.tab"

mkdir -p data

if [ -s "$target" ]; then
  echo "iso-639-3: $target already current, skipping fetch"
  exit 0
fi

echo "iso-639-3: fetching $URL -> $target"
rm -f data/iso-639-3-*.tab
curl -fsSL "$URL" -o "$target"
echo "iso-639-3: wrote $(wc -l <"$target") rows"
