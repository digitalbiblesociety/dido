package vad

import (
	"testing"
)

func TestRunVADAllSpeech(t *testing.T) {
	// uniform energy: everything speech
	energy := []float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	p := Params{LogEnergyThreshold: 0.0, MinNonspeechLength: 3}
	mask := RunVAD(energy, p)
	for i, v := range mask {
		if !v {
			t.Errorf("frame %d expected speech, got silence", i)
		}
	}
}

func TestRunVADClearSilence(t *testing.T) {
	// frames 4-7 have very low energy → should be marked nonspeech
	energy := []float64{5, 5, 5, 5, 0, 0, 0, 0, 5, 5}
	p := Params{LogEnergyThreshold: 2.0, MinNonspeechLength: 3}
	mask := RunVAD(energy, p)
	if len(mask) != len(energy) {
		t.Fatalf("mask length %d, want %d", len(mask), len(energy))
	}
	// frames 0-3 should be speech
	for i := 0; i < 4; i++ {
		if !mask[i] {
			t.Errorf("frame %d should be speech", i)
		}
	}
	// frames 4-6 (window of 3 starting at 4) should be silence
	// stop = 3 + 3 = 6 → mask[4:6] = false
	for i := 4; i < 7; i++ {
		if mask[i] {
			t.Errorf("frame %d should be silence", i)
		}
	}
	// frames 8-9 should be speech
	for i := 8; i < 10; i++ {
		if !mask[i] {
			t.Errorf("frame %d should be speech", i)
		}
	}
}

func TestRunVADExtendAfter(t *testing.T) {
	// silence at frames 4-7, extend_after=1 → silence starts at frame 5
	energy := []float64{5, 5, 5, 5, 0, 0, 0, 0, 5, 5}
	p := Params{LogEnergyThreshold: 2.0, MinNonspeechLength: 3, ExtendAfter: 1}
	mask := RunVAD(energy, p)
	// With extend_after=1: start of silence run (4) + 1 = 5
	if !mask[4] {
		t.Errorf("frame 4 should be speech due to extend_after=1")
	}
	if mask[5] {
		t.Errorf("frame 5 should be silence")
	}
}

func TestRunVADExtendBefore(t *testing.T) {
	// silence at frames 2-8, extend_before=1 → silence stops 1 frame earlier
	energy := make([]float64, 15)
	for i := range energy {
		energy[i] = 5
	}
	for i := 2; i <= 8; i++ {
		energy[i] = 0
	}
	p := Params{LogEnergyThreshold: 2.0, MinNonspeechLength: 3, ExtendBefore: 1}
	mask := RunVAD(energy, p)
	// With extend_before=1: stop (run.end + w = 6+3=9) - 1 = 8 → mask[2:8] = false
	// So frame 8 should be speech (not marked silence)
	if !mask[8] {
		t.Errorf("frame 8 should be speech due to extend_before=1")
	}
}

func TestRunVADEmpty(t *testing.T) {
	mask := RunVAD(nil, Params{MinNonspeechLength: 3})
	if len(mask) != 0 {
		t.Errorf("expected empty mask for empty input")
	}
}

func TestComputeIntervals(t *testing.T) {
	mask := []bool{true, true, false, false, true, false, true, true, true}
	si := ComputeIntervals(mask, true)
	if len(si) != 3 {
		t.Fatalf("want 3 speech intervals, got %d: %v", len(si), si)
	}
	if si[0] != [2]int{0, 1} {
		t.Errorf("si[0] = %v", si[0])
	}
	if si[1] != [2]int{4, 4} {
		t.Errorf("si[1] = %v", si[1])
	}
	if si[2] != [2]int{6, 8} {
		t.Errorf("si[2] = %v", si[2])
	}
	ni := ComputeIntervals(mask, false)
	if len(ni) != 2 {
		t.Fatalf("want 2 nonspeech intervals, got %d: %v", len(ni), ni)
	}
}

func TestParamsFromSeconds(t *testing.T) {
	p := ParamsFromSeconds(0.699, 0.200, 0.100, 0.050, 0.040)
	if p.MinNonspeechLength != 5 {
		t.Errorf("MinNonspeechLength = %d, want 5", p.MinNonspeechLength)
	}
	if p.ExtendBefore != 2 {
		t.Errorf("ExtendBefore = %d, want 2", p.ExtendBefore)
	}
	if p.ExtendAfter != 1 {
		t.Errorf("ExtendAfter = %d, want 1", p.ExtendAfter)
	}
}
