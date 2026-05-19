package pipeline

import (
	"testing"

	"github.com/digitalbiblesociety/dido/internal/mfcc"
)

// Same key → same *Compiled; different keys → different *Compileds.
func TestCompiledCacheKeyIdentity(t *testing.T) {
	cc := newCompiledCache()
	p := mfcc.DefaultParams()

	a1, err := cc.get(p, 16000)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := cc.get(p, 16000)
	if err != nil {
		t.Fatal(err)
	}
	if a1 != a2 {
		t.Errorf("same key returned different *Compiled (a1=%p a2=%p)", a1, a2)
	}

	b, err := cc.get(p, 22050)
	if err != nil {
		t.Fatal(err)
	}
	if a1 == b {
		t.Error("different sample rates returned the same *Compiled")
	}

	p2 := p
	p2.MFCCSize = p.MFCCSize + 1
	c, err := cc.get(p2, 16000)
	if err != nil {
		t.Fatal(err)
	}
	if a1 == c {
		t.Error("different params returned the same *Compiled")
	}
}

// A nil receiver disables caching — every call must hand back a fresh
// *Compiled. Pins the no-branching opt-out used by single-level Execute.
func TestCompiledCacheNilDisablesCaching(t *testing.T) {
	var cc *compiledCache
	p := mfcc.DefaultParams()
	a, err := cc.get(p, 16000)
	if err != nil {
		t.Fatal(err)
	}
	b, err := cc.get(p, 16000)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Errorf("nil cache must build a fresh Compiled each call (got same pointer %p)", a)
	}
}

// Compile errors propagate verbatim and don't poison the cache.
func TestCompiledCachePropagatesCompileError(t *testing.T) {
	cc := newCompiledCache()
	bad := mfcc.Params{FFTOrder: 3} // not a power of 2
	if _, err := cc.get(bad, 16000); err == nil {
		t.Fatal("expected error for invalid params")
	}
	if got := len(cc.m); got != 0 {
		t.Errorf("cache stored a failed build: len=%d", got)
	}
	if _, err := cc.get(mfcc.DefaultParams(), 16000); err != nil {
		t.Fatalf("good params after failed build: %v", err)
	}
}
