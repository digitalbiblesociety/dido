package pipeline

import (
	"sync"

	"github.com/digitalbiblesociety/dido/internal/mfcc"
)

// compiledCache memoises *mfcc.Compiled by (params, rate) so a multi-level
// alignment doesn't rebuild the same tables once per sibling.
//
// *mfcc.Compiled is NOT safe for concurrent Compute calls and this cache
// hands out the same pointer per key, so callers must not run two
// Computes concurrently against the same (params, rate). The pipeline
// holds to that by construction: the real (16 kHz) and synth (22050 Hz)
// MFCC goroutines key differently, and alignChildren walks siblings
// sequentially.
type compiledCache struct {
	mu sync.Mutex
	m  map[compiledKey]*mfcc.Compiled
}

type compiledKey struct {
	params mfcc.Params
	rate   uint32
}

func newCompiledCache() *compiledCache {
	return &compiledCache{m: make(map[compiledKey]*mfcc.Compiled)}
}

// get returns the Compiled for (p, rate), building on first use. A nil
// receiver disables caching so single-call sites can pass nil instead of
// branching at the call.
func (cc *compiledCache) get(p mfcc.Params, rate uint32) (*mfcc.Compiled, error) {
	if cc == nil {
		return mfcc.Compile(p, rate)
	}
	k := compiledKey{p, rate}
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if c, ok := cc.m[k]; ok {
		return c, nil
	}
	c, err := mfcc.Compile(p, rate)
	if err != nil {
		return nil, err
	}
	cc.m[k] = c
	return c, nil
}
