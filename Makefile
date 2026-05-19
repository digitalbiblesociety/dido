# Build entry points for dido. The `gen` step refreshes the embedded
# SIL ISO 639-3 registry at most once per day (it short-circuits when
# data/iso-639-3-$(date +%F).tab already exists).

GO ?= go
PKGS ?= ./...

.PHONY: build test gen fetch-iso639 clean-iso639-stale

# Default: refresh language data (cheap when current) and build.
build: gen
	$(GO) build $(PKGS)

# Run all tests with up-to-date language data.
test: gen
	$(GO) test $(PKGS)

# Refresh embedded data via `go generate` directives. Idempotent —
# the ISO 639-3 fetcher only hits the network once per day.
gen:
	$(GO) generate $(PKGS)

# Force-fetch the SIL ISO 639-3 registry, ignoring the per-day cache.
# Use when you need a mid-day refresh.
fetch-iso639:
	@today=$$(date +%Y-%m-%d); \
	rm -f internal/language/data/iso-639-3-*.tab; \
	curl -fsSL https://iso639-3.sil.org/sites/iso639-3/files/downloads/iso-639-3.tab \
	  -o internal/language/data/iso-639-3-$$today.tab; \
	ls -la internal/language/data/iso-639-3-*.tab

# Remove old dated cache files. The fetcher normally handles this,
# but this target lets you do it manually if multiple ever accumulate.
clean-iso639-stale:
	@today=$$(date +%Y-%m-%d); \
	find internal/language/data -name 'iso-639-3-*.tab' ! -name "iso-639-3-$$today.tab" -delete -print
