package semantic

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/store"
)

func TestMotionPreferencesSQLiteRoundTrip(t *testing.T) {
	custom := DefaultMotionPreferences()
	custom.Zones[ZoneTip] = ZoneRange{Min: 0.8, Max: 0.95}
	raw, err := MarshalMotionPreferences(custom)
	if err != nil {
		t.Fatal(err)
	}

	dir := store.TestDir(t)
	sqlite, err := config.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := sqlite.Snapshot()
	settings.Motion.MotionPreferences = raw
	if _, err := sqlite.Save(settings); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := config.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotSettings, _ := reloaded.Snapshot()
	loaded := LoadMotionPreferences(gotSettings.Motion.MotionPreferences)
	tip := loaded.Zones[ZoneTip]
	if tip.Min != 0.8 || tip.Max != 0.95 {
		t.Fatalf("tip zone = %+v, want 0.8..0.95", tip)
	}
}

func TestDefaultMotionPreferencesJSONMatchesLoad(t *testing.T) {
	raw := DefaultMotionPreferencesJSON()
	if len(raw) == 0 {
		t.Fatal("default motion preferences JSON is empty")
	}
	loaded := LoadMotionPreferences(raw)
	defaults := DefaultMotionPreferences()
	if loaded.Zones[ZoneTip] != defaults.Zones[ZoneTip] {
		t.Fatalf("loaded tip = %+v, want %+v", loaded.Zones[ZoneTip], defaults.Zones[ZoneTip])
	}
}
