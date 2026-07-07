package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SupportedLocales lists UI locale codes accepted by the app.
var SupportedLocales = []string{"en", "fr", "pt", "ru"}

// UIPreferences stores durable UI choices for one app data directory.
type UIPreferences struct {
	Locale                string `json:"locale"`
	LocalePromptDismissed bool   `json:"locale_prompt_dismissed"`
}

// LoadUIPreferences returns stored UI preferences or defaults.
func (db *DB) LoadUIPreferences() (UIPreferences, error) {
	var locale string
	var dismissed int
	err := db.sql.QueryRow(
		`SELECT locale, locale_prompt_dismissed FROM ui_preferences WHERE id = 1`,
	).Scan(&locale, &dismissed)
	if err == sql.ErrNoRows {
		return UIPreferences{Locale: "en"}, nil
	}
	if err != nil {
		return UIPreferences{}, fmt.Errorf("load ui preferences: %w", err)
	}
	return UIPreferences{
		Locale:                NormalizeLocale(locale),
		LocalePromptDismissed: dismissed != 0,
	}, nil
}

// SaveUIPreferences persists UI preferences.
func (db *DB) SaveUIPreferences(prefs UIPreferences) (UIPreferences, error) {
	locale, err := localeForSave(prefs.Locale)
	if err != nil {
		return UIPreferences{}, err
	}
	prefs.Locale = locale
	dismissed := 0
	if prefs.LocalePromptDismissed {
		dismissed = 1
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = db.withWrite(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO ui_preferences (id, locale, locale_prompt_dismissed, updated_at)
			 VALUES (1, ?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET
			   locale = excluded.locale,
			   locale_prompt_dismissed = excluded.locale_prompt_dismissed,
			   updated_at = excluded.updated_at`,
			prefs.Locale, dismissed, now,
		)
		if err != nil {
			return fmt.Errorf("save ui preferences: %w", err)
		}
		return nil
	})
	if err != nil {
		return UIPreferences{}, err
	}
	return prefs, nil
}

// NormalizeLocale returns a supported locale code or English.
func NormalizeLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	switch locale {
	case "en", "en-us", "en-gb":
		return "en"
	case "fr", "fr-fr":
		return "fr"
	case "pt", "pt-br", "pt-pt":
		return "pt"
	case "ru", "ru-ru":
		return "ru"
	default:
		if LocaleSupported(locale) {
			return locale
		}
		return "en"
	}
}

// LocaleSupported reports whether locale is allowed in the UI.
func LocaleSupported(locale string) bool {
	for _, item := range SupportedLocales {
		if item == locale {
			return true
		}
	}
	return false
}

func localeForSave(locale string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(locale))
	switch normalized {
	case "en", "en-us", "en-gb":
		return "en", nil
	case "fr", "fr-fr":
		return "fr", nil
	case "pt", "pt-br", "pt-pt":
		return "pt", nil
	case "ru", "ru-ru":
		return "ru", nil
	default:
		if LocaleSupported(normalized) {
			return normalized, nil
		}
		return "", fmt.Errorf("unsupported locale %q", locale)
	}
}
