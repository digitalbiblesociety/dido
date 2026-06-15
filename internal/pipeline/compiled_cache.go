package pipeline

import "github.com/digitalbiblesociety/dido/internal/mfcc"

// Each branch owns its cache. Siblings reuse tables sequentially, even
// when the recording and synthesis have identical parameters and rates.
type mfccCaches struct {
	recording compiledCache
	synthesis compiledCache
}

type compiledCache map[compiledKey]*mfcc.Compiled

type compiledKey struct {
	params mfcc.Params
	rate   uint32
}

// get returns the Compiled for (p, rate), building on first use. A nil
// receiver disables caching so single-call sites can pass nil instead of
// branching at the call.
func (cc compiledCache) get(p mfcc.Params, rate uint32) (*mfcc.Compiled, error) {
	if cc == nil {
		return mfcc.Compile(p, rate)
	}
	k := compiledKey{p, rate}
	if c, ok := cc[k]; ok {
		return c, nil
	}
	c, err := mfcc.Compile(p, rate)
	if err != nil {
		return nil, err
	}
	cc[k] = c
	return c, nil
}
