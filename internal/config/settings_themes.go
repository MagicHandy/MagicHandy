package config

// Theme constants identify the visual systems bundled with MagicHandy.
const (
	// ThemeSteelAzure is the default MagicHandy visual system.
	ThemeSteelAzure = "steel-azure"

	ThemeWarmTitanium = "warm-titanium"
	ThemeEmber        = "ember"
	ThemeMidnightGold = "midnight-gold"
	ThemeForestBrass  = "forest-brass"
	ThemeHarbor       = "harbor"
	ThemeCarbon       = "carbon"
	ThemeHighContrast = "high-contrast"
)

var supportedUIThemes = [...]string{
	ThemeSteelAzure,
	ThemeWarmTitanium,
	ThemeEmber,
	ThemeMidnightGold,
	ThemeForestBrass,
	ThemeHarbor,
	ThemeCarbon,
	ThemeHighContrast,
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
