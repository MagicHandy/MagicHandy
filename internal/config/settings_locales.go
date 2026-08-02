package config

const (
	// LocaleEnglish is the default interface and prompt-catalog locale.
	LocaleEnglish = "en"
	// LocaleSpanish selects Spanish.
	LocaleSpanish = "es"
	// LocalePortugueseBrazil selects Brazilian Portuguese.
	LocalePortugueseBrazil = "pt-BR"
	// LocaleSimplifiedChinese selects Simplified Chinese.
	LocaleSimplifiedChinese = "zh-Hans"
	// LocaleJapanese selects Japanese.
	LocaleJapanese = "ja"
	// UpdateCheckAutomatic checks GitHub Releases after the UI connects.
	UpdateCheckAutomatic = "automatic"
	// UpdateCheckManual only contacts GitHub after an explicit user action.
	UpdateCheckManual = "manual"
)

// UISettings contains presentation preferences shared by every browser client.
type UISettings struct {
	Locale          string `json:"locale"`
	Theme           string `json:"theme"`
	SetupCompleted  bool   `json:"setup_completed"`
	UpdateCheckMode string `json:"update_check_mode,omitempty"`
}

// IsSupportedLocale reports whether locale has bundled UI and prompt catalogs.
func IsSupportedLocale(locale string) bool {
	_, ok := PromptSetForLocale(locale)
	return ok
}

// PromptSetForLocale maps a supported reply locale to its built-in prompt set.
func PromptSetForLocale(locale string) (string, bool) {
	switch locale {
	case LocaleEnglish:
		return PromptSetMagicHandyMotionV1, true
	case LocaleSpanish:
		return PromptSetMagicHandyMotionV1ES, true
	case LocalePortugueseBrazil:
		return PromptSetMagicHandyMotionV1PTBR, true
	case LocaleSimplifiedChinese:
		return PromptSetMagicHandyMotionV1ZHHans, true
	case LocaleJapanese:
		return PromptSetMagicHandyMotionV1JA, true
	default:
		return "", false
	}
}
