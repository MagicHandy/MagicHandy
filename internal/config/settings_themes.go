package config

// Theme constants identify the visual systems bundled with MagicHandy.
const (
	// ThemeSteelAzure is the default MagicHandy visual system.
	ThemeSteelAzure = "steel-azure"

	ThemeCarbonEmerald    = "carbon-emerald"
	ThemeCarbonStealth    = "carbon-stealth"
	ThemePlatinum         = "platinum"
	ThemePolarFrost       = "polar-frost"
	ThemeSage             = "sage"
	ThemeAmberPhosphor    = "amber-phosphor"
	ThemeGreenPhosphor    = "green-phosphor"
	ThemePaperwhiteCRT    = "paperwhite-crt"
	ThemeCyberMidnight    = "cyber-midnight"
	ThemeDeepViolet       = "deep-violet"
	ThemeHighContrast     = "high-contrast"
	ThemeIcebergSlate     = "iceberg-slate"
	ThemeMidnightGraphite = "midnight-graphite"
	ThemeMoonlight        = "moonlight-lavender"
	ThemeNordicDusk       = "nordic-dusk"
	ThemeObsidianTeal     = "obsidian-teal"
	ThemeObsidianViolet   = "obsidian-violet"
	ThemeOceanTrench      = "ocean-trench"
	ThemeSolarisAmber     = "solaris-amber"
	ThemeWarmTitanium     = "warm-titanium"
	ThemeZenithMonochrome = "zenith-monochrome"
)

var supportedUIThemes = [...]string{
	ThemeSteelAzure,
	ThemeCarbonEmerald,
	ThemeCarbonStealth,
	ThemePlatinum,
	ThemePolarFrost,
	ThemeSage,
	ThemeAmberPhosphor,
	ThemeGreenPhosphor,
	ThemePaperwhiteCRT,
	ThemeCyberMidnight,
	ThemeDeepViolet,
	ThemeHighContrast,
	ThemeIcebergSlate,
	ThemeMidnightGraphite,
	ThemeMoonlight,
	ThemeNordicDusk,
	ThemeObsidianTeal,
	ThemeObsidianViolet,
	ThemeOceanTrench,
	ThemeSolarisAmber,
	ThemeWarmTitanium,
	ThemeZenithMonochrome,
}

// SupportedUIThemes returns the ordered settings catalog. The default remains
// first; callers receive a copy so the validation catalog cannot be mutated.
func SupportedUIThemes() []string {
	return append([]string(nil), supportedUIThemes[:]...)
}

// IsSupportedUITheme reports whether theme has a bundled CSS token set.
func IsSupportedUITheme(theme string) bool {
	return oneOf(theme, supportedUIThemes[:]...)
}
