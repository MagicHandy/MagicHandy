package store

import (
	"path/filepath"
	"testing"
)

func TestUIPreferencesRoundTrip(t *testing.T) {
	t.Parallel()

	dir := TestDir(t)
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	prefs, err := db.LoadUIPreferences()
	if err != nil {
		t.Fatalf("LoadUIPreferences: %v", err)
	}
	if prefs.Locale != "en" {
		t.Fatalf("default locale = %q, want en", prefs.Locale)
	}
	if prefs.LocalePromptDismissed {
		t.Fatal("default locale_prompt_dismissed should be false")
	}

	saved, err := db.SaveUIPreferences(UIPreferences{
		Locale:                "pt",
		LocalePromptDismissed: true,
	})
	if err != nil {
		t.Fatalf("SaveUIPreferences: %v", err)
	}
	if saved.Locale != "pt" || !saved.LocalePromptDismissed {
		t.Fatalf("saved = %+v, want pt + dismissed", saved)
	}

	loaded, err := db.LoadUIPreferences()
	if err != nil {
		t.Fatalf("LoadUIPreferences after save: %v", err)
	}
	if loaded.Locale != "pt" || !loaded.LocalePromptDismissed {
		t.Fatalf("loaded = %+v, want pt + dismissed", loaded)
	}

	if _, err := db.SaveUIPreferences(UIPreferences{Locale: "xx"}); err == nil {
		t.Fatal("expected error for unsupported locale")
	}

	_ = filepath.Join(dir, DBFileName)
}
