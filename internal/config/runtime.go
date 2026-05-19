// Package config provides runtime and task configuration.
// Port of aeneas/runtimeconfiguration.py + aeneas/configuration.py.
package config

import (
	"strconv"
	"strings"
)

// ParseConfigString splits "key1=val1|key2=val2|..." into a map.
// Matches globalconstants.py CONFIG_STRING_* symbols.
func ParseConfigString(s string) map[string]string {
	// Strip surrounding quotes if present.
	if len(s) >= 2 && s[0] == s[len(s)-1] && (s[0] == '"' || s[0] == '\'') {
		s = s[1 : len(s)-1]
	}
	m := make(map[string]string)
	for _, pair := range strings.Split(s, "|") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.IndexByte(pair, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(pair[:idx])
		val := strings.TrimSpace(pair[idx+1:])
		if key != "" {
			m[key] = val
		}
	}
	return m
}

// trueAliases matches the Python Configuration.TRUE_ALIASES list.
var trueAliases = map[string]bool{
	"TRUE": true, "True": true, "true": true,
	"YES": true, "Yes": true, "yes": true,
	"1": true,
}

func parseBool(v string) bool  { return trueAliases[v] }
func parseFloat(v string) float64 {
	f, _ := strconv.ParseFloat(v, 64)
	return f
}
func parseInt(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}

// RuntimeConfig holds all runtime configuration parameters.
// Field names match the Python RuntimeConfiguration key constants.
// Defaults reproduce the Python FIELDS defaults exactly.
type RuntimeConfig struct {
	// ABA (adjust boundary algorithm)
	ABANonspeechTolerance float64 // aba_nonspeech_tolerance
	ABANoZeroDuration     float64 // aba_no_zero_duration

	// Extension flags (always true in the Go port)
	AllowUnlistedLanguages bool // allow_unlisted_languages

	// DTW
	DTWAlgorithm string  // dtw_algorithm: "stripe" or "exact"
	DTWMargin    float64 // dtw_margin, seconds
	DTWMarginL1  float64 // dtw_margin_l1
	DTWMarginL2  float64 // dtw_margin_l2
	DTWMarginL3  float64 // dtw_margin_l3

	// MFCC
	MFCCFilters       int     // mfcc_filters
	MFCCSize          int     // mfcc_size
	MFCCFFTOrder      int     // mfcc_fft_order
	MFCCLowerFreq     float64 // mfcc_lower_frequency
	MFCCUpperFreq     float64 // mfcc_upper_frequency
	MFCCEmphasis      float64 // mfcc_emphasis_factor
	MFCCWindowLength  float64 // mfcc_window_length
	MFCCWindowShift   float64 // mfcc_window_shift
	MFCCMaskNonspeech bool    // mfcc_mask_nonspeech

	// Per-level MFCC overrides
	MFCCMaskNonspeechL1 bool
	MFCCWindowLengthL1  float64
	MFCCWindowShiftL1   float64
	MFCCMaskNonspeechL2 bool
	MFCCWindowLengthL2  float64
	MFCCWindowShiftL2   float64
	MFCCMaskNonspeechL3 bool
	MFCCWindowLengthL3  float64
	MFCCWindowShiftL3   float64

	// MFCC nonspeech masking
	MFCCMaskExtendAfter       int
	MFCCMaskExtendBefore      int
	MFCCMaskLogEnergyThreshold float64
	MFCCMaskMinNonspeechLength int

	// Audio / FFmpeg
	FFMpegPath       string // ffmpeg_path
	FFMpegSampleRate int    // ffmpeg_sample_rate
	FFProbePath      string // ffprobe_path

	// TTS
	TTS            string // tts engine name
	TTSPath        string // path to TTS binary/wrapper
	TTSVoice       string // tts_voice_code
	TTSCache       bool
	TTSConcurrency int  // tts_concurrency: max parallel TTS processes (0 = auto)
	TTSAutoTranslit bool // tts_auto_transliterate: romanise non-Latin text when no native voice (default true)

	// Per-level TTS
	TTSL1     string
	TTSPathL1 string
	TTSL2     string
	TTSPathL2 string
	TTSL3     string
	TTSPathL3 string

	// VAD
	VADExtendAfter       float64 // vad_extend_speech_after
	VADExtendBefore      float64 // vad_extend_speech_before
	VADLogEnergyThreshold float64
	VADMinNonspeechLength float64

	// Safety
	SafetyChecks bool

	// Limits
	TaskMaxAudioLength float64
	TaskMaxTextLength  int
	JobMaxTasks        int

	// Temp path
	TmpPath string
}

// Default returns a RuntimeConfig with the same defaults as the Python FIELDS.
func Default() RuntimeConfig {
	return RuntimeConfig{
		ABANonspeechTolerance: 0.080,
		ABANoZeroDuration:     0.001,

		AllowUnlistedLanguages: false,

		DTWAlgorithm: "stripe",
		DTWMargin:    60.0,
		DTWMarginL1:  60.0,
		DTWMarginL2:  30.0,
		DTWMarginL3:  10.0,

		MFCCFilters:      40,
		MFCCSize:         13,
		MFCCFFTOrder:     512,
		MFCCLowerFreq:    133.3333,
		MFCCUpperFreq:    6855.4976,
		MFCCEmphasis:     0.970,
		MFCCWindowLength: 0.100,
		MFCCWindowShift:  0.040,

		MFCCWindowLengthL1: 0.100,
		MFCCWindowShiftL1:  0.040,
		MFCCWindowLengthL2: 0.050,
		MFCCWindowShiftL2:  0.020,
		MFCCWindowLengthL3: 0.020,
		MFCCWindowShiftL3:  0.005,

		MFCCMaskLogEnergyThreshold: 0.699,
		MFCCMaskMinNonspeechLength: 1,

		FFMpegPath:       "ffmpeg",
		FFMpegSampleRate: 16000,
		FFProbePath:      "ffprobe",

		TTS:   "espeak-ng",
		TTSL1: "", // empty L-value = "no per-level override"; see SetTTS
		TTSL2: "",
		TTSL3: "",
		TTSAutoTranslit:  true,

		VADLogEnergyThreshold: 0.699,
		VADMinNonspeechLength: 0.200,

		SafetyChecks: true,

		TaskMaxAudioLength: 0,
		TaskMaxTextLength:  0,
		JobMaxTasks:        0,
	}
}

// Parse returns a RuntimeConfig starting from the defaults and overriding
// with values from the given config string ("key1=val1|key2=val2|...").
// Unknown keys are silently ignored; use ParseStrict to inspect them.
func Parse(configString string) RuntimeConfig {
	rc, _ := ParseStrict(configString)
	return rc
}

// ParseStrict is like Parse but also returns a sorted slice of keys from
// configString that this package does not recognise. The slice is empty
// when every key was consumed.
//
// Note: callers that combine runtime config with another schema (e.g.
// pipeline.TaskConfig also reads from the same string) should intersect
// the returned list with the other parser's unknown set — a key may be
// unknown to RuntimeConfig but recognised by another consumer.
func ParseStrict(configString string) (RuntimeConfig, []string) {
	rc := Default()
	if configString == "" {
		return rc, nil
	}
	kv := ParseConfigString(configString)
	var unknown []string
	for k, v := range kv {
		if !rc.set(k, v) {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 1 {
		sortStrings(unknown)
	}
	return rc, unknown
}

func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// set applies one key=value pair onto the config. Returns true if the key
// was recognised. Keys match Python field names.
func (rc *RuntimeConfig) set(key, val string) bool {
	switch key {
	case "aba_nonspeech_tolerance":
		rc.ABANonspeechTolerance = parseFloat(val)
	case "aba_no_zero_duration":
		rc.ABANoZeroDuration = parseFloat(val)
	case "allow_unlisted_languages":
		rc.AllowUnlistedLanguages = parseBool(val)
	case "dtw_algorithm":
		rc.DTWAlgorithm = val
	case "dtw_margin":
		rc.DTWMargin = parseFloat(val)
	case "dtw_margin_l1":
		rc.DTWMarginL1 = parseFloat(val)
	case "dtw_margin_l2":
		rc.DTWMarginL2 = parseFloat(val)
	case "dtw_margin_l3":
		rc.DTWMarginL3 = parseFloat(val)
	case "mfcc_filters":
		rc.MFCCFilters = parseInt(val)
	case "mfcc_size":
		rc.MFCCSize = parseInt(val)
	case "mfcc_fft_order":
		rc.MFCCFFTOrder = parseInt(val)
	case "mfcc_lower_frequency":
		rc.MFCCLowerFreq = parseFloat(val)
	case "mfcc_upper_frequency":
		rc.MFCCUpperFreq = parseFloat(val)
	case "mfcc_emphasis_factor":
		rc.MFCCEmphasis = parseFloat(val)
	case "mfcc_window_length":
		rc.MFCCWindowLength = parseFloat(val)
	case "mfcc_window_shift":
		rc.MFCCWindowShift = parseFloat(val)
	case "mfcc_mask_nonspeech":
		rc.MFCCMaskNonspeech = parseBool(val)
	case "mfcc_window_length_l1":
		rc.MFCCWindowLengthL1 = parseFloat(val)
	case "mfcc_window_shift_l1":
		rc.MFCCWindowShiftL1 = parseFloat(val)
	case "mfcc_mask_nonspeech_l1":
		rc.MFCCMaskNonspeechL1 = parseBool(val)
	case "mfcc_window_length_l2":
		rc.MFCCWindowLengthL2 = parseFloat(val)
	case "mfcc_window_shift_l2":
		rc.MFCCWindowShiftL2 = parseFloat(val)
	case "mfcc_mask_nonspeech_l2":
		rc.MFCCMaskNonspeechL2 = parseBool(val)
	case "mfcc_window_length_l3":
		rc.MFCCWindowLengthL3 = parseFloat(val)
	case "mfcc_window_shift_l3":
		rc.MFCCWindowShiftL3 = parseFloat(val)
	case "mfcc_mask_nonspeech_l3":
		rc.MFCCMaskNonspeechL3 = parseBool(val)
	case "mfcc_mask_extend_speech_after":
		rc.MFCCMaskExtendAfter = parseInt(val)
	case "mfcc_mask_extend_speech_before":
		rc.MFCCMaskExtendBefore = parseInt(val)
	case "mfcc_mask_log_energy_threshold":
		rc.MFCCMaskLogEnergyThreshold = parseFloat(val)
	case "mfcc_mask_min_nonspeech_length":
		rc.MFCCMaskMinNonspeechLength = parseInt(val)
	case "ffmpeg_path":
		rc.FFMpegPath = val
	case "ffmpeg_sample_rate":
		rc.FFMpegSampleRate = parseInt(val)
	case "ffprobe_path":
		rc.FFProbePath = val
	case "tts":
		rc.TTS = val
	case "tts_path":
		rc.TTSPath = val
	case "tts_voice_code":
		rc.TTSVoice = val
	case "tts_cache":
		rc.TTSCache = parseBool(val)
	case "tts_concurrency":
		rc.TTSConcurrency = parseInt(val)
	case "tts_auto_transliterate":
		rc.TTSAutoTranslit = parseBool(val)
	case "tts_l1":
		rc.TTSL1 = val
	case "tts_path_l1":
		rc.TTSPathL1 = val
	case "tts_l2":
		rc.TTSL2 = val
	case "tts_path_l2":
		rc.TTSPathL2 = val
	case "tts_l3":
		rc.TTSL3 = val
	case "tts_path_l3":
		rc.TTSPathL3 = val
	case "vad_extend_speech_after":
		rc.VADExtendAfter = parseFloat(val)
	case "vad_extend_speech_before":
		rc.VADExtendBefore = parseFloat(val)
	case "vad_log_energy_threshold":
		rc.VADLogEnergyThreshold = parseFloat(val)
	case "vad_min_nonspeech_length":
		rc.VADMinNonspeechLength = parseFloat(val)
	case "safety_checks":
		rc.SafetyChecks = parseBool(val)
	case "task_max_audio_length":
		rc.TaskMaxAudioLength = parseFloat(val)
	case "task_max_text_length":
		rc.TaskMaxTextLength = parseInt(val)
	case "job_max_tasks":
		rc.JobMaxTasks = parseInt(val)
	case "tmp_path":
		rc.TmpPath = val
	default:
		return false
	}
	return true
}

// SetGranularity updates MFCC window and DTW margin parameters for the
// alignment level being processed:
//
//	1 = paragraph     → L1 params (coarsest)
//	2 = sentence      → L2 params
//	3 = phrase, 4 = word → L3 params (finest; both reuse the same band)
//
// aeneas Python only models three levels (paragraph/sentence/word); the
// phrase layer is a dido extension that piggy-backs on L3 since both
// phrase and word are fine-grained leaves.
func (rc *RuntimeConfig) SetGranularity(level int) {
	switch {
	case level <= 1:
		rc.DTWMargin = rc.DTWMarginL1
		rc.MFCCMaskNonspeech = rc.MFCCMaskNonspeechL1
		rc.MFCCWindowLength = rc.MFCCWindowLengthL1
		rc.MFCCWindowShift = rc.MFCCWindowShiftL1
	case level == 2:
		rc.DTWMargin = rc.DTWMarginL2
		rc.MFCCMaskNonspeech = rc.MFCCMaskNonspeechL2
		rc.MFCCWindowLength = rc.MFCCWindowLengthL2
		rc.MFCCWindowShift = rc.MFCCWindowShiftL2
	default: // 3 (phrase), 4 (word), or any deeper future level
		rc.DTWMargin = rc.DTWMarginL3
		rc.MFCCMaskNonspeech = rc.MFCCMaskNonspeechL3
		rc.MFCCWindowLength = rc.MFCCWindowLengthL3
		rc.MFCCWindowShift = rc.MFCCWindowShiftL3
	}
}

// SetTTS updates TTS and TTSPath for the given granularity level.
// Phrase and word (levels 3-4) reuse L3 TTS params for the same reason
// SetGranularity does. Empty L-values are skipped so a user-set global
// `tts=`/`tts_path=` survives SetTTS when no per-level value was given.
func (rc *RuntimeConfig) SetTTS(level int) {
	var lvlTTS, lvlPath string
	switch {
	case level <= 1:
		lvlTTS, lvlPath = rc.TTSL1, rc.TTSPathL1
	case level == 2:
		lvlTTS, lvlPath = rc.TTSL2, rc.TTSPathL2
	default:
		lvlTTS, lvlPath = rc.TTSL3, rc.TTSPathL3
	}
	if lvlTTS != "" {
		rc.TTS = lvlTTS
	}
	if lvlPath != "" {
		rc.TTSPath = lvlPath
	}
}

// MFCCParams returns the MFCC computation parameters derived from this config.
func (rc *RuntimeConfig) MFCCParams() MFCCParams {
	return MFCCParams{
		FilterBankSize: rc.MFCCFilters,
		MFCCSize:       rc.MFCCSize,
		FFTOrder:       rc.MFCCFFTOrder,
		LowerFrequency: rc.MFCCLowerFreq,
		UpperFrequency: rc.MFCCUpperFreq,
		EmphasisFactor: rc.MFCCEmphasis,
		WindowLength:   rc.MFCCWindowLength,
		WindowShift:    rc.MFCCWindowShift,
	}
}

// MFCCParams mirrors mfcc.Params for decoupled use in config.
// Consumers should convert this to mfcc.Params.
type MFCCParams struct {
	FilterBankSize int
	MFCCSize       int
	FFTOrder       int
	LowerFrequency float64
	UpperFrequency float64
	EmphasisFactor float64
	WindowLength   float64
	WindowShift    float64
}
