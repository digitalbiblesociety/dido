// Package tts provides a subprocess-based eSpeak-ng TTS wrapper.
// Port of aeneas/ttswrappers/espeakngttswrapper.py.
package tts

// DefaultVoice is the eSpeak-ng voice used when the language is unknown.
const DefaultVoice = "en"

// DefaultSampleRate is the PCM sample rate produced by eSpeak-ng.
const DefaultSampleRate = 22050

// LanguageToVoice maps aeneas language codes (ISO 639-3 and BCP47 short forms)
// to eSpeak-ng voice strings.
//
// The base set is ported from LANGUAGE_TO_VOICE_CODE in espeakngttswrapper.py
// (aeneas ≤2017). Entries marked "extended" were added to cover languages
// available in eSpeak-ng ≥1.49 that postdate the original Python mapping.
var LanguageToVoice = map[string]string{
	// ── BCP47 / short codes ─────────────────────────────────────────────────
	"af":               "af",
	"am":               "am",
	"an":               "an",
	"as":               "as",
	"az":               "az",
	"ba":               "ba",   // extended: Bashkir
	"be":               "be",   // extended: Belarusian
	"bg":               "bg",
	"bn":               "bn",
	"bpy":              "bpy",  // extended: Bishnupriya Manipuri
	"bs":               "bs",
	"ca":               "ca",
	"ca-ba":            "ca-ba",  // extended: Catalan Balearic
	"ca-nw":            "ca-nw",  // extended: Catalan Northwest
	"ca-va":            "ca-va",  // extended: Catalan Valencian
	"cs":               "cs",
	"cv":               "cv",   // extended: Chuvash
	"cy":               "cy",
	"da":               "da",
	"de":               "de",
	"el":               "el",
	"en":               "en",
	"en-029":           "en-029",
	"en-gb":            "en-gb",
	"en-gb-scotland":   "en-gb-scotland",
	"en-gb-x-gbclan":   "en-gb-x-gbclan",
	"en-gb-x-gbcwmd":   "en-gb-x-gbcwmd",
	"en-gb-x-rp":       "en-gb-x-rp",
	"en-shaw":          "en-shaw", // extended: English Shaw alphabet
	"en-us":            "en-us",
	"en-us-nyc":        "en-us-nyc", // extended
	"eo":               "eo",
	"es":               "es",
	"es-419":           "es-419",
	"et":               "et",
	"eu":               "eu",
	"fa":               "fa",
	"fa-latn":          "fa-latn",
	"fa-Latn":          "fa-latn", // case-insensitive alias
	"fi":               "fi",
	"fo":               "fo",   // extended: Faroese
	"fr":               "fr",
	"fr-be":            "fr-be",
	"fr-ch":            "fr-ch",  // extended: French Switzerland
	"fr-fr":            "fr-fr",
	"ga":               "ga",
	"gd":               "gd",
	"gn":               "gn",
	"grc":              "grc",
	"gu":               "gu",
	"hak":              "hak",  // extended: Hakka Chinese
	"haw":              "haw",  // extended: Hawaiian
	"he":               "he",   // extended: Hebrew
	"hi":               "hi",
	"hr":               "hr",
	"ht":               "ht",   // extended: Haitian Creole
	"hu":               "hu",
	"hy":               "hy",
	"hy-arevmda":       "hy-arevmda",
	"hyw":              "hyw",  // extended: Western Armenian
	"ia":               "ia",
	"id":               "id",
	"io":               "io",   // extended: Ido
	"is":               "is",
	"it":               "it",
	"ja":               "ja",   // extended: Japanese
	"jbo":              "jbo",
	"ka":               "ka",
	"kaa":              "kaa",  // extended: Karakalpak
	"kk":               "kk",   // extended: Kazakh
	"kl":               "kl",
	"kn":               "kn",
	"ko":               "ko",   // extended: Korean
	"kok":              "kok",  // extended: Konkani
	"ku":               "ku",
	"ky":               "ky",
	"la":               "la",
	"lb":               "lb",   // extended: Luxembourgish
	"lfn":              "lfn",
	"lt":               "lt",
	"ltg":              "ltg",  // extended: Latgalian
	"lv":               "lv",
	"mi":               "mi",   // extended: Māori
	"mk":               "mk",
	"ml":               "ml",
	"mr":               "mr",
	"ms":               "ms",
	"mt":               "mt",
	"mto":              "mto",  // extended: Totontepec Mixe
	"my":               "my",
	"nb":               "nb",   // extended: Norwegian Bokmål (espeak-ng uses nb, no is alias)
	"nci":              "nci",
	"ne":               "ne",
	"nl":               "nl",
	"no":               "nb",   // map generic Norwegian to Bokmål
	"nog":              "nog",  // extended: Nogai
	"om":               "om",
	"or":               "or",
	"pa":               "pa",
	"pap":              "pap",
	"piqd":             "piqd", // extended: Klingon
	"pl":               "pl",
	"pt":               "pt",
	"pt-br":            "pt-br",
	"pt-pt":            "pt-pt",
	"py":               "py",   // extended: Pyash
	"qdb":              "qdb",  // extended: Lang Belta
	"qu":               "qu",   // extended: Quechua
	"quc":              "quc",  // extended: K'iche'
	"qya":              "qya",  // extended: Quenya
	"ro":               "ro",
	"ru":               "ru",
	"ru-cl":            "ru-cl",  // extended
	"ru-lv":            "ru-lv",  // extended
	"sd":               "sd",   // extended: Sindhi
	"shn":              "shn",  // extended: Shan
	"si":               "si",
	"sjn":              "sjn",  // extended: Sindarin
	"sk":               "sk",
	"sl":               "sl",
	"smj":              "smj",  // extended: Lule Sami
	"sq":               "sq",
	"sr":               "sr",
	"sv":               "sv",
	"sw":               "sw",
	"ta":               "ta",
	"te":               "te",
	"th":               "th",   // extended: Thai
	"ti":               "ti",   // extended: Tigrinya
	"tk":               "tk",   // extended: Turkmen
	"tn":               "tn",
	"tr":               "tr",
	"tt":               "tt",
	"ug":               "ug",   // extended: Uyghur
	"uk":               "uk",   // native Ukrainian voice (espeak-ng ≥1.49)
	"ur":               "ur",
	"uz":               "uz",   // extended: Uzbek
	"vi":               "vi",
	"vi-vn-x-central":  "vi-vn-x-central",
	"vi-vn-x-south":    "vi-vn-x-south",
	"zh":               "zh",
	"yue":              "yue",  // extended: alias for Cantonese
	"zh-yue":           "zh-yue",

	// ── ISO 639-3 codes ──────────────────────────────────────────────────────
	"afr": "af",
	"amh": "am",
	"arg": "an",
	"asm": "as",
	"aze": "az",
	"bak": "ba",   // extended: Bashkir
	"bel": "be",   // extended: Belarusian
	"ben": "bn",
	"bos": "bs",
	"bul": "bg",
	"cat": "ca",
	"ces": "cs",
	"cmn": "zh",
	"cym": "cy",
	"dan": "da",
	"deu": "de",
	"ell": "el",
	"eng": "en",
	"epo": "eo",
	"est": "et",
	"eus": "eu",
	"fao": "fo",   // extended: Faroese
	"fas": "fa",
	"fin": "fi",
	"fra": "fr",
	"gla": "gd",
	"gle": "ga",
	"grn": "gn",
	"guj": "gu",
	"heb": "he",   // extended: Hebrew
	"hin": "hi",
	"hrv": "hr",
	"hun": "hu",
	"hye": "hy",
	"ina": "ia",
	"ind": "id",
	"isl": "is",
	"ita": "it",
	"jpn": "ja",   // extended: Japanese
	"kal": "kl",
	"kan": "kn",
	"kat": "ka",
	"kaz": "kk",   // extended: Kazakh
	"kir": "ky",
	"kor": "ko",   // extended: Korean
	"kur": "ku",
	"lat": "la",
	"lav": "lv",
	"lit": "lt",
	"ltz": "lb",   // extended: Luxembourgish
	"mal": "ml",
	"mar": "mr",
	"mkd": "mk",
	"mlt": "mt",
	"mri": "mi",   // extended: Māori
	"msa": "ms",
	"mya": "my",
	"nah": "nci",
	"nep": "ne",
	"nld": "nl",
	"nob": "nb",   // extended: Norwegian Bokmål
	"nor": "nb",   // map generic Norwegian to Bokmål
	"ori": "or",
	"orm": "om",
	"pan": "pa",
	"pol": "pl",
	"por": "pt",
	"que": "qu",   // extended: Quechua
	"ron": "ro",
	"rus": "ru",
	"sin": "si",
	"slk": "sk",
	"slv": "sl",
	"snd": "sd",   // extended: Sindhi
	"spa": "es",
	"sqi": "sq",
	"srp": "sr",
	"swa": "sw",
	"swe": "sv",
	"tam": "ta",
	"tat": "tt",
	"tel": "te",
	"tha": "th",   // extended: Thai
	"tir": "ti",   // extended: Tigrinya
	"tsn": "tn",
	"tuk": "tk",   // extended: Turkmen
	"tur": "tr",
	"uig": "ug",   // extended: Uyghur
	"ukr": "uk",   // native Ukrainian (was mocked with Russian in Python aeneas)
	"urd": "ur",
	"uzb": "uz",   // extended: Uzbek
	"vie": "vi",
	"zho": "zh",

	// ── Regional variants ────────────────────────────────────────────────────
	"eng-GBR": "en-gb",
	"eng-SCT": "en-gb-scotland",
	"eng-USA": "en-us",
	"spa-ESP": "es",
	"fra-BEL": "fr-be",
	"fra-FRA": "fr-fr",
	"por-bra": "pt-br",
	"por-prt": "pt-pt",
}

// VoiceFor returns the eSpeak-ng voice string for lang, falling back to DefaultVoice.
func VoiceFor(lang string) string {
	if v, ok := LanguageToVoice[lang]; ok {
		return v
	}
	return DefaultVoice
}

// IsFallbackVoice reports whether VoiceFor(lang) had to fall back to
// DefaultVoice because no native eSpeak-ng voice was mapped for the
// language. Callers use this to decide whether to transliterate
// non-Latin input to Latin before passing it to eSpeak-ng (otherwise
// the English voice would mispronounce the raw byte sequence).
func IsFallbackVoice(lang string) bool {
	_, ok := LanguageToVoice[lang]
	return !ok
}
